package kvtx_vlogger

import (
	"sync"
	"time"

	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/sirupsen/logrus"
)

// Iterator implements the kvtx vlogger iterator.
type Iterator struct {
	closeOnce sync.Once
	le        *logrus.Entry
	ii        uint32
	ta        time.Time
	it        kvtx.Iterator
}

// NewIterator constructs a new Iterator.
func NewIterator(le *logrus.Entry, ii uint32, it kvtx.Iterator) *Iterator {
	return &Iterator{
		le: le,
		ii: ii,
		ta: time.Now(),
		it: it,
	}
}

// Err returns any error that has closed the iterator.
// May return context.Canceled if closed.
func (i *Iterator) Err() error {
	err := i.it.Err()
	i.le.Debugf(
		"Err() => %v",
		err,
	)
	return err
}

// Valid returns if the iterator points to a valid entry.
//
// If err is set, returns false.
func (i *Iterator) Valid() bool {
	v := i.it.Valid()
	i.le.Debugf(
		"Valid() => %v",
		v,
	)
	return v
}

// Key returns the current entry key, or nil if not valid.
func (i *Iterator) Key() []byte {
	k := i.it.Key()
	i.le.Debugf(
		"Key() => %s",
		keyForLogging(k),
	)
	return k
}

// Value returns the current entry value, or nil if not valid.
//
// May cache the value between calls, copy if modifying.
func (i *Iterator) Value() ([]byte, error) {
	v, err := i.it.Value()
	if err != nil {
		i.le.Warnf("Value() => err(%v)", err)
		return nil, err
	}
	i.le.Debugf(
		"Value() => len(%v)",
		len(v),
	)
	return v, nil
}

// ValueCopy copies the key to the given byte slice and returns it.
// If the slice is not big enough (cap), it must create a new one and return it.
// May use the value cached from Value() call as the source of the data.
// May return nil if !Valid().
func (i *Iterator) ValueCopy(bt []byte) ([]byte, error) {
	v, err := i.it.ValueCopy(bt)
	i.le.Debugf(
		"ValueCopy(cap(%d) len(%d)) => len(%v) err(%v)",
		cap(bt), len(bt),
		len(v),
		err,
	)
	return v, err
}

// Next advances to the next entry and returns Valid.
func (i *Iterator) Next() bool {
	v := i.it.Next()
	i.le.Debugf(
		"Next() => %v",
		v,
	)
	return v
}

// Seek moves the iterator to the selected key, or the next key after the key.
// Pass nil to seek to the beginning (or end if reversed).
func (i *Iterator) Seek(k []byte) error {
	logKey := keyForLogging(k)
	i.le.Debugf(
		"Seek(%s)",
		logKey,
	)
	err := i.it.Seek(k)
	if err != nil {
		i.le.Warnf("Seek(%s) => err(%s)", logKey, err.Error())
	}
	return err
}

// Close closes the iterator.
// Note: it is not necessary to close all iterators before Discard().
func (i *Iterator) Close() {
	i.closeOnce.Do(func() {
		i.le.Debug("Close()")
	})
	i.it.Close()
}

// _ is a type assertion
var _ kvtx.Iterator = ((*Iterator)(nil))
