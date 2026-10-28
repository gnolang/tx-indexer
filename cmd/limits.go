package main

import (
	"net/http"
	"net/netip"

	"github.com/gnolang/tx-indexer/serve/observe"
)

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
