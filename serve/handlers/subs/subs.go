package subs

import (
	"fmt"
	"reflect"

	"github.com/gnolang/tx-indexer/serve/encode"
	"github.com/gnolang/tx-indexer/serve/filters"
	"github.com/gnolang/tx-indexer/serve/filters/filter"
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

// NewTransactionFilterHandler creates a transaction filter object
func (h *Handler) NewTransactionFilterHandler(
	_ *metadata.Metadata,
	params []any,
) (any, *spec.BaseJSONError) {
	// Check the params
	if len(params) < 1 {
		return nil, spec.GenerateInvalidParamCountError()
	}

	var options filter.TxFilterOption

	err := spec.ParseObjectParameter(params[0], &options)
	if err != nil {
		return nil, spec.GenerateInvalidParamError(1)
	}

	return h.newTxFilter(options), nil
}

func (h *Handler) newTxFilter(options filter.TxFilterOption) string {
	return h.filterManager.NewTxFilter(options)
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
	case subscription.NewTransactionsEvent:
		return h.filterManager.NewTransactionSubscription(conn), nil
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

	// Handle filter changes
	changes, err := h.getFilterChanges(f)
	if err != nil {
		return nil, spec.GenerateResponseError(err)
	}

	results := make([]string, len(changes))

	for index, changed := range changes {
		encodedResponse, encodeErr := encode.PrepareValue(changed)
		if encodeErr != nil {
			return nil, spec.GenerateResponseError(encodeErr)
		}

		results[index] = encodedResponse
	}

	return results, nil
}

func (h *Handler) getFilterChanges(filter filters.Filter) ([]any, error) {
	// Get updates
	changes := filter.GetChanges()
	value := reflect.ValueOf(changes)

	if value.Kind() == reflect.Ptr {
		value = value.Elem()
	}

	if value.Kind() != reflect.Slice {
		return nil, fmt.Errorf("forEachValue: expected slice type, found %q", value.Kind().String())
	}

	results := make([]any, value.Len())

	for i := 0; i < value.Len(); i++ {
		val := value.Index(i).Interface()
		results[i] = val
	}

	return results, nil
}
