package subs

import (
	"fmt"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/serve/encode"
	"github.com/gnolang/tx-indexer/serve/filters"
	"github.com/gnolang/tx-indexer/serve/filters/subscription"
	"github.com/gnolang/tx-indexer/serve/metadata"
	"github.com/gnolang/tx-indexer/serve/spec"
)

type Handler struct {
	connFetcher ConnectionFetcher

	filterManager *filters.Manager
}

func NewHandler(
	filterManager *filters.Manager,
	conns ConnectionFetcher,
) *Handler {
	return &Handler{
		connFetcher:   conns,
		filterManager: filterManager,
	}
}

// NewBlockFilterHandler creates a block filter object
func (h *Handler) NewBlockFilterHandler(
	_ *metadata.Metadata,
	params []any,
) (any, *spec.BaseJSONError) {
	// Check the params
	if len(params) != 0 {
		return nil, spec.GenerateInvalidParamCountError()
	}

	return h.newBlockFilter(), nil
}

func (h *Handler) newBlockFilter() string {
	return h.filterManager.NewBlockFilter()
}

// UninstallFilterHandler uninstalls a filter with given id
func (h *Handler) UninstallFilterHandler(
	_ *metadata.Metadata,
	params []any,
) (any, *spec.BaseJSONError) {
	// Check the params
	if len(params) != 1 {
		return nil, spec.GenerateInvalidParamCountError()
	}

	// Extract the params
	id, ok := params[0].(string)
	if !ok {
		return nil, spec.GenerateInvalidParamError(1)
	}

	return h.uninstallFilter(id), nil
}

func (h *Handler) uninstallFilter(id string) bool {
	return h.filterManager.UninstallFilter(id)
}

func (h *Handler) SubscribeHandler(
	metadata *metadata.Metadata,
	params []any,
) (any, *spec.BaseJSONError) {
	// This method can only be called through a WS connection
	if !metadata.IsWS() {
		return nil, spec.NewJSONError(
			"Method only supported over WS",
			spec.ServerErrorCode,
		)
	}

	// Check the params
	if len(params) == 0 {
		return nil, spec.GenerateInvalidParamCountError()
	}

	// Extract the params
	eventType, ok := params[0].(string)
	if !ok {
		return nil, spec.GenerateInvalidParamError(1)
	}

	subscriptionID, err := h.subscribe(*metadata.WebSocketID, eventType)
	if err != nil {
		return nil, spec.NewJSONError(
			fmt.Sprintf("unable to subscribe, %s", err.Error()),
			spec.ServerErrorCode,
		)
	}

	return subscriptionID, nil
}

func (h *Handler) subscribe(connID, eventType string) (string, error) {
	conn := h.connFetcher.GetWSConnection(connID)
	if conn == nil {
		return "", fmt.Errorf("WS connection with ID %s not found", connID)
	}

	switch eventType {
	case subscription.NewHeadsEvent:
		return h.filterManager.NewBlockSubscription(conn), nil
	default:
		return "", fmt.Errorf("invalid event type: %s", eventType)
	}
}

func (h *Handler) UnsubscribeHandler(
	metadata *metadata.Metadata,
	params []any,
) (any, *spec.BaseJSONError) {
	// This method can only be called through a WS connection
	if !metadata.IsWS() {
		return nil, spec.NewJSONError(
			"Method only supported over WS",
			spec.ServerErrorCode,
		)
	}

	// Check the params
	if len(params) != 1 {
		return nil, spec.GenerateInvalidParamCountError()
	}

	// Extract the params
	subscriptionID, ok := params[0].(string)
	if !ok {
		return nil, spec.GenerateInvalidParamError(1)
	}

	return h.unsubscribe(subscriptionID), nil
}

func (h *Handler) unsubscribe(subscriptionID string) bool {
	return h.filterManager.UninstallSubscription(subscriptionID)
}

// GetFilterChangesHandler returns recent changes for a specified filter
func (h *Handler) GetFilterChangesHandler(_ *metadata.Metadata, params []any) (any, *spec.BaseJSONError) {
	// Check the params
	if len(params) != 1 {
		return nil, spec.GenerateInvalidParamCountError()
	}

	// Extract the params
	id, ok := params[0].(string)
	if !ok {
		return nil, spec.GenerateInvalidParamError(1)
	}

	// Get filter by id
	f, err := h.filterManager.GetFilter(id)
	if err != nil {
		return nil, spec.GenerateResponseError(err)
	}

	// Handle block filter changes
	changes := h.getBlockChanges(f)

	// Encode the response
	encodedResponses := make([]string, len(changes))

	for index, change := range changes {
		encodedResponse, encodeErr := encode.PrepareValue(change)
		if encodeErr != nil {
			return nil, spec.GenerateResponseError(encodeErr)
		}

		encodedResponses[index] = encodedResponse
	}

	return encodedResponses, nil
}

func (h *Handler) getBlockChanges(filter filters.Filter) []types.Header {
	// Get updates
	blockHeaders, _ := filter.GetChanges().([]types.Header)

	return blockHeaders
}
