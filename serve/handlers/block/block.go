package block

import (
	"errors"
	"strconv"

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

func (h *Handler) GetBlockHandler(
	_ *metadata.Metadata,
	params []any,
) (any, *spec.BaseJSONError) {
	// Check the params
	if len(params) != 1 {
		return nil, spec.GenerateInvalidParamCountError()
	}

	// Extract the params
	requestedBlock, ok := params[0].(string)
	if !ok {
		return nil, spec.GenerateInvalidParamError(1)
	}

	blockNum, err := strconv.ParseInt(requestedBlock, 10, 64)
	if err != nil {
		return nil, spec.GenerateInvalidParamError(1)
	}

	// Run the handler
	response, err := h.getBlock(blockNum)
	if err != nil {
		return nil, spec.GenerateResponseError(err)
	}

	if response == nil {
		return nil, nil
	}

	encodedResponse, err := encode.EncodeValue(response)
	if err != nil {
		return nil, spec.GenerateResponseError(err)
	}

	return encodedResponse, nil
}

// getBlock fetches the block from storage, if any
func (h *Handler) getBlock(blockNum int64) (*types.Block, error) {
	block, err := h.storage.GetBlock(blockNum)
	if errors.Is(err, storageErrors.ErrNotFound) {
		// Wrap the error
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	return block, nil
}
