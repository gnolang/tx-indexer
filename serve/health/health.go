package health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"github.com/gnolang/tx-indexer/storage"
)

func Setup(s storage.Storage, rc ReadyChecker, m *chi.Mux) *chi.Mux {
	m.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		e := json.NewEncoder(w)

		h, err := s.GetLatestHeight()
		if err != nil {
			e.Encode(&response{
				Message: fmt.Sprintf("storage is not reachable: %s", err.Error()),
				Info: map[string]any{
					"time": fmt.Sprintf("%s", time.Now()),
				},
			})

			render.Status(r, http.StatusInternalServerError)

			return
		}

		e.Encode(&response{
			Message: "Server is responding",
			Info: map[string]any{
				"time":   fmt.Sprintf("%s", time.Now()),
				"height": h,
			},
		})
	})

	m.HandleFunc("/ready", func(w http.ResponseWriter, r *http.Request) {
		e := json.NewEncoder(w)

		ok, err := rc.IsReady()
		if !ok {
			e.Encode(&response{
				Message: fmt.Sprintf("node not ready: %s", err.Error()),
				Info: map[string]any{
					"time": fmt.Sprintf("%s", time.Now()),
				},
			})

			render.Status(r, http.StatusInternalServerError)

			return
		}

		e.Encode(&response{
			Message: "node is ready",
			Info: map[string]any{
				"time": fmt.Sprintf("%s", time.Now()),
			},
		})
	})

	return m
}

type response struct {
	Message string         `json:"message"`
	Info    map[string]any `json:"info"`
}

type ReadyChecker interface {
	IsReady() (bool, error)
}
