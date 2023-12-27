package tx

import (
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/tx-indexer/serve/metadata"
	"github.com/gnolang/tx-indexer/serve/spec"
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
	if len(params) < 1 {
		return nil, spec.GenerateInvalidParamCountError()
	}

	// Extract the params
	requestedTx, ok := params[0].(string)
	if !ok {
		return nil, spec.GenerateInvalidParamError(1)
	}

	// TODO check if there needs to be a decoding / encoding
	// step to the hash

	// Run the handler
	response, err := h.getTx([]byte(requestedTx))
	if err != nil {
		return nil, spec.GenerateResponseError(err)
	}

	return response, nil
}

// getTx fetches the tx from storage, if any
func (h *Handler) getTx(txHash []byte) (*types.TxResult, error) {
	tx, err := h.storage.GetTx(txHash)
	if err != nil {
		return nil, err
	}

	return tx, nil
}
