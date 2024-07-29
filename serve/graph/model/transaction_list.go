package model

func NewTransactionList(edges []*TransactionListEdge, hasNext bool) *TransactionList {
	pageInfo := NewPageInfo(nil, nil, hasNext)

	if len(edges) > 0 {
		first := edges[0]
		last := edges[len(edges)-1]
		pageInfo = NewPageInfo(&first.Cursor, &last.Cursor, hasNext)
	}

	return &TransactionList{
		PageInfo: pageInfo,
		Edges:    edges,
	}
}

func NewTransactionListEdge(transaction *Transaction) *TransactionListEdge {
	return &TransactionListEdge{
		Cursor:      NewCursor(transaction.ID()),
		Transaction: transaction,
	}
}
