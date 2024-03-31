package model

import "math"

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
	if !filter.hasMessageTypes(tx) {
		return false
	}
	for _, message := range tx.messages {
		if !filter.filterByMessageContent(message) {
			return false
		}
	}
	return true
}

func (filter *TransactionFilter) hasMessageTypes(tx *Transaction) bool {
	if filter.Message.TypeURL == nil {
		return true
	}
	for _, message := range tx.messages {
		if message.TypeUrl.String() == filter.Message.TypeURL.String() {
			return true
		}
	}
	return false
}

func (filter *TransactionFilter) filterByMessageContent(tm *TransactionMessage) bool {
	if filter.Message.TypeURL != nil && filter.Message.TypeURL.String() != tm.TypeUrl.String() {
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

	switch tm.TypeUrl {
	case MessageTypeSend:
		if !filter.validMessageOfBankMsgSend(tm.BankMsgSend()) {
			return false
		}
	case MessageTypeExec:
		if !filter.validMessageOfMsgCall(tm.VmMsgCall()) {
			return false
		}
	case MessageTypeAddPackage:
		if !filter.validMessageOfMsgAddPackage(tm.VmAddPackage()) {
			return false
		}
	case MessageTypeRun:
		if !filter.validMessageOfMsgRun(tm.VmMsgRun()) {
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

func (filter *TransactionFilter) validMessageOfBankMsgSend(messageValue BankMsgSend) bool {
	params := filter.Message.BankParam
	if params == nil || params.Send == nil {
		return true
	}
	if params.Send.Amount != nil && *params.Send.Amount != messageValue.Amount {
		return false
	}
	if params.Send.FromAddress != nil && *params.Send.FromAddress != messageValue.FromAddress {
		return false
	}
	if params.Send.ToAddress != nil && *params.Send.ToAddress != messageValue.ToAddress {
		return false
	}
	return true
}

func (filter *TransactionFilter) validMessageOfMsgCall(messageValue MsgCall) bool {
	params := filter.Message.VMParam
	if params == nil {
		return true
	}
	if params.MCall == nil {
		return false
	}
	if params.MCall.Caller != nil && *params.MCall.Caller != messageValue.Caller {
		return false
	}
	if params.MCall.Func != nil && *params.MCall.Func != messageValue.Func {
		return false
	}
	if params.MCall.PkgPath != nil && *params.MCall.PkgPath != messageValue.PkgPath {
		return false
	}
	if params.MCall.Send != nil && *params.MCall.Send != messageValue.Send {
		return false
	}
	if params.MCall.Args != nil {
		messageArgs := messageValue.Args
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

func (filter *TransactionFilter) validMessageOfMsgAddPackage(messageValue MsgAddPackage) bool {
	params := filter.Message.VMParam
	if params == nil {
		return true
	}
	if params.MAddpkg == nil {
		return false
	}
	if params.MAddpkg.Creator != nil && *params.MAddpkg.Creator != messageValue.Creator {
		return false
	}
	if params.MAddpkg.Deposit != nil && *params.MAddpkg.Deposit != messageValue.Deposit {
		return false
	}
	if params.MAddpkg.Package != nil {
		if params.MAddpkg.Package.Name != nil && *params.MAddpkg.Package.Name != messageValue.Package.Name {
			return false
		}
		if params.MAddpkg.Package.Path != nil && *params.MAddpkg.Package.Path != messageValue.Package.Path {
			return false
		}
	}
	return true
}

func (filter *TransactionFilter) validMessageOfMsgRun(messageValue MsgRun) bool {
	params := filter.Message.VMParam
	if params == nil {
		return true
	}
	if params.MRun == nil {
		return false
	}
	if params.MRun.Caller != nil && *params.MRun.Caller != messageValue.Caller {
		return false
	}
	if params.MRun.Send != nil && *params.MRun.Send != messageValue.Send {
		return false
	}
	if params.MRun.Package != nil {
		if params.MRun.Package.Name != nil && *params.MRun.Package.Name != messageValue.Package.Name {
			return false
		}
		if params.MRun.Package.Path != nil && *params.MRun.Package.Path != messageValue.Package.Path {
			return false
		}
	}
	return true
}
