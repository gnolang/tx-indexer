package main

import (
	"net/http"
	"net/netip"
	"time"

	"github.com/go-chi/httprate"
	"go.uber.org/zap"

	"github.com/gnolang/tx-indexer/metrics"
	"github.com/gnolang/tx-indexer/serve/observe"
)

// rateLimitWindow is the window --http-rate-limit counts a client's requests
// over, and the Retry-After httprate answers a refusal with
const rateLimitWindow = time.Minute

// ipv6ClientBits is the length of the prefix an IPv6 client is counted under:
// one end site is given a /64 or wider, so its host picks any address in it
const ipv6ClientBits = 64

// clientAddress is the address of the client that sent r, the one the request
// log names: read from --client-ip-header as far as the trusted proxies
// reach, or the address the request came from with no header trusted. A
// header a client sets itself is never read.
func (c *startCfg) clientAddress(r *http.Request) string {
	return observe.ClientAddress(r, c.clientIPHeader, c.clientIPTrustedHops)
}

// clientKey is the key the client limits count r under: its client address.
// An IPv6 client is counted under its /64, as httprate's own key functions
// count one. An IPv4 address in IPv6 form is counted as the IPv4 address it
// names, where those functions count every such address under one key, "::".
func (c *startCfg) clientKey(r *http.Request) string {
	client := c.clientAddress(r)

	address, err := netip.ParseAddr(client)
	if err != nil {
		// An address that is no IP, which no TCP peer has, is counted as it
		// is
		return client
	}

	address = address.Unmap()
	if address.Is4() {
		return address.String()
	}

	return netip.PrefixFrom(address, ipv6ClientBits).Masked().String()
}

// newRateLimit is the count limit, --http-rate-limit requests per minute for
// each client key. A refused request is recorded by observer, under the API
// its path serves, and answered 429 with a JSON body the GraphQL playground
// shows.
func (c *startCfg) newRateLimit(observer *observe.Observer, logger *zap.Logger) func(http.Handler) http.Handler {
	return httprate.Limit(
		c.rateLimit,
		rateLimitWindow,
		httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
			return c.clientKey(r), nil
		}),
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			logger.Debug("too many requests", zap.String("from", c.clientKey(r)))

			observer.Refused(c.clientAddress(r), apiOf(r.URL.Path), metrics.RefusalHTTPRateLimit, rateLimitWindow)

			http.Error(w, `{"error": "too many requests"}`, http.StatusTooManyRequests)
		}),
	)
}

// apiOf names the API a request path serves: JSON-RPC's POST endpoint and its
// WebSocket, and GraphQL's endpoint. It is "" for any other path: /health,
// /ready and the playground.
func apiOf(path string) string {
	switch path {
	case "/", "/ws":
		return metrics.APIJSONRPC
	case "/graphql/query":
		return metrics.APIGraphQL
	default:
		return ""
	}
}

// onAPIRoutes applies limit to the requests of the API routes, and passes the
// others on as they are, so a load balancer's health checks are never refused
func onAPIRoutes(limit func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		limited := limit(next)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if apiOf(r.URL.Path) == "" {
				next.ServeHTTP(w, r)

				return
			}

			limited.ServeHTTP(w, r)
		})
	}
}
