package graph

import (
	"math"

	"github.com/gnolang/gno/tm2/pkg/std"

	"github.com/gnolang/tx-indexer/serve/graph/model"
)

// `FilteredTransactionBy` checks for conditions in GasUsed, GasWanted, Memo, and Message.
// By default, the condition is only checked if the input parameter exists.
func FilteredTransactionBy(tx *model.Transaction, filter model.TransactionFilter) bool {
	if !filteredTransactionBySuccess(tx, filter.Success) {
		return false
	}

	if !filteredTransactionByGasUsed(tx, filter.FromGasUsed, filter.ToGasUsed) {
		return false
	}

	if !filteredTransactionByGasWanted(tx, filter.FromGasWanted, filter.ToGasWanted) {
		return false
	}

	if !filteredTransactionByMemo(tx, filter.Memo) {
		return false
	}

	if !filteredTransactionByEvents(tx, filter.Events) {
		return false
	}

	if !filteredTransactionByMessages(tx, filter.Message) {
		return false
	}

	return true
}

// `filteredTransactionBySuccess` will check the success or failure results of the transaction.
func filteredTransactionBySuccess(tx *model.Transaction, success *bool) bool {
	if success == nil {
		return true
	}

	return deref(success) == tx.Success()
}

// `filteredTransactionByEvents` checks for events in the transaction's results.
func filteredTransactionByEvents(tx *model.Transaction, eventInputs []*model.EventInput) bool {
	if len(eventInputs) == 0 {
		return true
	}

	events := tx.Response().Events()
	if len(events) == 0 {
		return false
	}

	for _, event := range events {
		for _, eventInput := range eventInputs {
			if filteredEventBy(event, eventInput) {
				return true
			}
		}
	}

	return false
}

// `filteredEventBy` checks whether an event satisfies the input set for its
// own type. An input with no event type set matches any event.
func filteredEventBy(event model.Event, eventInput *model.EventInput) bool {
	if event == nil {
		return false
	}

	if !eventInputNamesType(eventInput) {
		return true
	}

	switch typed := event.(type) {
	case *model.GnoEvent:
		return eventInput.GnoEvent != nil && filteredGnoEventBy(typed, eventInput.GnoEvent)
	case *model.TransferEvent:
		return eventInput.TransferEvent != nil && filteredTransferEventBy(typed, eventInput.TransferEvent)
	case *model.StorageDepositEvent:
		return eventInput.StorageDepositEvent != nil &&
			filteredStorageDepositEventBy(typed, eventInput.StorageDepositEvent)
	case *model.StorageUnlockEvent:
		return eventInput.StorageUnlockEvent != nil &&
			filteredStorageUnlockEventBy(typed, eventInput.StorageUnlockEvent)
	default:
		return false
	}
}

// `eventInputNamesType` reports whether the input asks for an event type at all.
func eventInputNamesType(input *model.EventInput) bool {
	return input.GnoEvent != nil ||
		input.StorageDepositEvent != nil ||
		input.StorageUnlockEvent != nil ||
		input.TransferEvent != nil
}

// `filteredTransferEventBy` checks the conditions of a TransferEvent.
func filteredTransferEventBy(event *model.TransferEvent, input *model.TransferEventInput) bool {
	if input.Type != nil && deref(input.Type) != event.Type {
		return false
	}

	if input.From != nil && deref(input.From) != event.From {
		return false
	}

	if input.To != nil && deref(input.To) != event.To {
		return false
	}

	return filteredAmountBy(event.Coins, input.Coins)
}

// `filteredGnoEventBy` checks the conditions of a GnoEvent.
func filteredGnoEventBy(event *model.GnoEvent, input *model.GnoEventInput) bool {
	if input.Type != nil && deref(input.Type) != event.Type {
		return false
	}

	if input.PkgPath != nil && deref(input.PkgPath) != event.PkgPath {
		return false
	}

	if input.Attrs != nil && !filteredGnoEventAttributesBy(event.Attrs, input.Attrs) {
		return false
	}

	return true
}

// `filteredStorageDepositEventBy` checks the conditions of a StorageDepositEvent.
func filteredStorageDepositEventBy(event *model.StorageDepositEvent, input *model.StorageDepositEventInput) bool {
	if input.Type != nil && deref(input.Type) != event.Type {
		return false
	}

	if input.BytesDelta != nil && deref(input.BytesDelta) != event.BytesDelta {
		return false
	}

	if input.PkgPath != nil && deref(input.PkgPath) != event.PkgPath {
		return false
	}

	return filteredCoinBy(event.FeeDelta, input.FeeDelta)
}

// `filteredStorageUnlockEventBy` checks the conditions of a StorageUnlockEvent.
func filteredStorageUnlockEventBy(event *model.StorageUnlockEvent, input *model.StorageUnlockEventInput) bool {
	if input.Type != nil && deref(input.Type) != event.Type {
		return false
	}

	if input.BytesDelta != nil && deref(input.BytesDelta) != event.BytesDelta {
		return false
	}

	if input.PkgPath != nil && deref(input.PkgPath) != event.PkgPath {
		return false
	}

	if input.RefundWithheld != nil && deref(input.RefundWithheld) != event.RefundWithheld {
		return false
	}

	return filteredCoinBy(event.FeeRefund, input.FeeRefund)
}

// `filteredCoinBy` checks a coin against the amount and denom set on the input.
func filteredCoinBy(coin *model.Coin, input *model.CoinInput) bool {
	if input == nil {
		return true
	}

	if coin == nil {
		return false
	}

	if input.Amount != nil && deref(input.Amount) != coin.Amount {
		return false
	}

	if input.Denom != nil && deref(input.Denom) != coin.Denom {
		return false
	}

	return true
}

// `filteredGnoEventAttributesBy` check the conditions of event attributes
func filteredGnoEventAttributesBy(
	attrs []*model.GnoEventAttribute,
	filterAttrs []*model.EventAttributeInput,
) bool {
	if len(attrs) == 0 {
		return true
	}

	for _, attr := range attrs {
		for _, attributeFilter := range filterAttrs {
			if attributeFilter.Key != nil && attr.Key != deref(attributeFilter.Key) {
				continue
			}

			if attributeFilter.Value != nil && attr.Value != deref(attributeFilter.Value) {
				continue
			}

			return true
		}
	}

	return false
}

// `filteredAmountBy` checks a token represented as a string(<value><denomination>)
// against a range of amount and a denomination.
func filteredAmountBy(amountStr string, amountInput *model.AmountInput) bool {
	if amountInput == nil {
		return true
	}

	coins, err := std.ParseCoins(amountStr)
	if err != nil {
		return false
	}

	// If the input parameter for denomination is not used, all denominations are checked.
	isAllDenomination := amountInput.Denomination == nil
	if !isAllDenomination {
		if deref(amountInput.Denomination) == "" && coins.Empty() {
			return true
		}
	}

	for _, coin := range coins {
		isSameDenomination := coin.Denom == deref(amountInput.Denomination)
		if isAllDenomination || isSameDenomination {
			fromAmount := int64(deref(amountInput.From))
			toAmount := int64(deref(amountInput.To))

			if toAmount == 0 {
				toAmount = math.MaxInt
			}

			if coin.Amount >= fromAmount && coin.Amount <= toAmount {
				return true
			}
		}
	}

	return false
}

// `filteredTransactionByGasUsed` checks transactions based on gasUsed.
func filteredTransactionByGasUsed(tx *model.Transaction, filterFromGasUsed, filterToGasUsed *int) bool {
	gasUsed := tx.GasUsed()
	fromGasUsed := deref(filterFromGasUsed)
	toGasUsed := deref(filterToGasUsed)

	if toGasUsed == 0 {
		toGasUsed = math.MaxInt
	}

	return gasUsed >= fromGasUsed && gasUsed <= toGasUsed
}

// `filteredTransactionByGasWanted` checks transactions based on gasWanted.
func filteredTransactionByGasWanted(tx *model.Transaction, filterFromGasWanted, filterToGasWanted *int) bool {
	gasWanted := tx.GasWanted()
	fromGasWanted := deref(filterFromGasWanted)
	toGasWanted := deref(filterToGasWanted)

	if toGasWanted == 0 {
		toGasWanted = math.MaxInt
	}

	return gasWanted >= fromGasWanted && gasWanted <= toGasWanted
}

// `filteredTransactionByMemo` checks transactions based on memo.
func filteredTransactionByMemo(tx *model.Transaction, filterMemo *string) bool {
	if filterMemo == nil {
		return true
	}

	return deref(filterMemo) == tx.Memo()
}

// `filteredTransactionByMessages` checks transaction's messages.
func filteredTransactionByMessages(tx *model.Transaction, messageInputs []*model.TransactionMessageInput) bool {
	if len(messageInputs) == 0 {
		return true
	}

	messages := tx.Messages()
	if len(messages) == 0 {
		return false
	}

	for _, message := range messages {
		for _, messageInput := range messageInputs {
			if filteredTransactionMessageBy(message, messageInput) {
				return true
			}
		}
	}

	return false
}

// `filteredTransactionMessageBy` checks for conditions based on the transaction message type.
func filteredTransactionMessageBy(
	tm *model.TransactionMessage,
	messageInput *model.TransactionMessageInput,
) bool {
	if tm == nil {
		return false
	}

	if messageInput.Route != nil && deref(messageInput.Route) != model.MessageRoute(tm.Route) {
		return false
	}

	if messageInput.TypeURL != nil && deref(messageInput.TypeURL) != model.MessageType(tm.TypeURL) {
		return false
	}

	if messageInput.BankParam == nil && messageInput.VMParam == nil {
		return true
	}

	switch tm.Route {
	case model.MessageRouteBank.String():
		if messageInput.BankParam == nil {
			return false
		}
	case model.MessageRouteVM.String():
		if messageInput.VMParam == nil {
			return false
		}
	default:
		return false
	}

	switch tm.TypeURL {
	case model.MessageTypeSend.String():
		if !filteredMessageOfBankMsgSendBy(tm.BankMsgSend(), messageInput.BankParam) {
			return false
		}
	case model.MessageTypeExec.String():
		if !filteredMessageOfMsgCallBy(tm.VMMsgCall(), messageInput.VMParam) {
			return false
		}
	case model.MessageTypeAddPackage.String():
		if !filteredMessageOfMsgAddPackageBy(tm.VMAddPackage(), messageInput.VMParam) {
			return false
		}
	case model.MessageTypeRun.String():
		if !filteredMessageOfMsgRunBy(tm.VMMsgRun(), messageInput.VMParam) {
			return false
		}
	case model.MessageTypeEnablePackage.String():
		if !filteredMessageOfMsgEnablePackageBy(tm.VMMsgEnablePackage(), messageInput.VMParam) {
			return false
		}
	case model.MessageTypeRejectPackage.String():
		if !filteredMessageOfMsgRejectPackageBy(tm.VMMsgRejectPackage(), messageInput.VMParam) {
			return false
		}
	default:
		return false
	}

	return true
}

// `filteredMessageOfBankMsgSendBy` checks the conditions of a message of type BankMsgSend
func filteredMessageOfBankMsgSendBy(
	messageValue model.BankMsgSend,
	bankMessageInput *model.TransactionBankMessageInput,
) bool {
	params := bankMessageInput
	if params == nil || params.Send == nil {
		return true
	}

	if params.Send.Amount != nil && !filteredAmountBy(messageValue.Amount, params.Send.Amount) {
		return false
	}

	if params.Send.FromAddress != nil && deref(params.Send.FromAddress) != messageValue.FromAddress {
		return false
	}

	if params.Send.ToAddress != nil && deref(params.Send.ToAddress) != messageValue.ToAddress {
		return false
	}

	return true
}

// `filteredMessageOfMsgCallBy` checks the conditions of a message of type MsgCall
func filteredMessageOfMsgCallBy(
	messageValue model.MsgCall,
	vmMessageInput *model.TransactionVMMessageInput,
) bool {
	params := vmMessageInput
	if params == nil {
		return true
	}

	if params.Exec == nil {
		return false
	}

	if params.Exec.Caller != nil && deref(params.Exec.Caller) != messageValue.Caller {
		return false
	}

	if params.Exec.Func != nil && deref(params.Exec.Func) != messageValue.Func {
		return false
	}

	if params.Exec.PkgPath != nil && deref(params.Exec.PkgPath) != messageValue.PkgPath {
		return false
	}

	if params.Exec.Send != nil && !filteredAmountBy(messageValue.Send, params.Exec.Send) {
		return false
	}

	if params.Exec.Args != nil {
		messageArgs := messageValue.Args
		if messageArgs == nil {
			return false
		}

		messageFilterArgs := params.Exec.Args
		for index, arg := range messageArgs {
			if index < len(messageFilterArgs) {
				if arg != "" && messageFilterArgs[index] != arg {
					return false
				}
			}
		}
	}

	return true
}

// `filteredMessageOfMsgAddPackageBy` checks the conditions of a message of type MsgAddPackage
func filteredMessageOfMsgAddPackageBy(
	messageValue model.MsgAddPackage,
	vmMessageInput *model.TransactionVMMessageInput,
) bool {
	params := vmMessageInput
	if params == nil {
		return true
	}

	if params.AddPackage == nil {
		return false
	}

	if params.AddPackage.Creator != nil && deref(params.AddPackage.Creator) != messageValue.Creator {
		return false
	}

	if params.AddPackage.Deposit != nil && !filteredAmountBy(messageValue.Deposit, params.AddPackage.Deposit) {
		return false
	}

	if params.AddPackage.Package != nil {
		if params.AddPackage.Package.Name != nil && deref(params.AddPackage.Package.Name) != messageValue.Package.Name {
			return false
		}

		if params.AddPackage.Package.Path != nil && deref(params.AddPackage.Package.Path) != messageValue.Package.Path {
			return false
		}
	}

	return true
}

// `filteredMessageOfMsgRunBy` checks the conditions of a message of type MsgRun
func filteredMessageOfMsgRunBy(messageValue model.MsgRun, vmMessageInput *model.TransactionVMMessageInput) bool {
	params := vmMessageInput
	if params == nil {
		return true
	}

	if params.Run == nil {
		return false
	}

	if params.Run.Caller != nil && deref(params.Run.Caller) != messageValue.Caller {
		return false
	}

	if params.Run.Send != nil && !filteredAmountBy(messageValue.Send, params.Run.Send) {
		return false
	}

	if params.Run.Package != nil {
		if deref(params.Run.Package.Name) != messageValue.Package.Name {
			return false
		}

		if deref(params.Run.Package.Path) != messageValue.Package.Path {
			return false
		}
	}

	return true
}

// `filteredMessageOfMsgEnablePackageBy` checks the conditions of a message of type MsgEnablePackage
func filteredMessageOfMsgEnablePackageBy(
	messageValue model.MsgEnablePackage,
	vmMessageInput *model.TransactionVMMessageInput,
) bool {
	params := vmMessageInput
	if params == nil {
		return true
	}

	if params.EnablePackage == nil {
		return false
	}

	if params.EnablePackage.Approver != nil && deref(params.EnablePackage.Approver) != messageValue.Approver {
		return false
	}

	if params.EnablePackage.PkgPath != nil && deref(params.EnablePackage.PkgPath) != messageValue.PkgPath {
		return false
	}

	if params.EnablePackage.PkgHash != nil && deref(params.EnablePackage.PkgHash) != messageValue.PkgHash {
		return false
	}

	if params.EnablePackage.PkgHeight != nil && deref(params.EnablePackage.PkgHeight) != messageValue.PkgHeight {
		return false
	}

	return true
}

// `filteredMessageOfMsgRejectPackageBy` checks the conditions of a message of type MsgRejectPackage
func filteredMessageOfMsgRejectPackageBy(
	messageValue model.MsgRejectPackage,
	vmMessageInput *model.TransactionVMMessageInput,
) bool {
	params := vmMessageInput
	if params == nil {
		return true
	}

	if params.RejectPackage == nil {
		return false
	}

	if params.RejectPackage.Sender != nil && deref(params.RejectPackage.Sender) != messageValue.Sender {
		return false
	}

	if params.RejectPackage.PkgPath != nil && deref(params.RejectPackage.PkgPath) != messageValue.PkgPath {
		return false
	}

	return true
}
