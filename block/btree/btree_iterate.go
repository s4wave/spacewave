package btree

// IteratorCallback is the iterator callback function.
type IteratorCallback = func(key, val []byte) (ctnu bool, err error)

// ScanPrefix iterates over keys with a prefix.
func (t *Tx) ScanPrefix(prefix []byte, cb func(key, val []byte) error) error {
	it := func(key, val []byte) (ctnu bool, err error) {
		err = cb(key, val)
		ctnu = err == nil
		return
	}
	if len(prefix) == 0 {
		return t.Ascend(it)
	}

	endRange := make([]byte, len(prefix)+1)
	copy(endRange, prefix)
	endRange[len(prefix)] = ^byte(0)
	return t.AscendRange(prefix, endRange, it)
}

// AscendRange calls the iterator for every value in the tree within the range
// [greaterOrEqual, lessThan), until iterator returns false.
func (t *Tx) AscendRange(greaterOrEqual, lessThan []byte, iterator IteratorCallback) error {
	if t.b.rootCursor == nil {
		return nil
	}
	if t.rn.GetLength() == 0 {
		return nil
	}
	_, _, err := t.iterate(
		t.baseNodCursor,
		t.baseNod,
		true,
		false,
		greaterOrEqual,
		lessThan,
		true,
		iterator,
	)
	return err
}

// AscendLessThan calls the iterator for every value in the tree within the range
// [first, pivot), until iterator returns false.
func (t *Tx) AscendLessThan(pivot []byte, iterator IteratorCallback) error {
	if t.b.rootCursor == nil {
		return nil
	}
	if t.rn.GetLength() == 0 {
		return nil
	}
	_, _, err := t.iterate(
		t.baseNodCursor,
		t.baseNod,
		true,
		false,
		nil,
		pivot,
		true,
		iterator,
	)
	return err
}

// AscendGreaterOrEqual calls the iterator for every value in the tree within
// the range [pivot, last], until iterator returns false.
func (t *Tx) AscendGreaterOrEqual(pivot []byte, iterator IteratorCallback) error {
	if t.b.rootCursor == nil {
		return nil
	}
	if t.rn.GetLength() == 0 {
		return nil
	}
	_, _, err := t.iterate(
		t.baseNodCursor,
		t.baseNod,
		true,
		false,
		pivot,
		nil,
		true,
		iterator,
	)
	return err
}

// Ascend calls the iterator for every value in the tree within the range
// [first, last], until iterator returns false.
func (t *Tx) Ascend(iterator IteratorCallback) error {
	if t.b.rootCursor == nil {
		return nil
	}
	if t.rn.GetLength() == 0 {
		return nil
	}
	_, _, err := t.iterate(
		t.baseNodCursor,
		t.baseNod,
		true,
		false,
		nil,
		nil,
		false,
		iterator,
	)
	return err
}

// DescendRange calls the iterator for every value in the tree within the range
// [lessOrEqual, greaterThan), until iterator returns false.
func (t *Tx) DescendRange(lessOrEqual, greaterThan []byte, iterator IteratorCallback) error {
	if t.b.rootCursor == nil {
		return nil
	}
	if t.rn.GetLength() == 0 {
		return nil
	}
	_, _, err := t.iterate(
		t.baseNodCursor,
		t.baseNod,
		false,
		false,
		lessOrEqual,
		greaterThan,
		true,
		iterator,
	)
	return err
}

// DescendLessOrEqual calls the iterator for every value in the tree within the range
// [pivot, first], until iterator returns false.
func (t *Tx) DescendLessOrEqual(pivot []byte, iterator IteratorCallback) error {
	if t.b.rootCursor == nil {
		return nil
	}
	if t.rn.GetLength() == 0 {
		return nil
	}
	_, _, err := t.iterate(
		t.baseNodCursor,
		t.baseNod,
		false,
		false,
		pivot,
		nil,
		true,
		iterator,
	)
	return err
}

// DescendGreaterThan calls the iterator for every value in the tree within
// the range (pivot, last], until iterator returns false.
func (t *Tx) DescendGreaterThan(pivot []byte, iterator IteratorCallback) error {
	if t.b.rootCursor == nil {
		return nil
	}
	if t.rn.GetLength() == 0 {
		return nil
	}
	_, _, err := t.iterate(
		t.baseNodCursor,
		t.baseNod,
		false,
		false,
		nil,
		pivot,
		false,
		iterator,
	)
	return err
}

// Descend calls the iterator for every value in the tree within the range
// [last, first], until iterator returns false.
func (t *Tx) Descend(iterator IteratorCallback) error {
	if t.b.rootCursor == nil {
		return nil
	}
	if t.rn.GetLength() == 0 {
		return nil
	}
	_, _, err := t.iterate(
		t.baseNodCursor,
		t.baseNod,
		false,
		false,
		nil,
		nil,
		false,
		iterator,
	)
	return err
}
