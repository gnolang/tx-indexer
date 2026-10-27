package observe

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// clientAddress is the IP address of the client that sent r. With no header
// trusted, it is the address the request came from. With one, it is the entry
// trustedHops from the right of the comma-separated list the header carries
// over all its lines: each trusted proxy appends the address it was reached
// from, so the entries further left are the client's own to forge, and are
// never read. A header missing, holding fewer entries than trustedHops, or
// holding no IP address at that entry leaves the address the request came
// from. An IPv6 zone is dropped: it names a link on the proxy's host, in text
// the client can choose.
func clientAddress(r *http.Request, header string, trustedHops int) string {
	peer := hostOf(r.RemoteAddr)

	if header == "" || trustedHops < 1 {
		return peer
	}

	entry, ok := entryFromRight(r.Header.Values(header), trustedHops)
	if !ok {
		return peer
	}

	client, err := netip.ParseAddr(strings.TrimSpace(entry))
	if err != nil {
		return peer
	}

	return client.WithZone("").String()
}

// entryFromRight returns the entry n from the right of the comma-separated
// list lines carry, walking back from the last line no further than that
// entry. It reports false when the lines hold fewer than n entries.
func entryFromRight(lines []string, n int) (string, bool) {
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]

		for {
			comma := strings.LastIndexByte(line, ',')

			n--
			if n == 0 {
				return line[comma+1:], true
			}

			if comma < 0 {
				break
			}

			line = line[:comma]
		}
	}

	return "", false
}

// hostOf is the host of a request's remote address, which carries a port
func hostOf(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}

	return host
}
