package btree

// IteratorCallback is the iterator callback function.
type IteratorCallback = func(key string) (ctnu bool, err error)

// AscendRange calls the iterator for every value in the tree within the range
// [greaterOrEqual, lessThan), until iterator returns false.
func (t *BTree) AscendRange(greaterOrEqual, lessThan string, iterator IteratorCallback) error {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	if t.rootCursor == nil {
		return nil
	}
	r, rnn, _, _, rootNodeCursor, err := t.fetchRoot()
	if err != nil || r.GetLength() == 0 {
		return err
	}
	_, _, err = t.iterate(rootNodeCursor, rnn, true, false, greaterOrEqual, lessThan, true, iterator)
	return err
}

// AscendLessThan calls the iterator for every value in the tree within the range
// [first, pivot), until iterator returns false.
func (t *BTree) AscendLessThan(pivot string, iterator IteratorCallback) error {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	if t.rootCursor == nil {
		return nil
	}
	r, rnn, _, _, rootNodeCursor, err := t.fetchRoot()
	if err != nil || r.GetLength() == 0 {
		return err
	}
	_, _, err = t.iterate(rootNodeCursor, rnn, true, false, "", pivot, true, iterator)
	return err
}

// AscendGreaterOrEqual calls the iterator for every value in the tree within
// the range [pivot, last], until iterator returns false.
func (t *BTree) AscendGreaterOrEqual(pivot string, iterator IteratorCallback) error {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	if t.rootCursor == nil {
		return nil
	}
	r, rnn, _, _, rootNodeCursor, err := t.fetchRoot()
	if err != nil || r.GetLength() == 0 {
		return err
	}
	_, _, err = t.iterate(rootNodeCursor, rnn, true, false, pivot, "", true, iterator)
	return err
}

// Ascend calls the iterator for every value in the tree within the range
// [first, last], until iterator returns false.
func (t *BTree) Ascend(iterator IteratorCallback) error {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	if t.rootCursor == nil {
		return nil
	}
	r, rnn, _, _, rootNodeCursor, err := t.fetchRoot()
	if err != nil || r.GetLength() == 0 {
		return err
	}
	_, _, err = t.iterate(rootNodeCursor, rnn, true, false, "", "", false, iterator)
	return err
}

// DescendRange calls the iterator for every value in the tree within the range
// [lessOrEqual, greaterThan), until iterator returns false.
func (t *BTree) DescendRange(lessOrEqual, greaterThan string, iterator IteratorCallback) error {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	if t.rootCursor == nil {
		return nil
	}
	r, rnn, _, _, rootNodeCursor, err := t.fetchRoot()
	if err != nil || r.GetLength() == 0 {
		return err
	}
	_, _, err = t.iterate(rootNodeCursor, rnn, false, false, lessOrEqual, greaterThan, true, iterator)
	return err
}

// DescendLessOrEqual calls the iterator for every value in the tree within the range
// [pivot, first], until iterator returns false.
func (t *BTree) DescendLessOrEqual(pivot string, iterator IteratorCallback) error {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	if t.rootCursor == nil {
		return nil
	}
	r, rnn, _, _, rootNodeCursor, err := t.fetchRoot()
	if err != nil || r.GetLength() == 0 {
		return err
	}
	_, _, err = t.iterate(rootNodeCursor, rnn, false, false, pivot, "", true, iterator)
	return err
}

// DescendGreaterThan calls the iterator for every value in the tree within
// the range (pivot, last], until iterator returns false.
func (t *BTree) DescendGreaterThan(pivot string, iterator IteratorCallback) error {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	if t.rootCursor == nil {
		return nil
	}
	r, rnn, _, _, rootNodeCursor, err := t.fetchRoot()
	if err != nil || r.GetLength() == 0 {
		return err
	}
	_, _, err = t.iterate(rootNodeCursor, rnn, false, false, "", pivot, false, iterator)
	return err
}

// Descend calls the iterator for every value in the tree within the range
// [last, first], until iterator returns false.
func (t *BTree) Descend(iterator IteratorCallback) error {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	if t.rootCursor == nil {
		return nil
	}
	r, rnn, _, _, rootNodeCursor, err := t.fetchRoot()
	if err != nil || r.GetLength() == 0 {
		return err
	}
	_, _, err = t.iterate(rootNodeCursor, rnn, false, false, "", "", false, iterator)
	return err
}
