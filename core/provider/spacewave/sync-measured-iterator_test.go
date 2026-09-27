package provider_spacewave

import "github.com/s4wave/spacewave/db/kvtx"

// syncMeasuredIterator counts consumed values while preserving cursor behavior.
type syncMeasuredIterator struct {
	// iterator traverses the actual backend.
	iterator kvtx.Iterator
	// tx accounts for each consumed value.
	tx *syncMeasuredTx
}

// Err returns the backend's terminal error.
func (i *syncMeasuredIterator) Err() error { return i.iterator.Err() }

// Valid reports whether the cursor names an entry.
func (i *syncMeasuredIterator) Valid() bool { return i.iterator.Valid() }

// Key returns the current borrowed key.
func (i *syncMeasuredIterator) Key() []byte { return i.iterator.Key() }

// Value counts the consumed marker value.
func (i *syncMeasuredIterator) Value() ([]byte, error) {
	i.tx.visit(i.iterator.Key())
	return i.iterator.Value()
}

// ValueCopy counts the copied marker value.
func (i *syncMeasuredIterator) ValueCopy(dst []byte) ([]byte, error) {
	i.tx.visit(i.iterator.Key())
	return i.iterator.ValueCopy(dst)
}

// Next advances the backend cursor.
func (i *syncMeasuredIterator) Next() bool { return i.iterator.Next() }

// Seek positions the backend cursor.
func (i *syncMeasuredIterator) Seek(key []byte) error { return i.iterator.Seek(key) }

// Close releases the backend cursor.
func (i *syncMeasuredIterator) Close() { i.iterator.Close() }

// _ is a type assertion.
var _ kvtx.Iterator = (*syncMeasuredIterator)(nil)
