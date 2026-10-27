package observe

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The client address is the address the request came from, unless a header
// is trusted to name it: then it is the entry as many hops from the right of
// the header's list as there are trusted proxies appending to it. The entries
// further left are the client's own to forge, and are never read.
func TestClientAddress(t *testing.T) {
	t.Parallel()

	const peer = "10.0.0.7:41234"

	testTable := []struct {
		name        string
		header      string
		want        string
		values      []string
		trustedHops int
	}{
		{"no header trusted", "", "10.0.0.7", nil, 1},
		{"no header trusted, one sent", "", "10.0.0.7", []string{"203.0.113.9"}, 1},
		{"a single-value header", "CF-Connecting-IP", "203.0.113.9", []string{"203.0.113.9"}, 1},
		{"one proxy", "X-Forwarded-For", "203.0.113.9", []string{"203.0.113.9"}, 1},
		{"one proxy, spoofed entries", "X-Forwarded-For", "203.0.113.9", []string{"6.6.6.6, 7.7.7.7, 203.0.113.9"}, 1},
		{"two proxies", "X-Forwarded-For", "203.0.113.9", []string{"6.6.6.6, 203.0.113.9, 198.51.100.4"}, 2},
		{"split over header lines", "X-Forwarded-For", "203.0.113.9", []string{"6.6.6.6", "203.0.113.9"}, 1},
		{"an IPv6 client", "X-Forwarded-For", "2001:db8::1", []string{"2001:db8::1"}, 1},
		{"fewer entries than proxies", "X-Forwarded-For", "10.0.0.7", []string{"203.0.113.9"}, 2},
		{"the header missing", "X-Forwarded-For", "10.0.0.7", nil, 1},
		{"an entry that is no address", "X-Forwarded-For", "10.0.0.7", []string{"unknown"}, 1},
		{"an empty entry", "X-Forwarded-For", "10.0.0.7", []string{"203.0.113.9, "}, 1},
		{"no trusted proxy", "X-Forwarded-For", "10.0.0.7", []string{"203.0.113.9"}, 0},
		{"two proxies over two lines", "X-Forwarded-For", "203.0.113.9", []string{"6.6.6.6, 203.0.113.9", "198.51.100.4"}, 2},
		{"three lines", "X-Forwarded-For", "203.0.113.9", []string{"203.0.113.9", "198.51.100.4", "198.51.100.5"}, 3},
		{"an empty line between entries", "X-Forwarded-For", "10.0.0.7", []string{"203.0.113.9", "", "198.51.100.4"}, 2},
		{"an IPv6 client with a zone", "X-Forwarded-For", "fe80::1", []string{"fe80::1%eth0"}, 1},
		{"a zone carrying text", "X-Forwarded-For", "2001:db8::1", []string{"2001:db8::1%<forged log text>"}, 1},
		{"an entry with a port", "X-Forwarded-For", "203.0.113.7", []string{"203.0.113.7:51234"}, 1},
		{"an IPv6 entry with a port", "X-Forwarded-For", "2001:db8::1", []string{"[2001:db8::1]:443"}, 1},
		{"an IPv6 entry with a zone and a port", "X-Forwarded-For", "fe80::1", []string{"[fe80::1%eth0]:443"}, 1},
		{"a port behind spoofed entries", "X-Forwarded-For", "203.0.113.7", []string{"6.6.6.6:1, 203.0.113.7:51234"}, 1},
		{"a port with no address", "X-Forwarded-For", "10.0.0.7", []string{":51234"}, 1},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			r := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
			r.RemoteAddr = peer

			for _, value := range testCase.values {
				r.Header.Add(testCase.header, value)
			}

			// A client sends whatever it likes: a header no proxy is trusted
			// for is never read
			r.Header.Add("X-Real-IP", "6.6.6.6")

			assert.Equal(t, testCase.want, clientAddress(r, testCase.header, testCase.trustedHops))
		})
	}
}

// The address is read from the right of the header, as far as the trusted
// entries reach: the entries a client forges to their left are never split,
// however many it sends
func TestClientAddress_ReadsNoFurtherThanTheTrustedEntries(t *testing.T) {
	// Not parallel: it reads the allocation total of the whole process
	r := httptest.NewRequest(http.MethodPost, "/", http.NoBody)
	r.RemoteAddr = "10.0.0.7:41234"
	r.Header.Set("X-Forwarded-For", strings.Repeat(",", 1<<20)+"203.0.113.9")

	var before, after runtime.MemStats

	runtime.ReadMemStats(&before)

	client := clientAddress(r, "X-Forwarded-For", 1)

	runtime.ReadMemStats(&after)

	assert.Equal(t, "203.0.113.9", client)
	assert.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(64<<10), "bytes allocated")
}
