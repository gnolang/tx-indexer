// Code generated by github.com/99designs/gqlgen, DO NOT EDIT.

package model

import (
	"fmt"
	"io"
	"strconv"
	"time"
)

type Event interface {
	IsEvent()
}

type MessageValue interface {
	IsMessageValue()
}

// `AmountInput` is a range of token quantities to filter by.
type AmountInput struct {
	// The minimum quantity of tokens to check for.
	From *int `json:"from,omitempty"`
	// The maximum quantity of tokens to check for.
	To *int `json:"to,omitempty"`
	// Filter by token's denomination.
	// If set to an empty string, it will get an empty value.
	Denomination *string `json:"denomination,omitempty"`
}

// `BankMsgSend` is a message with a message router of `bank` and a message type of `send`.
// `BankMsgSend` is the fund transfer tx message.
type BankMsgSend struct {
	// the bech32 address of the fund sender.
	// ex) `g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5`
	FromAddress string `json:"from_address"`
	// the bech32 address of the fund receiver.
	// ex) `g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5`
	ToAddress string `json:"to_address"`
	// the denomination and amount of fund sent ("<amount><denomination>").
	// ex) `1000000ugnot`
	Amount string `json:"amount"`
}

func (BankMsgSend) IsMessageValue() {}

// `BankMsgSendInput` represents input parameters required when the message type is `send`.
type BankMsgSendInput struct {
	// the bech32 address of the fund sender.
	// You can filter by the fund sender address.
	// ex) `g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5`
	FromAddress *string `json:"from_address,omitempty"`
	// the bech32 address of the fund receiver.
	// You can filter by the fund receiver address.
	// ex) `g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5`
	ToAddress *string `json:"to_address,omitempty"`
	// the denomination and amount of fund sent ("<amount><denomination>").
	// ex) `1000000ugnot`
	Amount *AmountInput `json:"amount,omitempty"`
}

// Filters for querying Blocks within specified criteria related to their attributes.
type BlockFilter struct {
	// Minimum block height from which to start fetching Blocks, inclusive. If unspecified, there is no lower bound.
	FromHeight *int `json:"from_height,omitempty"`
	// Maximum block height up to which Blocks should be fetched, exclusive. If unspecified, there is no upper bound.
	ToHeight *int `json:"to_height,omitempty"`
	// Minimum timestamp from which to start fetching Blocks, inclusive. Blocks created at or after this time will be included.
	FromTime *time.Time `json:"from_time,omitempty"`
	// Maximum timestamp up to which to fetch Blocks, exclusive. Only Blocks created before this time are included.
	ToTime *time.Time `json:"to_time,omitempty"`
}

// Defines a transaction within a block, its execution specifics and content.
type BlockTransaction struct {
	// Hash computes the TMHASH hash of the wire encoded transaction.
	Hash string `json:"hash"`
	// Fee information for the transaction.
	Fee *TxFee `json:"fee"`
	// `memo` are string information stored within a transaction.
	// `memo` can be utilized to find or distinguish transactions.
	// For example, when trading a specific exchange, you would utilize the memo field of the transaction.
	Memo string `json:"memo"`
	// The payload of the Transaction in a raw format, typically containing the instructions and any data necessary for execution.
	ContentRaw string `json:"content_raw"`
}

// Define the quantity and denomination of a coin.
type Coin struct {
	// The amount of coins.
	Amount int `json:"amount"`
	// The denomination of the coin.
	Denom string `json:"denom"`
}

// Transaction event's attribute to filter transaction.
// "EventAttributeInput" can be configured as a filter with a event attribute's `key` and `value`.
type EventAttributeInput struct {
	// `key` is the key of the event attribute.
	Key *string `json:"key,omitempty"`
	// `value` is the value of the event attribute.
	Value *string `json:"value,omitempty"`
}

// Transaction's event to filter transactions.
// "EventInput" can be configured as a filter with a transaction event's `type` and `pkg_path` and `func`, and `attrs`.
type EventInput struct {
	// `type` is the type of transaction event emitted.
	Type *string `json:"type,omitempty"`
	// `pkg_path` is the path to the package that emitted the event.
	PkgPath *string `json:"pkg_path,omitempty"`
	// `func` is the name of the function that emitted the event.
	Func *string `json:"func,omitempty"`
	// `attrs` filters transactions whose events contain attributes.
	// `attrs` is entered as an array and works exclusively.
	// ex) `attrs[0] || attrs[1] || attrs[2]`
	Attrs []*EventAttributeInput `json:"attrs,omitempty"`
}

// `GnoEvent` is the event information exported by the Gno VM.
// It has `type`, `pkg_path`, `func`, and `attrs`.
type GnoEvent struct {
	// `type` is the type of transaction event emitted.
	Type string `json:"type"`
	// `pkg_path` is the path to the package that emitted the event.
	PkgPath string `json:"pkg_path"`
	// `func` is the name of the function that emitted the event.
	Func string `json:"func"`
	// `attrs` is the event's attribute information.
	Attrs []*GnoEventAttribute `json:"attrs,omitempty"`
}

func (GnoEvent) IsEvent() {}

// `GnoEventAttribute` is the attributes that the event has.
// It has `key` and `value`.
type GnoEventAttribute struct {
	// The key of the event attribute.
	Key string `json:"key"`
	// The value of the event attribute.
	Value string `json:"value"`
}

// `MemFile` is the metadata information tied to a single gno package / realm file
type MemFile struct {
	// the name of the source file.
	Name string `json:"name"`
	// the content of the source file.
	Body string `json:"body"`
}

// `MemFileInput` is the metadata information tied to a single gno package / realm file.
type MemFileInput struct {
	// the name of the source file.
	Name *string `json:"name,omitempty"`
	// the content of the source file.
	Body *string `json:"body,omitempty"`
}

// `MemPackage` is the metadata information tied to package / realm deployment.
type MemPackage struct {
	// the name of the package.
	Name string `json:"name"`
	// the gno path of the package.
	Path string `json:"path"`
	// the associated package gno source.
	Files []*MemFile `json:"files,omitempty"`
}

// `MemPackageInput` represents a package stored in memory.
type MemPackageInput struct {
	// the name of the package.
	Name *string `json:"name,omitempty"`
	// the gno path of the package.
	Path *string `json:"path,omitempty"`
	// the associated package gno source.
	Files []*MemFileInput `json:"files,omitempty"`
}

// `MsgAddPackage` is a message with a message router of `vm` and a message type of `add_package`.
// `MsgAddPackage` is the package deployment tx message.
type MsgAddPackage struct {
	// the bech32 address of the package deployer.
	// ex) `g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5`
	Creator string `json:"creator"`
	// the package being deployed.
	Package *MemPackage `json:"package"`
	// the amount of funds to be deposited at deployment, if any ("<amount><denomination>").
	// ex) `1000000ugnot`
	Deposit string `json:"deposit"`
}

func (MsgAddPackage) IsMessageValue() {}

// `MsgAddPackageInput` represents input parameters required when the message type is `add_package`.
type MsgAddPackageInput struct {
	// the bech32 address of the package deployer.
	// You can filter by the package deployer's address.
	// ex) `g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5`
	Creator *string `json:"creator,omitempty"`
	// the package being deployed.
	Package *MemPackageInput `json:"package,omitempty"`
	// the amount of funds to be deposited at deployment, if any ("<amount><denomination>").
	// ex) `1000000ugnot`
	Deposit *AmountInput `json:"deposit,omitempty"`
}

// `MsgCall` is a message with a message router of `vm` and a message type of `exec`.
// `MsgCall` is the method invocation tx message.
type MsgCall struct {
	// the bech32 address of the function caller.
	// ex) `g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5`
	Caller string `json:"caller"`
	// the amount of funds to be deposited to the package, if any ("<amount><denomination>").
	// ex) `1000000ugnot`
	Send string `json:"send"`
	// the gno package path.
	PkgPath string `json:"pkg_path"`
	// the function name being invoked.
	Func string `json:"func"`
	// `args` are the arguments passed to the executed function.
	Args []string `json:"args,omitempty"`
}

func (MsgCall) IsMessageValue() {}

// `MsgCallInput` represents input parameters required when the message type is `exec`.
type MsgCallInput struct {
	// the bech32 address of the function caller.
	// You can filter by the function caller's address.
	// ex) `g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5`
	Caller *string `json:"caller,omitempty"`
	// the amount of funds to be deposited to the package, if any ("<amount><denomination>").
	// ex) `1000000ugnot`
	Send *AmountInput `json:"send,omitempty"`
	// the gno package path.
	PkgPath *string `json:"pkg_path,omitempty"`
	// the function name being invoked.
	Func *string `json:"func,omitempty"`
	// `args` are the arguments passed to the executed function.
	// The arguments are checked in the order of the argument array and
	// if they are empty strings, they are excluded from the filtering criteria.
	// ex) `["", "", "1"]` <- Empty strings skip the condition.
	Args []string `json:"args,omitempty"`
}

// `MsgRun` is a message with a message router of `vm` and a message type of `run`.
// `MsgRun is the execute arbitrary Gno code tx message`.
type MsgRun struct {
	// the bech32 address of the function caller.
	// ex) `g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5`
	Caller string `json:"caller"`
	// the amount of funds to be deposited to the package, if any ("<amount><denomination>").
	// ex) `1000000ugnot`
	Send string `json:"send"`
	// the package being executed.
	Package *MemPackage `json:"package"`
}

func (MsgRun) IsMessageValue() {}

// `MsgRunInput` represents input parameters required when the message type is `run`.
type MsgRunInput struct {
	// the bech32 address of the function caller.
	// You can filter by the function caller's address.
	// ex) `g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5`
	Caller *string `json:"caller,omitempty"`
	// the amount of funds to be deposited to the package, if any ("<amount><denomination>").
	// ex) `1000000ugnot`
	Send *AmountInput `json:"send,omitempty"`
	// the package being executed.
	Package *MemPackageInput `json:"package,omitempty"`
}

// Root Query type to fetch data about Blocks and Transactions based on filters or retrieve the latest block height.
type Query struct {
}

// Subscriptions provide a way for clients to receive real-time updates about Transactions and Blocks based on specified filter criteria.
// Subscribers will only receive updates for events occurring after the subscription is established.
type Subscription struct {
}

// `TransactionBankMessageInput` represents input parameters required when the message router is `bank`.
type TransactionBankMessageInput struct {
	// send represents input parameters required when the message type is `send`.
	Send *BankMsgSendInput `json:"send,omitempty"`
}

// Filters for querying Transactions within specified criteria related to their execution and placement within Blocks.
type TransactionFilter struct {
	// Minimum block height from which to start fetching Transactions, inclusive. Aids in scoping the search to recent Transactions.
	FromBlockHeight *int `json:"from_block_height,omitempty"`
	// Maximum block height up to which Transactions should be fetched, exclusive. Helps in limiting the search to older Transactions.
	ToBlockHeight *int `json:"to_block_height,omitempty"`
	// Minimum Transaction index from which to start fetching, inclusive. Facilitates ordering in Transaction queries.
	FromIndex *int `json:"from_index,omitempty"`
	// Maximum Transaction index up to which to fetch, exclusive. Ensures a limit on the ordering range for Transaction queries.
	ToIndex *int `json:"to_index,omitempty"`
	// Minimum `gas_wanted` value to filter Transactions by, inclusive. Filters Transactions based on the minimum computational effort declared.
	FromGasWanted *int `json:"from_gas_wanted,omitempty"`
	// Maximum `gas_wanted` value for filtering Transactions, exclusive. Limits Transactions based on the declared computational effort.
	ToGasWanted *int `json:"to_gas_wanted,omitempty"`
	// Minimum `gas_used` value to filter Transactions by, inclusive. Selects Transactions based on the minimum computational effort actually used.
	FromGasUsed *int `json:"from_gas_used,omitempty"`
	// Maximum `gas_used` value for filtering Transactions, exclusive. Refines selection based on the computational effort actually consumed.
	ToGasUsed *int `json:"to_gas_used,omitempty"`
	// Hash from Transaction content in base64 encoding. If this filter is used, any other filter will be ignored.
	Hash *string `json:"hash,omitempty"`
	// Transaction's message to filter Transactions.
	// `message` can be configured as a filter with a transaction message's `router` and `type` and `parameters(bank / vm)`.
	Message *TransactionMessageInput `json:"message,omitempty"`
	// `memo` are string information stored within a transaction.
	// `memo` can be utilized to find or distinguish transactions.
	// For example, when trading a specific exchange, you would utilize the memo field of the transaction.
	Memo *string `json:"memo,omitempty"`
	// `success` is whether the transaction was successful or not.
	// `success` enables you to filter between successful and unsuccessful transactions.
	Success *bool `json:"success,omitempty"`
	// `events` are what the transaction has emitted.
	// `events` can be filtered with a specific event to query its transactions.
	// `events` is entered as an array and works exclusively.
	// ex) `events[0] || events[1] || events[2]`
	Events []*EventInput `json:"events,omitempty"`
}

// Transaction's message to filter Transactions.
// `TransactionMessageInput` can be configured as a filter with a transaction message's `router` and `type` and `parameters(bank / vm)`.
type TransactionMessageInput struct {
	// The type of transaction message.
	// The value of `typeUrl` can be `send`, `exec`, `add_package`, `run`.
	TypeURL *MessageType `json:"type_url,omitempty"`
	// The route of transaction message.
	// The value of `route` can be `bank`, `vm`.
	Route *MessageRoute `json:"route,omitempty"`
	// `TransactionBankMessageInput` represents input parameters required when the message router is `bank`.
	BankParam *TransactionBankMessageInput `json:"bank_param,omitempty"`
	// `TransactionVmMessageInput` represents input parameters required when the message router is `vm`.
	VMParam *TransactionVMMessageInput `json:"vm_param,omitempty"`
}

// `TransactionVmMessageInput` represents input parameters required when the message router is `vm`.
type TransactionVMMessageInput struct {
	// `MsgCallInput` represents input parameters required when the message type is `exec`.
	Exec *MsgCallInput `json:"exec,omitempty"`
	// `MsgAddPackageInput` represents input parameters required when the message type is `add_package`.
	AddPackage *MsgAddPackageInput `json:"add_package,omitempty"`
	// `MsgRunInput` represents input parameters required when the message type is `run`.
	Run *MsgRunInput `json:"run,omitempty"`
}

// The `TxFee` has information about the fee used in the transaction and the maximum gas fee specified by the user.
type TxFee struct {
	// gas limit
	GasWanted int `json:"gas_wanted"`
	// The gas fee used in the transaction.
	GasFee *Coin `json:"gas_fee"`
}

// `UnexpectedMessage` is an Undefined Message, which is a message that decoding failed.
type UnexpectedMessage struct {
	Raw string `json:"raw"`
}

func (UnexpectedMessage) IsMessageValue() {}

// `UnknownEvent` is an unknown event type.
// It has `value`.
type UnknownEvent struct {
	// `value` is an raw event string.
	Value string `json:"value"`
}

func (UnknownEvent) IsEvent() {}

// `MessageRoute` is route type of the transactional message.
// `MessageRoute` has the values of vm and bank.
type MessageRoute string

const (
	MessageRouteVM   MessageRoute = "vm"
	MessageRouteBank MessageRoute = "bank"
)

var AllMessageRoute = []MessageRoute{
	MessageRouteVM,
	MessageRouteBank,
}

func (e MessageRoute) IsValid() bool {
	switch e {
	case MessageRouteVM, MessageRouteBank:
		return true
	}
	return false
}

func (e MessageRoute) String() string {
	return string(e)
}

func (e *MessageRoute) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = MessageRoute(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid MessageRoute", str)
	}
	return nil
}

func (e MessageRoute) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

// `MessageType` is message type of the transaction.
// `MessageType` has the values `send`, `exec`, `add_package`, and `run`.
type MessageType string

const (
	// The route value for this message type is `bank`, and the value for transactional messages is `BankMsgSend`.
	// This is a transaction message used when sending native tokens.
	MessageTypeSend MessageType = "send"
	// The route value for this message type is `vm`, and the value for transactional messages is `MsgCall`.
	// This is a transaction message that executes a function in realm or package that is deployed in the GNO chain.
	MessageTypeExec MessageType = "exec"
	// The route value for this message type is `vm`, and the value for transactional messages is `MsgAddPackage`.
	// This is a transactional message that adds a package to the GNO chain.
	MessageTypeAddPackage MessageType = "add_package"
	// The route value for this message type is `vm`, and the value for transactional messages is `MsgRun`.
	// This is a transactional message that executes an arbitrary Gno-coded TX message.
	MessageTypeRun MessageType = "run"
)

var AllMessageType = []MessageType{
	MessageTypeSend,
	MessageTypeExec,
	MessageTypeAddPackage,
	MessageTypeRun,
}

func (e MessageType) IsValid() bool {
	switch e {
	case MessageTypeSend, MessageTypeExec, MessageTypeAddPackage, MessageTypeRun:
		return true
	}
	return false
}

func (e MessageType) String() string {
	return string(e)
}

func (e *MessageType) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}

	*e = MessageType(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid MessageType", str)
	}
	return nil
}

func (e MessageType) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}
