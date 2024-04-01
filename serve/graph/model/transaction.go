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
	memo     string
	txResult *types.TxResult
	messages []*TransactionMessage
}

func NewTransaction(txResult *types.TxResult) *Transaction {
	var stdTx std.Tx

	if err := amino.Unmarshal(txResult.Tx, &stdTx); err != nil {
		return nil
	}

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
	Value   MessageValue
	TypeURL MessageType
	Route   MessageRoute
}

func NewTransactionMessage(message std.Msg) *TransactionMessage {
	var contentMessage *TransactionMessage

	switch message.Route() {
	case bank.RouterKey:
		if message.Type() == MessageTypeSend.String() {
			contentMessage = &TransactionMessage{
				Route:   MessageRouteBank,
				TypeURL: MessageTypeSend,
				Value:   ParseBankMsgSend(message),
			}
		}
	case vm.RouterKey:
		switch message.Type() {
		case MessageTypeExec.String():
			contentMessage = &TransactionMessage{
				Route:   MessageRouteVM,
				TypeURL: MessageTypeExec,
				Value:   ParseVMMsgCall(message),
			}
		case MessageTypeAddPackage.String():
			contentMessage = &TransactionMessage{
				Route:   MessageRouteVM,
				TypeURL: MessageTypeAddPackage,
				Value:   ParseVMAddPackage(message),
			}
		case MessageTypeRun.String():
			contentMessage = &TransactionMessage{
				Route:   MessageRouteVM,
				TypeURL: MessageTypeRun,
				Value:   ParseVMMsgRun(message),
			}
		}
	}

	return contentMessage
}

func (tm *TransactionMessage) BankMsgSend() BankMsgSend {
	return tm.Value.(BankMsgSend)
}

func (tm *TransactionMessage) VMMsgCall() MsgCall {
	return tm.Value.(MsgCall)
}

func (tm *TransactionMessage) VMAddPackage() MsgAddPackage {
	return tm.Value.(MsgAddPackage)
}

func (tm *TransactionMessage) VMMsgRun() MsgRun {
	return tm.Value.(MsgRun)
}
