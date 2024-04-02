package resolvers

import (
	"math"

	"github.com/gnolang/tx-indexer/serve/graph/model"
)

// The resolver for handling the transaction model.
type TransactionResolver struct {
	transaction *model.Transaction
}

func NewTransactionResolver(transaction *model.Transaction) *TransactionResolver {
	return &TransactionResolver{
		transaction: transaction,
	}
}

func (r *TransactionResolver) GetTransaction() *model.Transaction {
	return r.transaction
}

func (r *TransactionResolver) FilteredBy(filter model.TransactionFilter) bool {
	if !r.filteredByGasUsed(filter.FromGasUsed, filter.ToGasUsed) {
		return false
	}

	if !r.filteredByGasWanted(filter.FromGasWanted, filter.ToGasWanted) {
		return false
	}

	if !r.filteredByMemo(filter.Memo) {
		return false
	}

	if filter.Message != nil {
		if !r.filteredByMessageRoute(filter.Message.Route) {
			return false
		}

		if !r.filteredByMessageType(filter.Message.TypeURL) {
			return false
		}

		if !r.filteredByMessages(filter.Message) {
			return false
		}
	}

	return true
}

func (r *TransactionResolver) filteredByGasUsed(filterFromGasUsed, filterToGasUsed *int) bool {
	fromGasUsed := deref(filterFromGasUsed)
	toGasUsed := deref(filterToGasUsed)
	transactionGasUsed := r.transaction.GasUsed()

	if toGasUsed == 0 {
		toGasUsed = math.MaxInt
	}

	return transactionGasUsed >= fromGasUsed && transactionGasUsed <= toGasUsed
}

func (r *TransactionResolver) filteredByGasWanted(filterFromGasWanted, filterToGasWanted *int) bool {
	fromGasWanted := deref(filterFromGasWanted)
	toGasWanted := deref(filterToGasWanted)
	transactionGasUsed := r.transaction.GasWanted()

	if toGasWanted == 0 {
		toGasWanted = math.MaxInt
	}

	return transactionGasUsed >= fromGasWanted && transactionGasUsed <= toGasWanted
}

func (r *TransactionResolver) filteredByMemo(filterMemo *string) bool {
	if filterMemo == nil {
		return true
	}

	return deref(filterMemo) == r.transaction.Memo()
}

func (r *TransactionResolver) filteredByMessages(messageInput *model.TransactionMessageInput) bool {
	messages := r.transaction.Messages()
	for _, message := range messages {
		if !r.filteredByTransactionMessage(messageInput, message) {
			return false
		}
	}

	return true
}

func (r *TransactionResolver) filteredByMessageRoute(messageRoute *model.MessageRoute) bool {
	if messageRoute == nil {
		return true
	}

	messages := r.transaction.Messages()
	for _, message := range messages {
		if message.Route.String() == messageRoute.String() {
			return true
		}
	}

	return false
}

func (r *TransactionResolver) filteredByMessageType(messageType *model.MessageType) bool {
	if messageType == nil {
		return true
	}

	messages := r.transaction.Messages()
	for _, message := range messages {
		if message.TypeURL.String() == messageType.String() {
			return true
		}
	}

	return false
}

func (r *TransactionResolver) filteredByTransactionMessage(messageInput *model.TransactionMessageInput, tm *model.TransactionMessage) bool {
	if messageInput.TypeURL != nil && messageInput.TypeURL.String() != tm.TypeURL.String() {
		return false
	}

	if messageInput.BankParam == nil && messageInput.VMParam == nil {
		return true
	}

	switch tm.Route {
	case model.MessageRouteBank:
		if messageInput.BankParam == nil {
			return false
		}
	case model.MessageRouteVM:
		if messageInput.VMParam == nil {
			return false
		}
	}

	switch tm.TypeURL {
	case model.MessageTypeSend:
		if !checkMessageOfBankMsgSend(messageInput.BankParam, tm.BankMsgSend()) {
			return false
		}
	case model.MessageTypeExec:
		if !checkByMessageOfMsgCall(messageInput.VMParam, tm.VMMsgCall()) {
			return false
		}
	case model.MessageTypeAddPackage:
		if !checkMessageOfMsgAddPackage(messageInput.VMParam, tm.VMAddPackage()) {
			return false
		}
	case model.MessageTypeRun:
		if !checkMessageOfMsgRun(messageInput.VMParam, tm.VMMsgRun()) {
			return false
		}
	}

	return true
}

func checkMessageOfBankMsgSend(bankMessageInput *model.TransactionBankMessageInput, messageValue model.BankMsgSend) bool {
	params := bankMessageInput
	if params == nil || params.Send == nil {
		return true
	}

	if params.Send.Amount != nil && deref(params.Send.Amount) != messageValue.Amount {
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

func checkByMessageOfMsgCall(vmMessageInput *model.TransactionVMMessageInput, messageValue model.MsgCall) bool {
	params := vmMessageInput
	if params == nil {
		return true
	}

	if params.MCall == nil {
		return false
	}

	if params.MCall.Caller != nil && deref(params.MCall.Caller) != messageValue.Caller {
		return false
	}

	if params.MCall.Func != nil && deref(params.MCall.Func) != messageValue.Func {
		return false
	}

	if params.MCall.PkgPath != nil && deref(params.MCall.PkgPath) != messageValue.PkgPath {
		return false
	}

	if params.MCall.Send != nil && deref(params.MCall.Send) != messageValue.Send {
		return false
	}

	if params.MCall.Args != nil {
		messageArgs := messageValue.Args
		if messageArgs == nil {
			return false
		}

		messageFilterArgs := params.MCall.Args
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

func checkMessageOfMsgAddPackage(vmMessageInput *model.TransactionVMMessageInput, messageValue model.MsgAddPackage) bool {
	params := vmMessageInput
	if params == nil {
		return true
	}

	if params.MAddpkg == nil {
		return false
	}

	if params.MAddpkg.Creator != nil && deref(params.MAddpkg.Creator) != messageValue.Creator {
		return false
	}

	if params.MAddpkg.Deposit != nil && deref(params.MAddpkg.Deposit) != messageValue.Deposit {
		return false
	}

	if params.MAddpkg.Package != nil {
		if params.MAddpkg.Package.Name != nil && deref(params.MAddpkg.Package.Name) != messageValue.Package.Name {
			return false
		}

		if params.MAddpkg.Package.Path != nil && deref(params.MAddpkg.Package.Path) != messageValue.Package.Path {
			return false
		}
	}

	return true
}

func checkMessageOfMsgRun(vmMessageInput *model.TransactionVMMessageInput, messageValue model.MsgRun) bool {
	params := vmMessageInput
	if params == nil {
		return true
	}

	if params.MRun == nil {
		return false
	}

	if params.MRun.Caller != nil && deref(params.MRun.Caller) != messageValue.Caller {
		return false
	}

	if params.MRun.Send != nil && deref(params.MRun.Send) != messageValue.Send {
		return false
	}

	if params.MRun.Package != nil {
		if deref(params.MRun.Package.Name) != messageValue.Package.Name {
			return false
		}

		if deref(params.MRun.Package.Path) != messageValue.Package.Path {
			return false
		}
	}

	return true
}
