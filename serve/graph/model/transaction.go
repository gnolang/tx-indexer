package model

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gnolang/gno/gno.land/pkg/sdk/vm"
	"github.com/gnolang/gno/tm2/pkg/amino"
	abci "github.com/gnolang/gno/tm2/pkg/bft/abci/types"
	"github.com/gnolang/gno/tm2/pkg/bft/types"
	"github.com/gnolang/gno/tm2/pkg/sdk/bank"
	"github.com/gnolang/gno/tm2/pkg/std"
)

type Transaction struct {
	stdTx    *std.Tx
	txResult *types.TxResult
	messages []*TransactionMessage

	mu           sync.Mutex
	onceTx       sync.Once
	onceMessages sync.Once
}

func NewTransaction(txResult *types.TxResult) *Transaction {
	return &Transaction{
		txResult: txResult,
		messages: make([]*TransactionMessage, 0),
	}
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

func (t *Transaction) Response() TransactionResponse {
	return TransactionResponse{
		response: t.txResult.Response,
	}
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
	if t.getStdTx() == nil {
		return ""
	}

	return t.getStdTx().GetMemo()
}

func (t *Transaction) Messages() []*TransactionMessage {
	return t.getMessages()
}

func (t *Transaction) Fee() *TxFee {
	return &TxFee{
		GasWanted: t.GasWanted(),
		GasFee:    t.GasUsed(),
	}
}

func (t *Transaction) getStdTx() *std.Tx {
	// The function to unmarshal a `std.Tx` is executed once.
	unmarshalTx := func() {
		var stdTx std.Tx
		if err := amino.Unmarshal(t.txResult.Tx, &stdTx); err != nil {
			t.stdTx = nil
		}

		t.mu.Lock()
		t.stdTx = &stdTx
		t.mu.Unlock()
	}

	t.onceTx.Do(unmarshalTx)

	return t.stdTx
}

func (t *Transaction) getMessages() []*TransactionMessage {
	// Functions that unmarshal transaction messages are executed once.
	unmarshalMessages := func() {
		stdTx := t.getStdTx()
		messages := make([]*TransactionMessage, 0)

		for _, message := range stdTx.GetMsgs() {
			messages = append(messages, NewTransactionMessage(message))
		}

		t.mu.Lock()
		t.messages = messages
		t.mu.Unlock()
	}

	t.onceMessages.Do(unmarshalMessages)

	return t.messages
}

//nolint:errname // Provide a field named `error` as the GraphQL response value
type TransactionResponse struct {
	response abci.ResponseDeliverTx
}

func (tr *TransactionResponse) Log() string {
	return tr.response.Log
}

func (tr *TransactionResponse) Info() string {
	return tr.response.Info
}

func (tr *TransactionResponse) Error() string {
	if tr.response.IsErr() {
		return tr.response.Error.Error()
	}

	return ""
}

func (tr *TransactionResponse) Data() string {
	return string(tr.response.Data)
}

type TransactionMessage struct {
	Value   MessageValue
	Route   string
	TypeURL string
}

func NewTransactionMessage(message std.Msg) *TransactionMessage {
	var contentMessage *TransactionMessage

	switch message.Route() {
	case bank.RouterKey:
		if message.Type() == MessageTypeSend.String() {
			contentMessage = &TransactionMessage{
				Route:   MessageRouteBank.String(),
				TypeURL: MessageTypeSend.String(),
				Value:   makeBankMsgSend(message),
			}
		}
	case vm.RouterKey:
		switch message.Type() {
		case MessageTypeExec.String():
			contentMessage = &TransactionMessage{
				Route:   MessageRouteVM.String(),
				TypeURL: MessageTypeExec.String(),
				Value:   makeVMMsgCall(message),
			}
		case MessageTypeAddPackage.String():
			contentMessage = &TransactionMessage{
				Route:   MessageRouteVM.String(),
				TypeURL: MessageTypeAddPackage.String(),
				Value:   makeVMAddPackage(message),
			}
		case MessageTypeRun.String():
			contentMessage = &TransactionMessage{
				Route:   MessageRouteVM.String(),
				TypeURL: MessageTypeRun.String(),
				Value:   makeVMMsgRun(message),
			}
		}
	}

	if contentMessage == nil {
		contentMessage = &TransactionMessage{
			Route:   message.Route(),
			TypeURL: message.Type(),
			Value:   makeUnexpectedMessage(message),
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

func makeUnexpectedMessage(value std.Msg) UnexpectedMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		return UnexpectedMessage{
			Raw: "",
		}
	}

	return UnexpectedMessage{
		Raw: string(raw),
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
