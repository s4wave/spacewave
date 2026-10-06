package s4db

// pageIter iterates the index tree as a merge layer.
type pageIter struct {
	cursor
}

// entry returns the current entry.
func (l *pageIter) entry() mentry {
	return mentry{val: l.value()}
}

// _ is a type assertion
var _ layer = (*pageIter)(nil)
