package gas

import (
	"fmt"
	"strconv"

	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/vektah/gqlparser/v2/gqlerror"

	"github.com/gnolang/tx-indexer/serve/metadata"
	"github.com/gnolang/tx-indexer/serve/methods"
	"github.com/gnolang/tx-indexer/serve/spec"
)

const DefaultBlockRangeSize = 1_000

type Handler struct {
	storage Storage
}

func NewHandler(storage Storage) *Handler {
	return &Handler{
		storage: storage,
	}
}

func (h *Handler) GetGasPriceHandler(
	_ *metadata.Metadata,
	params []any,
) (any, *spec.BaseJSONError) {
	// Check the params
	if len(params) != 0 && len(params) != 2 {
		return nil, spec.GenerateInvalidParamCountError()
	}

	var toBlockNum, fromBlockNum uint64

	if len(params) == 0 {
		latestHeight, err := h.storage.GetLatestHeight()
		if err != nil {
			return nil, spec.GenerateResponseError(err)
		}

		fromBlockNum, toBlockNum = initializeDefaultBlockRangeByHeight(latestHeight)
	} else {
		fromBlockNum, toBlockNum = parseBlockRangeByParams(params)
	}

	response, err := h.getGasPriceBy(fromBlockNum, toBlockNum)
	if err != nil {
		return nil, spec.GenerateResponseError(err)
	}

	return response, nil
}

func (h *Handler) getGasPriceBy(fromBlockNum, toBlockNum uint64) ([]*methods.GasPrice, error) {
	it, err := h.
		storage.
		BlockIterator(
			fromBlockNum,
			toBlockNum,
		)
	if err != nil {
		return nil, gqlerror.Wrap(err)
	}

	defer it.Close()

	blocks := make([]*types.Block, 0)

	for it.Next() {
		block, itErr := it.Value()
		if itErr != nil {
			return nil, err
		}

		blocks = append(blocks, block)
	}

	gasPrices, err := methods.GetGasPricesByBlocks(blocks)
	if err != nil {
		return nil, err
	}

	return gasPrices, nil
}

func initializeDefaultBlockRangeByHeight(latestHeight uint64) (uint64, uint64) {
	toBlockNum := latestHeight

	var fromBlockNum uint64

	if latestHeight > DefaultBlockRangeSize {
		fromBlockNum = latestHeight - DefaultBlockRangeSize
	}

	return fromBlockNum, toBlockNum
}

func parseBlockRangeByParams(params []any) (uint64, uint64) {
	fromBlockNum, err := strconv.ParseUint(fmt.Sprintf("%v", params[0]), 10, 64)
	if err != nil {
		fromBlockNum = 0
	}

	toBlockNum, err := strconv.ParseUint(fmt.Sprintf("%v", params[1]), 10, 64)
	if err != nil {
		toBlockNum = 0
	}

	return fromBlockNum, toBlockNum
}
