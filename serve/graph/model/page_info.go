package model

func NewPageInfo(first, last *Cursor, hasNext bool) *PageInfo {
	return &PageInfo{
		First:   first,
		Last:    last,
		HasNext: hasNext,
	}
}
