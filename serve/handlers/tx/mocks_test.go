package tx

import "github.com/gnolang/gno/tm2/pkg/bft/types"

type getTxDelegate func(uint64, uint32) (*types.TxResult, error)

type getTxHashDelegate func(string) (*types.TxResult, error)

type mockStorage struct {
	getTxFn     getTxDelegate
	getTxHashFn getTxHashDelegate
}

func (m *mockStorage) GetTx(bn uint64, ti uint32) (*types.TxResult, error) {
	if m.getTxFn != nil {
		return m.getTxFn(bn, ti)
	}

	return nil, nil
}

func (m *mockStorage) GetTxByHash(h string) (*types.TxResult, error) {
	if m.getTxHashFn != nil {
		return m.getTxHashFn(h)
	}

	return nil, nil
}
