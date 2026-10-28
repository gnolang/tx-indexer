//go:build !race

package graph

import (
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/gnolang/tx-indexer/events"
)

// The widest conflicts under the default token limit, 1,999 fields and
// 999,000 errors of one rule, are answered the first 100 and one saying the
// rest are left out. Their validation is the slowest of the package's tests,
// about nine times slower under the race detector, which leaves it out.
func TestValidation_CapsTheWidestConflictsUnderTheDefaultLimit(t *testing.T) {
	t.Parallel()

	mux := Setup(&slowStore{}, events.NewManager(), nil, chi.NewMux(), Options{TokenLimit: 4000})

	requireCapped(t, mux, conflicts(1999), "OverlappingFieldsCanBeMerged", conflictMessage)
}
