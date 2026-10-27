package spec

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gnolang/tx-indexer/serve/deadline"
)

const (
	JSONRPCVersion     = "2.0"
	SubscriptionMethod = "subscription"
)

// BaseJSON defines the base JSON fields
// all JSON-RPC requests and responses need to have
type BaseJSON struct {
	JSONRPC string `json:"jsonrpc"`
	ID      uint   `json:"id,omitempty"`
}

// BaseJSONRequest defines the base JSON request format
type BaseJSONRequest struct {
	BaseJSON

	Method string `json:"method"`
	Params []any  `json:"params"`
}

// BaseJSONRequests represents a batch of JSON-RPC requests
type BaseJSONRequests []*BaseJSONRequest

// BaseJSONResponses represents a batch of JSON-RPC responses
type BaseJSONResponses []*BaseJSONResponse

// BaseJSONResponse defines the base JSON response format
type BaseJSONResponse struct {
	Result any            `json:"result"`
	Error  *BaseJSONError `json:"error,omitempty"`
	BaseJSON
}

// BaseJSONSubscribeResponse defines the base JSON response
// format for subscribe responses
type BaseJSONSubscribeResponse struct {
	Params  *SubscribeResponse `json:"params"`
	JSONRPC string             `json:"jsonrpc"`
	Method  string             `json:"method"`
}

// SubscribeResponse defines the subscribe response data
type SubscribeResponse struct {
	Result       any    `json:"result,omitempty"`
	Subscription string `json:"subscription"`
}

// BaseJSONError defines the base JSON response error format. It is an error
// itself, wrapping the error it was made from, if any, so the error a call is
// answered with can be inspected with errors.Is and errors.As.
type BaseJSONError struct {
	Data any `json:"data,omitempty"`

	// cause is the error GenerateResponseError made the answer from
	cause error

	Message string `json:"message"`
	Code    int    `json:"code"`
}

// Error is the message the call is answered with
func (e *BaseJSONError) Error() string {
	return e.Message
}

// Unwrap is the error the answer was made from, nil when it was made from a
// message alone
func (e *BaseJSONError) Unwrap() error {
	return e.cause
}

// NewJSONRequest creates a new JSON-RPC request
func NewJSONRequest(
	id uint,
	method string,
	params []any,
) *BaseJSONRequest {
	return &BaseJSONRequest{
		BaseJSON: BaseJSON{
			ID:      id,
			JSONRPC: JSONRPCVersion,
		},
		Method: method,
		Params: params,
	}
}

// NewJSONResponse creates a new JSON-RPC response
func NewJSONResponse(
	id uint,
	result any,
	err *BaseJSONError,
) *BaseJSONResponse {
	return &BaseJSONResponse{
		BaseJSON: BaseJSON{
			ID:      id,
			JSONRPC: JSONRPCVersion,
		},
		Result: result,
		Error:  err,
	}
}

// NewJSONError creates a new JSON-RPC error
func NewJSONError(message string, code int) *BaseJSONError {
	return &BaseJSONError{
		Code:    code,
		Message: message,
	}
}

// NewJSONSubscribeResponse creates a new JSON-RPC response for the subscribe responses.
// These WS responses (for this method only) are different from any other response
func NewJSONSubscribeResponse(id string, result any) *BaseJSONSubscribeResponse {
	return &BaseJSONSubscribeResponse{
		JSONRPC: JSONRPCVersion,
		Method:  SubscriptionMethod,
		Params: &SubscribeResponse{
			Result:       result,
			Subscription: id,
		},
	}
}

// GenerateResponseError generates the JSON-RPC server error response. An
// error past the query deadline carries its machine-readable details in Data.
func GenerateResponseError(err error) *BaseJSONError {
	jsonErr := NewJSONError(err.Error(), ServerErrorCode)
	jsonErr.cause = err

	var exceeded *deadline.ExceededError
	if errors.As(err, &exceeded) {
		jsonErr.Data = exceeded.Details()
	}

	return jsonErr
}

// GenerateInvalidParamError generates the JSON-RPC invalid param error response
func GenerateInvalidParamError(index int) *BaseJSONError {
	return NewJSONError(
		fmt.Sprintf(
			"Invalid %s parameter",
			getOrdinalSuffix(index),
		),
		InvalidParamsErrorCode,
	)
}

func getOrdinalSuffix(num int) string {
	switch num % 10 {
	case 1:
		if num%100 != 11 {
			return fmt.Sprintf("%d%s", num, "st")
		}
	case 2:
		if num%100 != 12 {
			return fmt.Sprintf("%d%s", num, "nd")
		}
	case 3:
		if num%100 != 13 {
			return fmt.Sprintf("%d%s", num, "rd")
		}
	}

	return fmt.Sprintf("%d%s", num, "th")
}

// GenerateInvalidParamCountError generates the JSON-RPC invalid param count error
func GenerateInvalidParamCountError() *BaseJSONError {
	return NewJSONError(
		"Invalid number of parameters",
		InvalidParamsErrorCode,
	)
}

func ParseObjectParameter[T any](param any, data *T) error {
	marshaled, err := json.Marshal(param)
	if err != nil {
		return err
	}

	err = json.Unmarshal(marshaled, data)
	if err != nil {
		return err
	}

	return nil
}
