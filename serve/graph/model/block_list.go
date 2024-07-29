package model

func NewBlockList(edges []*BlockListEdge, hasNext bool) *BlockList {
	pageInfo := NewPageInfo(nil, nil, hasNext)

	if len(edges) > 0 {
		first := edges[0]
		last := edges[len(edges)-1]
		pageInfo = NewPageInfo(&first.Cursor, &last.Cursor, hasNext)
	}

	return &BlockList{
		PageInfo: pageInfo,
		Edges:    edges,
	}
}

func NewBlockListEdge(block *Block) *BlockListEdge {
	return &BlockListEdge{
		Cursor: NewCursor(block.ID()),
		Block:  block,
	}
}
