package tx

import "github.com/gnolang/gno/tm2/pkg/bft/types"

type getTxDelegate func(int64, uint32) (*types.TxResult, error)

type mockStorage struct {
	getTxFn getTxDelegate
}

func (m *mockStorage) GetTx(bn int64, ti uint32) (*types.TxResult, error) {
	if m.getTxFn != nil {
		return m.getTxFn(bn, ti)
	}

	return nil, nil
}
