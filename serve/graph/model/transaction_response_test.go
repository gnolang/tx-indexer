package model

import (
	"testing"
	"unicode/utf8"

	abci "github.com/gnolang/gno/tm2/pkg/bft/abci/types"
	"github.com/stretchr/testify/assert"
)

func TestTransactionResponse_Data(t *testing.T) {
	t.Parallel()

	testTable := []struct {
		name     string
		data     []byte
		expected string
	}{
		{"empty", nil, ""},
		{"ascii", []byte("(\"hello\" string)\n\n"), "(\"hello\" string)\n\n"},
		{"multi-byte", []byte("héllo 世界 🙂"), "héllo 世界 🙂"},
		{"control bytes", []byte{0x00, 0x01, 0x1f}, "\x00\x01\x1f"},
		{"invalid run", []byte("ok \xff\xfe end"), "ok � end"},
		{"truncated rune", []byte("ab\xe4\xb8"), "ab�"},
		{"binary", []byte{0x80, 'a', 0xc0, 0xaf}, "�a�"},
	}

	for _, testCase := range testTable {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			tr := NewTransactionResponse(abci.ResponseDeliverTx{
				ResponseBase: abci.ResponseBase{Data: testCase.data},
			})

			got := tr.Data()

			assert.True(t, utf8.ValidString(got))
			assert.Equal(t, testCase.expected, got)
		})
	}
}
