package model

import (
	"encoding/base64"
	"fmt"

	"github.com/gnolang/gno/gno.land/pkg/sdk/vm"
	"github.com/gnolang/gno/tm2/pkg/amino"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/sdk/bank"
	"github.com/gnolang/gno/tm2/pkg/std"
)

type Transaction struct {
	txResult *types.TxResult
	messages []*TransactionMessage
	memo     string
}

func NewTransaction(txResult *types.TxResult) *Transaction {
	var stdTx std.Tx
	amino.Unmarshal(txResult.Tx, &stdTx)

	messages := make([]*TransactionMessage, 0)
	for _, message := range stdTx.GetMsgs() {
		messages = append(messages, NewTransactionMessage(message))
	}
	return &Transaction{txResult: txResult, messages: messages, memo: stdTx.GetMemo()}
}

func (t *Transaction) ID() string {
	return fmt.Sprintf("%d_%d", t.txResult.Height, t.txResult.Index)
}

func (t *Transaction) Index() int {
	return int(t.txResult.Index)
}

func (t *Transaction) Hash() string {
	return base64.StdEncoding.EncodeToString(t.txResult.Tx.Hash())
}

func (t *Transaction) BlockHeight() int {
	return int(t.txResult.Height)
}

func (t *Transaction) GasWanted() int {
	return int(t.txResult.Response.GasWanted)
}

func (t *Transaction) GasUsed() int {
	return int(t.txResult.Response.GasUsed)
}

func (t *Transaction) ContentRaw() string {
	return t.txResult.Tx.String()
}

func (t *Transaction) Memo() string {
	return t.memo
}

func (t *Transaction) Messages() []*TransactionMessage {
	return t.messages
}

func (t *Transaction) Fee() *TxFee {
	return &TxFee{
		GasWanted: t.GasWanted(),
		GasFee:    t.GasUsed(),
	}
}

type TransactionMessage struct {
	TypeUrl MessageType
	Route   MessageRoute
	Value   MessageValue
}

func NewTransactionMessage(message std.Msg) *TransactionMessage {
	var contentMessage *TransactionMessage

	switch message.Route() {
	case bank.RouterKey:
		if message.Type() == MessageTypeSend.String() {
			contentMessage = &TransactionMessage{
				Route:   MessageRouteBank,
				TypeUrl: MessageTypeSend,
				Value:   ParseBankMsgSend(message),
			}
		}
	case vm.RouterKey:
		if message.Type() == MessageTypeExec.String() {
			contentMessage = &TransactionMessage{
				Route:   MessageRouteVM,
				TypeUrl: MessageTypeExec,
				Value:   ParseVmMsgCall(message),
			}
		}
		if message.Type() == MessageTypeAddPackage.String() {
			contentMessage = &TransactionMessage{
				Route:   MessageRouteVM,
				TypeUrl: MessageTypeAddPackage,
				Value:   ParseVmAddPackage(message),
			}
		}
		if message.Type() == MessageTypeRun.String() {
			contentMessage = &TransactionMessage{
				Route:   MessageRouteVM,
				TypeUrl: MessageTypeRun,
				Value:   ParseVmMsgRun(message),
			}
		}
	}

	return contentMessage
}

func (tm *TransactionMessage) BankMsgSend() BankMsgSend {
	return tm.Value.(BankMsgSend)
}

func (tm *TransactionMessage) VmMsgCall() MsgCall {
	return tm.Value.(MsgCall)
}

func (tm *TransactionMessage) VmAddPackage() MsgAddPackage {
	return tm.Value.(MsgAddPackage)
}

func (tm *TransactionMessage) VmMsgRun() MsgRun {
	return tm.Value.(MsgRun)
}
