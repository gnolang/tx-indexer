package graph

import (
	"math"

	"github.com/gnolang/gno/tm2/pkg/std"
	"github.com/gnolang/tx-indexer/serve/graph/model"
)

func FilteredTransactionBy(tx *model.Transaction, filter model.TransactionFilter) bool {
	if !filteredTransactionByGasUsed(tx, filter.FromGasUsed, filter.ToGasUsed) {
		return false
	}

	if !filteredTransactionByGasWanted(tx, filter.FromGasWanted, filter.ToGasWanted) {
		return false
	}

	if !filteredTransactionByMemo(tx, filter.Memo) {
		return false
	}

	if filter.Message != nil {
		if !filteredTransactionByMessageRoute(tx, filter.Message.Route) {
			return false
		}

		if !filteredTransactionByMessageType(tx, filter.Message.TypeURL) {
			return false
		}

		if !filteredTransactionByMessages(tx, filter.Message) {
			return false
		}
	}

	return true
}

func filteredAmountBy(amountStr string, amountInput *model.AmountInput) bool {
	if amountInput == nil {
		return true
	}

	coins, err := std.ParseCoins(amountStr)
	if err != nil {
		return false
	}

	if amountInput.Denomination != nil {
		if deref(amountInput.Denomination) == "" && coins.Empty() {
			return true
		}
	}

	for _, coin := range coins {
		if amountInput.Denomination == nil || coin.Denom == deref(amountInput.Denomination) {
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

func filteredTransactionByGasUsed(tx *model.Transaction, filterFromGasUsed, filterToGasUsed *int) bool {
	fromGasUsed := deref(filterFromGasUsed)
	toGasUsed := deref(filterToGasUsed)
	transactionGasUsed := tx.GasUsed()

	if toGasUsed == 0 {
		toGasUsed = math.MaxInt
	}

	return transactionGasUsed >= fromGasUsed && transactionGasUsed <= toGasUsed
}

func filteredTransactionByGasWanted(tx *model.Transaction, filterFromGasWanted, filterToGasWanted *int) bool {
	fromGasWanted := deref(filterFromGasWanted)
	toGasWanted := deref(filterToGasWanted)
	transactionGasUsed := tx.GasWanted()

	if toGasWanted == 0 {
		toGasWanted = math.MaxInt
	}

	return transactionGasUsed >= fromGasWanted && transactionGasUsed <= toGasWanted
}

func filteredTransactionByMemo(tx *model.Transaction, filterMemo *string) bool {
	if filterMemo == nil {
		return true
	}

	return deref(filterMemo) == tx.Memo()
}

func filteredTransactionByMessages(tx *model.Transaction, messageInput *model.TransactionMessageInput) bool {
	messages := tx.Messages()
	for _, message := range messages {
		if !filteredTransactionMessageBy(message, messageInput) {
			return false
		}
	}

	return true
}

func filteredTransactionByMessageRoute(tx *model.Transaction, messageRoute *model.MessageRoute) bool {
	if messageRoute == nil {
		return true
	}

	messages := tx.Messages()
	for _, message := range messages {
		if message.Route == messageRoute.String() {
			return true
		}
	}

	return false
}

func filteredTransactionByMessageType(tx *model.Transaction, messageType *model.MessageType) bool {
	if messageType == nil {
		return true
	}

	messages := tx.Messages()
	for _, message := range messages {
		if message.TypeURL == messageType.String() {
			return true
		}
	}

	return false
}

func filteredTransactionMessageBy(
	tm *model.TransactionMessage,
	messageInput *model.TransactionMessageInput,
) bool {
	if messageInput.TypeURL != nil && messageInput.TypeURL.String() != tm.TypeURL {
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
	}

	switch tm.TypeURL {
	case model.MessageTypeSend.String():
		if !checkMessageOfBankMsgSend(tm.BankMsgSend(), messageInput.BankParam) {
			return false
		}
	case model.MessageTypeExec.String():
		if !checkByMessageOfMsgCall(tm.VMMsgCall(), messageInput.VMParam) {
			return false
		}
	case model.MessageTypeAddPackage.String():
		if !checkMessageOfMsgAddPackage(tm.VMAddPackage(), messageInput.VMParam) {
			return false
		}
	case model.MessageTypeRun.String():
		if !checkMessageOfMsgRun(tm.VMMsgRun(), messageInput.VMParam) {
			return false
		}
	}

	return true
}

func checkMessageOfBankMsgSend(
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

func checkByMessageOfMsgCall(
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

	if params.Exec.Send != nil && filteredAmountBy(messageValue.Send, params.Exec.Send) {
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

func checkMessageOfMsgAddPackage(
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

	if params.AddPackage.Deposit != nil && filteredAmountBy(messageValue.Deposit, params.AddPackage.Deposit) {
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

func checkMessageOfMsgRun(messageValue model.MsgRun, vmMessageInput *model.TransactionVMMessageInput) bool {
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

	if params.Run.Send != nil && filteredAmountBy(messageValue.Send, params.Run.Send) {
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
