package model

import (
	"encoding/base64"
	"encoding/json"
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
				Value:   makeBankMsgSend(message),
			}
		}
	case vm.RouterKey:
		switch message.Type() {
		case MessageTypeExec.String():
			contentMessage = &TransactionMessage{
				Route:   MessageRouteVM,
				TypeURL: MessageTypeExec,
				Value:   makeVMMsgCall(message),
			}
		case MessageTypeAddPackage.String():
			contentMessage = &TransactionMessage{
				Route:   MessageRouteVM,
				TypeURL: MessageTypeAddPackage,
				Value:   makeVMAddPackage(message),
			}
		case MessageTypeRun.String():
			contentMessage = &TransactionMessage{
				Route:   MessageRouteVM,
				TypeURL: MessageTypeRun,
				Value:   makeVMMsgRun(message),
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

func makeBankMsgSend(value std.Msg) BankMsgSend {
	decodedMessage, err := cast[bank.MsgSend](value)
	if err != nil {
		return BankMsgSend{}
	}

	return BankMsgSend{
		FromAddress: decodedMessage.FromAddress.String(),
		ToAddress:   decodedMessage.ToAddress.String(),
		Amount:      decodedMessage.Amount.String(),
	}
}

func makeVMMsgCall(value std.Msg) MsgCall {
	decodedMessage, err := cast[vm.MsgCall](value)
	if err != nil {
		return MsgCall{}
	}

	return MsgCall{
		Caller:  decodedMessage.Caller.String(),
		Send:    decodedMessage.Send.String(),
		PkgPath: decodedMessage.PkgPath,
		Func:    decodedMessage.Func,
		Args:    decodedMessage.Args,
	}
}

func makeVMAddPackage(value std.Msg) MsgAddPackage {
	decodedMessage, err := cast[vm.MsgAddPackage](value)
	if err != nil {
		return MsgAddPackage{}
	}

	memFiles := make([]*MemFile, 0)
	for _, file := range decodedMessage.Package.Files {
		memFiles = append(memFiles, &MemFile{
			Name: file.Name,
			Body: file.Body,
		})
	}

	return MsgAddPackage{
		Creator: decodedMessage.Creator.String(),
		Package: &MemPackage{
			Name:  decodedMessage.Package.Name,
			Path:  decodedMessage.Package.Path,
			Files: memFiles,
		},
		Deposit: decodedMessage.Deposit.String(),
	}
}

func makeVMMsgRun(value std.Msg) MsgRun {
	decodedMessage, err := cast[vm.MsgRun](value)
	if err != nil {
		return MsgRun{}
	}

	memFiles := make([]*MemFile, 0)
	for _, file := range decodedMessage.Package.Files {
		memFiles = append(memFiles, &MemFile{
			Name: file.Name,
			Body: file.Body,
		})
	}

	return MsgRun{
		Caller: decodedMessage.Caller.String(),
		Send:   decodedMessage.Send.String(),
		Package: &MemPackage{
			Name:  decodedMessage.Package.Name,
			Path:  decodedMessage.Package.Path,
			Files: memFiles,
		},
	}
}

func cast[T any](input any) (*T, error) {
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	var data T
	if err := json.Unmarshal(encoded, &data); err != nil {
		return nil, err
	}

	return &data, nil
}
