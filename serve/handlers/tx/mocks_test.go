package tx

import "github.com/gnolang/gno/tm2/pkg/bft/types"

type getTxDelegate func([]byte) (*types.TxResult, error)

type mockStorage struct {
	getTxFn getTxDelegate
}

func (m *mockStorage) GetTx(hash []byte) (*types.TxResult, error) {
	if m.getTxFn != nil {
		return m.getTxFn(hash)
	}

	return nil, nil
}
