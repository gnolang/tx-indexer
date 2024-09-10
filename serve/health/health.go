package health

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"

	"github.com/gnolang/tx-indexer/storage"
)

func Setup(s storage.Storage, m *chi.Mux) *chi.Mux {
	m.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		h, err := s.GetLatestHeight()
		if err != nil {
			fmt.Fprintf(w, "ERROR: %s\n", err)
			render.Status(r, http.StatusInternalServerError)
			return
		}

		fmt.Fprintf(w, "Server is responding\n")
		fmt.Fprintf(w, "- Time: %s\n", time.Now())
		fmt.Fprintf(w, "- Latest Height: %d\n", h)
	})

	return m
}
