package model

import "math"

// Filters for querying Transactions within specified criteria related to their execution and placement within Blocks.
type TransactionFilter struct {
	// Minimum block height from which to start fetching Transactions, inclusive.
	// Aids in scoping the search to recent Transactions.
	FromBlockHeight *int `json:"fromBlockHeight,omitempty"`
	// Maximum block height up to which Transactions should be fetched, exclusive.
	// Helps in limiting the search to older Transactions.
	ToBlockHeight *int `json:"toBlockHeight,omitempty"`
	// Minimum Transaction index from which to start fetching, inclusive.
	// Facilitates ordering in Transaction queries.
	FromIndex *int `json:"fromIndex,omitempty"`
	// Maximum Transaction index up to which to fetch, exclusive.
	// Ensures a limit on the ordering range for Transaction queries.
	ToIndex *int `json:"toIndex,omitempty"`
	// Minimum `gas_wanted` value to filter Transactions by, inclusive.
	// Filters Transactions based on the minimum computational effort declared.
	FromGasWanted *int `json:"fromGasWanted,omitempty"`
	// Maximum `gas_wanted` value for filtering Transactions, exclusive.
	// Limits Transactions based on the declared computational effort.
	ToGasWanted *int `json:"toGasWanted,omitempty"`
	// Minimum `gas_used` value to filter Transactions by, inclusive.
	// Selects Transactions based on the minimum computational effort actually used.
	FromGasUsed *int `json:"fromGasUsed,omitempty"`
	// Maximum `gas_used` value for filtering Transactions, exclusive.
	// Refines selection based on the computational effort actually consumed.
	ToGasUsed *int `json:"toGasUsed,omitempty"`
	// Hash from Transaction content in base64 encoding.
	// If this filter is used, any other filter will be ignored.
	Hash *string `json:"hash,omitempty"`
	// Transaction's message to filter Transactions.
	Message *TransactionMessageInput `json:"message,omitempty"`
	// `memo` value to filter Transaction's memo.
	Memo *string `json:"memo,omitempty"`
}

func (filter *TransactionFilter) GetFromBlockHeight() uint64 {
	return uint64(Deref(filter.FromBlockHeight))
}

func (filter *TransactionFilter) GetToBlockHeight() uint64 {
	return uint64(Deref(filter.ToBlockHeight))
}

func (filter *TransactionFilter) GetFromIndex() uint32 {
	return uint32(Deref(filter.FromIndex))
}

func (filter *TransactionFilter) GetToIndex() uint32 {
	return uint32(Deref(filter.ToIndex))
}

// Transaction
// for all filter types
func (filter *TransactionFilter) FilterBy(tx *Transaction) bool {
	if filter == nil {
		return true
	}

	if !filter.filterByGasUsed(tx) {
		return false
	}

	if !filter.filterByGasWanted(tx) {
		return false
	}

	if !filter.filterByMemo(tx) {
		return false
	}

	if !filter.filterByMessages(tx) {
		return false
	}

	return true
}

func (filter *TransactionFilter) filterByGasUsed(tx *Transaction) bool {
	fromGasUsed := Deref(filter.FromGasUsed)
	toGasUsed := Deref(filter.ToGasUsed)

	if toGasUsed == 0 {
		toGasUsed = math.MaxInt
	}

	return tx.GasUsed() >= fromGasUsed && tx.GasUsed() <= toGasUsed
}

func (filter *TransactionFilter) filterByGasWanted(tx *Transaction) bool {
	fromGasWanted := Deref(filter.FromGasWanted)
	toGasWanted := Deref(filter.ToGasWanted)

	if toGasWanted == 0 {
		toGasWanted = math.MaxInt
	}

	return tx.GasWanted() >= fromGasWanted && tx.GasWanted() <= toGasWanted
}

func (filter *TransactionFilter) filterByMessages(tx *Transaction) bool {
	if filter.Message == nil {
		return true
	}

	if !filter.filterByMessageRoutes(tx) {
		return false
	}

	if !filter.filterByMessageTypes(tx) {
		return false
	}

	for _, message := range tx.messages {
		if !filter.filterByMessageContent(message) {
			return false
		}
	}

	return true
}

func (filter *TransactionFilter) filterByMessageRoutes(tx *Transaction) bool {
	if filter.Message.Route == nil {
		return true
	}

	for _, message := range tx.messages {
		if message.Route.String() == filter.Message.Route.String() {
			return true
		}
	}

	return false
}

func (filter *TransactionFilter) filterByMessageTypes(tx *Transaction) bool {
	if filter.Message.TypeURL == nil {
		return true
	}

	for _, message := range tx.messages {
		if message.TypeURL.String() == filter.Message.TypeURL.String() {
			return true
		}
	}

	return false
}

func (filter *TransactionFilter) filterByMessageContent(tm *TransactionMessage) bool {
	if filter.Message.TypeURL != nil && filter.Message.TypeURL.String() != tm.TypeURL.String() {
		return false
	}

	if filter.Message.BankParam == nil && filter.Message.VMParam == nil {
		return true
	}

	switch tm.Route {
	case MessageRouteBank:
		if filter.Message.BankParam == nil {
			return false
		}
	case MessageRouteVM:
		if filter.Message.VMParam == nil {
			return false
		}
	}

	switch tm.TypeURL {
	case MessageTypeSend:
		if !filter.filterByMessageOfBankMsgSend(tm.BankMsgSend()) {
			return false
		}
	case MessageTypeExec:
		if !filter.filterByMessageOfMsgCall(tm.VMMsgCall()) {
			return false
		}
	case MessageTypeAddPackage:
		if !filter.filterByMessageOfMsgAddPackage(tm.VMAddPackage()) {
			return false
		}
	case MessageTypeRun:
		if !filter.filterByMessageOfMsgRun(tm.VMMsgRun()) {
			return false
		}
	}

	return true
}

func (filter *TransactionFilter) filterByMemo(tx *Transaction) bool {
	if filter.Memo == nil {
		return true
	}

	return *filter.Memo == tx.Memo()
}

func (filter *TransactionFilter) filterByMessageOfBankMsgSend(messageValue BankMsgSend) bool {
	params := filter.Message.BankParam
	if params == nil || params.Send == nil {
		return true
	}

	if params.Send.Amount != nil && Deref(params.Send.Amount) != messageValue.Amount {
		return false
	}

	if params.Send.FromAddress != nil && Deref(params.Send.FromAddress) != messageValue.FromAddress {
		return false
	}

	if params.Send.ToAddress != nil && Deref(params.Send.ToAddress) != messageValue.ToAddress {
		return false
	}

	return true
}

func (filter *TransactionFilter) filterByMessageOfMsgCall(messageValue MsgCall) bool {
	params := filter.Message.VMParam
	if params == nil {
		return true
	}

	if params.MCall == nil {
		return false
	}

	if params.MCall.Caller != nil && Deref(params.MCall.Caller) != messageValue.Caller {
		return false
	}

	if params.MCall.Func != nil && Deref(params.MCall.Func) != messageValue.Func {
		return false
	}

	if params.MCall.PkgPath != nil && Deref(params.MCall.PkgPath) != messageValue.PkgPath {
		return false
	}

	if params.MCall.Send != nil && Deref(params.MCall.Send) != messageValue.Send {
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

func (filter *TransactionFilter) filterByMessageOfMsgAddPackage(messageValue MsgAddPackage) bool {
	params := filter.Message.VMParam
	if params == nil {
		return true
	}

	if params.MAddpkg == nil {
		return false
	}

	if params.MAddpkg.Creator != nil && Deref(params.MAddpkg.Creator) != messageValue.Creator {
		return false
	}

	if params.MAddpkg.Deposit != nil && Deref(params.MAddpkg.Deposit) != messageValue.Deposit {
		return false
	}

	if params.MAddpkg.Package != nil {
		if params.MAddpkg.Package.Name != nil && Deref(params.MAddpkg.Package.Name) != messageValue.Package.Name {
			return false
		}

		if params.MAddpkg.Package.Path != nil && Deref(params.MAddpkg.Package.Path) != messageValue.Package.Path {
			return false
		}
	}

	return true
}

func (filter *TransactionFilter) filterByMessageOfMsgRun(messageValue MsgRun) bool {
	params := filter.Message.VMParam
	if params == nil {
		return true
	}

	if params.MRun == nil {
		return false
	}

	if params.MRun.Caller != nil && Deref(params.MRun.Caller) != messageValue.Caller {
		return false
	}

	if params.MRun.Send != nil && Deref(params.MRun.Send) != messageValue.Send {
		return false
	}

	if params.MRun.Package != nil {
		if Deref(params.MRun.Package.Name) != messageValue.Package.Name {
			return false
		}

		if Deref(params.MRun.Package.Path) != messageValue.Package.Path {
			return false
		}
	}

	return true
}
