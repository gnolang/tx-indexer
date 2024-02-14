package tx

import (
	"errors"

	"github.com/gnolang/gno/tm2/pkg/bft/types"

	"github.com/gnolang/tx-indexer/serve/encode"
	"github.com/gnolang/tx-indexer/serve/metadata"
	"github.com/gnolang/tx-indexer/serve/spec"
	storageErrors "github.com/gnolang/tx-indexer/storage/errors"
)

type Handler struct {
	storage Storage
}

func NewHandler(storage Storage) *Handler {
	return &Handler{
		storage: storage,
	}
}

func (h *Handler) GetTxHandler(
	_ *metadata.Metadata,
	params []any,
) (any, *spec.BaseJSONError) {
	// Check the params
	if len(params) < 2 {
		return nil, spec.GenerateInvalidParamCountError()
	}

	// Extract the params
	blockNum, ok := params[0].(int64)
	if !ok {
		return nil, spec.GenerateInvalidParamError(1)
	}

	txIndex, ok := params[1].(uint32)
	if !ok {
		return nil, spec.GenerateInvalidParamError(1)
	}

	// Run the handler
	response, err := h.getTx(blockNum, txIndex)
	if err != nil {
		return nil, spec.GenerateResponseError(err)
	}

	if response == nil {
		return nil, nil
	}

	encodedResponse, err := encode.PrepareValue(response)
	if err != nil {
		return nil, spec.GenerateResponseError(err)
	}

	return encodedResponse, nil
}

// getTx fetches the tx from storage, if any
func (h *Handler) getTx(blockNum int64, txIndex uint32) (*types.TxResult, error) {
	tx, err := h.storage.GetTx(blockNum, txIndex)
	if errors.Is(err, storageErrors.ErrNotFound) {
		// Wrap the error
		//nolint:nilnil // This is a special case
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return tx, nil
}
