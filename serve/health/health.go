package health

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"github.com/gnolang/tx-indexer/storage"
)

func Setup(s storage.Storage, rc ReadyChecker, m *chi.Mux) *chi.Mux {
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
		ok, err := rc.IsReady(r.Context())
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
