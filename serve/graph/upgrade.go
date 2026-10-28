package graph

import (
	"bufio"
	"net"
	"net/http"
)

// hijackAwareWriter is the response to a request carrying an Upgrade header.
// gorilla's upgrader can fail after it has taken the connection from the HTTP
// server, as it does for a client that sends data before the handshake
// completes, and gqlgen still answers that failure: the HTTP server would log
// the answer at error, as a write on a hijacked connection, which any client
// could cause. Once the connection is taken, the writer drops what is written.
// gqlgen also writes its status after gorilla's, for an upgrade refused before
// the connection is taken: the writer keeps the first and drops the second,
// which the HTTP server would log at error as superfluous.
type hijackAwareWriter struct {
	http.ResponseWriter

	// hijacked reports whether the connection was taken
	hijacked bool

	// answered reports whether the response's status was written
	answered bool
}

// gorilla's upgrader takes the connection through http.Hijacker, by a type
// assertion
var _ http.Hijacker = (*hijackAwareWriter)(nil)

// Hijack takes the connection from the HTTP server, and marks it taken once
// it is
func (w *hijackAwareWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil {
		w.hijacked = true
	}

	return conn, rw, err
}

// WriteHeader writes the status, unless the connection was taken or the
// response has its status already. An informational status, which another
// follows, leaves the response without one, as it does in the HTTP server.
func (w *hijackAwareWriter) WriteHeader(code int) {
	if w.hijacked || w.answered {
		return
	}

	informational := code >= 100 && code < http.StatusOK && code != http.StatusSwitchingProtocols
	w.answered = !informational

	w.ResponseWriter.WriteHeader(code)
}

// Write writes b, unless the connection was taken: it then fails with
// http.ErrHijacked, as the HTTP server's own response does
func (w *hijackAwareWriter) Write(b []byte) (int, error) {
	if w.hijacked {
		return 0, http.ErrHijacked
	}

	// A write without a status answers 200
	w.answered = true

	return w.ResponseWriter.Write(b)
}

// Unwrap is the response the writer wraps, for http.ResponseController
func (w *hijackAwareWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// hijackAware hands next the response to a request carrying an Upgrade
// header, the requests gqlgen's WebSocket transport takes, as a
// hijackAwareWriter
func hijackAware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "" {
			w = &hijackAwareWriter{ResponseWriter: w}
		}

		next.ServeHTTP(w, r)
	})
}
