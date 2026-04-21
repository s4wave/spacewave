package world

import "context"

// engineObjectIterator implements ObjectIterator for a Engine.
type engineObjectIterator struct {
	// e is the engine
	e Engine
	// ctx is the context
	ctx context.Context
	// prefix is the prefix to filter by
	prefix string
	// reversed indicates if iteration is reversed
	reversed bool

	// err is any error that occurred
	err error
	// currKey is the current key if valid
	currKey string
	// valid indicates if the iterator is valid
	valid bool
}

// NewEngineObjectIterator constructs a new engine object iterator.
func NewEngineObjectIterator(
	ctx context.Context,
	e Engine,
	prefix string,
	reversed bool,
) *engineObjectIterator {
	return &engineObjectIterator{
		e:        e,
		ctx:      ctx,
		prefix:   prefix,
		reversed: reversed,
	}
}

// Err returns any error that has closed the iterator.
func (e *engineObjectIterator) Err() error {
	return e.err
}

// Valid returns if the iterator points to a valid entry.
func (e *engineObjectIterator) Valid() bool {
	return e.err == nil && e.valid
}

// Key returns the current entry key, or empty string if not valid.
func (e *engineObjectIterator) Key() string {
	if !e.Valid() {
		return ""
	}
	return e.currKey
}

// Next advances to the next entry and returns Valid.
func (e *engineObjectIterator) Next() bool {
	if e.err != nil {
		return false
	}

	var valid bool
	tx, err := e.e.NewTransaction(e.ctx, false)
	if err != nil {
		e.err = err
		e.valid = false
		return false
	}
	defer tx.Discard()

	err = func() error {
		iter := tx.IterateObjects(e.ctx, e.prefix, e.reversed)
		if iter == nil {
			return nil
		}
		defer iter.Close()

		if e.currKey != "" {
			if err := iter.Seek(e.currKey); err != nil {
				return err
			}

			if !iter.Valid() {
				return iter.Err()
			}

			// Check if Seek already moved us past the current key
			if iter.Key() == e.currKey {
				// Still on same key, need to move past it
				if !iter.Next() {
					return iter.Err()
				}
			}
		}

		if iter.Valid() {
			e.currKey = iter.Key()
			valid = true
			return nil
		}

		// Need to move to first valid entry
		if !iter.Next() {
			return iter.Err()
		}

		if !iter.Valid() {
			return iter.Err()
		}

		e.currKey = iter.Key()
		valid = true
		return nil
	}()
	if err != nil {
		e.err = err
		e.valid = false
		return false
	}

	e.valid = valid
	return valid
}

// Seek moves the iterator to the first key >= the provided key (or <= in reverse mode).
func (e *engineObjectIterator) Seek(k string) error {
	if e.err != nil {
		return e.err
	}

	var valid bool
	tx, err := e.e.NewTransaction(e.ctx, false)
	if err != nil {
		e.err = err
		e.valid = false
		return err
	}
	defer tx.Discard()

	err = func() error {
		iter := tx.IterateObjects(e.ctx, e.prefix, e.reversed)
		if iter == nil {
			return nil
		}
		defer iter.Close()

		if err := iter.Seek(k); err != nil {
			return err
		}

		if !iter.Valid() {
			return iter.Err()
		}

		e.currKey = iter.Key()
		valid = true
		return nil
	}()
	if err != nil {
		e.err = err
		e.valid = false
		return err
	}

	e.valid = valid
	return nil
}

// Close releases the iterator.
func (e *engineObjectIterator) Close() {
	e.valid = false
	e.err = context.Canceled
}

// _ is a type assertion
var _ ObjectIterator = ((*engineObjectIterator)(nil))
