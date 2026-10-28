package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/go-chi/httprate"
	"go.uber.org/zap"

	"github.com/gnolang/tx-indexer/metrics"
	"github.com/gnolang/tx-indexer/serve"
	"github.com/gnolang/tx-indexer/serve/budget"
	"github.com/gnolang/tx-indexer/serve/graph"
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

// timeBudgetGate admits the requests of the API routes to the time budget,
// each client charged under its key. A request over HTTP is admitted, then
// charged its wall time once served; one refused is recorded by the observer,
// under the API its path serves, and answered 429 with Retry-After and its
// API's error. A WebSocket upgrade is neither charged nor refused: it carries
// its client to the connection, which charges each query or call it serves.
type timeBudgetGate struct {
	// cfg reads each request's client key and address
	cfg *startCfg

	// budget is the budget the clients are charged in
	budget *budget.Budget

	// jsonrpc writes a refused JSON-RPC request's answer
	jsonrpc *serve.JSONRPC

	// observer records each refusal
	observer *observe.Observer

	// logger logs what goes wrong writing a refusal
	logger *zap.Logger
}

// wrap admits the requests next serves to the budget
func (g *timeBudgetGate) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if overWebSocket(r) {
			client := budget.Client{Budget: g.budget, Key: g.cfg.clientKey(r), Address: g.cfg.clientAddress(r)}

			next.ServeHTTP(w, r.WithContext(budget.NewContext(r.Context(), client)))

			return
		}

		end, err := g.budget.Reserve(g.cfg.clientKey(r))
		if err != nil {
			g.refuse(w, r, err)

			return
		}

		defer end()

		next.ServeHTTP(w, r)
	})
}

// refuse answers r, refused by the budget with err, with a 429, Retry-After
// and its API's error, and records the refusal
func (g *timeBudgetGate) refuse(w http.ResponseWriter, r *http.Request, err error) {
	var exhausted *budget.ExhaustedError
	if !errors.As(err, &exhausted) {
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	api := apiOf(r.URL.Path)

	g.observer.Refused(g.cfg.clientAddress(r), api, metrics.RefusalClientTimeBudget, exhausted.RetryAfter)

	w.Header().Set("Retry-After", strconv.FormatInt(exhausted.RetryAfterSeconds(), 10))

	if api == metrics.APIJSONRPC {
		g.jsonrpc.WriteRefusal(w, r, err)

		return
	}

	if writeErr := graph.WriteRefusal(w, err); writeErr != nil {
		g.logger.Debug("unable to write a refusal", zap.Error(writeErr))
	}
}

// overWebSocket reports whether r opens a WebSocket connection: any request
// to JSON-RPC's /ws, and a request to GraphQL's endpoint with an Upgrade
// header, which gqlgen hands its WebSocket transport. Neither runs a query
// before the connection is open.
func overWebSocket(r *http.Request) bool {
	switch r.URL.Path {
	case "/ws":
		return true
	case "/graphql/query":
		return r.Header.Get("Upgrade") != ""
	default:
		return false
	}
}

// checkTimeBudget refuses a time budget set inconsistently: one of its two
// flags without the other, a negative value, a refill of 0, which would leave a
// client that spent its budget refused for good and remembered forever, or a
// budget without the query deadline, which bounds each query's execution
func (c *startCfg) checkTimeBudget() error {
	if c.isSet("client-time-budget") != c.isSet("client-time-budget-refill") {
		return errors.New("--client-time-budget and --client-time-budget-refill must be set together")
	}

	if c.clientTimeBudget < 0 || c.clientTimeBudgetRefill < 0 {
		return fmt.Errorf(
			"invalid --client-time-budget %s or --client-time-budget-refill %s: neither may be negative",
			c.clientTimeBudget,
			c.clientTimeBudgetRefill,
		)
	}

	if c.clientTimeBudget == 0 {
		return nil
	}

	if c.clientTimeBudgetRefill == 0 {
		return fmt.Errorf(
			"invalid --client-time-budget-refill 0s with --client-time-budget %s: a budget needs a refill above 0",
			c.clientTimeBudget,
		)
	}

	if c.queryDeadline <= 0 {
		return fmt.Errorf(
			"--client-time-budget %s needs --query-deadline above 0: the deadline bounds each query's execution",
			c.clientTimeBudget,
		)
	}

	return nil
}

// isSet reports whether the flag named name was set, to its default or not
func (c *startCfg) isSet(name string) bool {
	set := false

	if c.flags != nil {
		c.flags.Visit(func(f *flag.Flag) {
			set = set || f.Name == name
		})
	}

	return set
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
