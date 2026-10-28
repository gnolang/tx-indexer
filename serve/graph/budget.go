package graph

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/99designs/gqlgen/graphql"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gnolang/tx-indexer/serve/budget"
)

// WriteRefusal answers a GraphQL request refused before it was served with a
// 429 and err as its one error, in the shape the query deadline's takes: its
// message, and its details under extensions, with no data
func WriteRefusal(w http.ResponseWriter, err error) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)

	return json.NewEncoder(w).Encode(&graphql.Response{Errors: gqlerror.List{refusalError(err)}})
}

// refusalError is err as a GraphQL error, with the details of a time budget
// refusal under extensions
func refusalError(err error) *gqlerror.Error {
	refusal := &gqlerror.Error{Err: err, Message: err.Error()}

	var exhausted *budget.ExhaustedError
	if errors.As(err, &exhausted) {
		refusal.Extensions = exhausted.Details()
	}

	return refusal
}
