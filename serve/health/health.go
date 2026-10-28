package health

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"github.com/gnolang/tx-indexer/storage"
)

// readinessTTL is how long /ready answers from one check of the node
const readinessTTL = time.Second

func Setup(s storage.Storage, rc ReadyChecker, m *chi.Mux) *chi.Mux {
	ready := &cachedReadiness{checker: rc, ttl: readinessTTL}

	m.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		h, err := s.GetLatestHeight()
		if err != nil {
			render.JSON(w, r, &response{
				Message: fmt.Sprintf("storage is not reachable: %s", err.Error()),
				Info: map[string]any{
					"time": time.Now().String(),
				},
			})

			render.Status(r, http.StatusInternalServerError)

			return
		}

		render.JSON(w, r, &response{
			Message: "Server is responding",
			Info: map[string]any{
				"time":   time.Now().String(),
				"height": h,
			},
		})
	})

	m.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		ok, err := ready.IsReady(r.Context())
		if !ok {
			render.JSON(w, r, &response{
				Message: fmt.Sprintf("node not ready: %s", err.Error()),
				Info: map[string]any{
					"time": time.Now().String(),
				},
			})
			render.Status(r, http.StatusInternalServerError)

			return
		}

		render.JSON(w, r, &response{
			Message: "node is ready",
			Info: map[string]any{
				"time": time.Now().String(),
			},
		})
	})

	return m
}

type response struct {
	Info    map[string]any `json:"info"`
	Message string         `json:"message"`
}

type ReadyChecker interface {
	IsReady(context.Context) (bool, error)
}

// cachedReadiness answers from one check of its checker for ttl. A check of
// the fetcher's costs the node a status RPC, and no client limit refuses
// /ready: however many requests ask, the node is checked at most once a ttl.
type cachedReadiness struct {
	checker ReadyChecker

	// checked is when the answer was checked, zero before the first check
	checked time.Time

	// err is the answer's error, if the node was not ready
	err error

	ttl time.Duration

	// mu holds the answer, and the check while it runs: requests that arrive
	// meanwhile wait for its answer rather than check again
	mu sync.Mutex

	ready bool
}

// IsReady answers the last check, made within ttl, or checks again. The check
// outlives a request that leaves while it runs: its answer serves the others.
func (c *cachedReadiness) IsReady(ctx context.Context) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.checked.IsZero() && time.Since(c.checked) < c.ttl {
		return c.ready, c.err
	}

	c.ready, c.err = c.checker.IsReady(context.WithoutCancel(ctx))
	c.checked = time.Now()

	return c.ready, c.err
}
