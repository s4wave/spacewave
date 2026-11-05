package s4db

// pageIter iterates the index tree or a delta as a merge layer.
type pageIter struct {
	cursor
}

// entry returns the current entry.
func (l *pageIter) entry() mentry {
	v := l.value()
	return mentry{del: v.del, val: v}
}

// _ is a type assertion
var _ layer = (*pageIter)(nil)
