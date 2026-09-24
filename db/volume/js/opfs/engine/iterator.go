package engine

import (
	"bytes"
	"context"
	"sort"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume/workload"
)

// iterator merges one bounded committed partition with bounded pending changes.
// Its transaction retains file protection until Commit or Discard.
type iterator struct {
	// ctx controls storage reads and iteration cancellation.
	ctx context.Context
	// tx owns the pinned generation and lifetime.
	tx *transaction
	// id identifies the iterator in workload records.
	id uint64
	// visited counts positioned entries for the closing workload record.
	visited int64
	// prefix restricts returned full keys.
	prefix []byte
	// reverse selects descending key order.
	reverse bool
	// pending is the ordered mutation snapshot captured at iterator creation.
	pending []*Record
	// pendingIndex selects the next pending record in the chosen direction.
	pendingIndex int
	// committed contains only one copied partition's remaining records.
	committed []*Record
	// boundary marks the last committed entry consumed or the initial seek key.
	boundary []byte
	// exclusive excludes the boundary key after it has been consumed.
	exclusive bool
	// exhausted records the end of committed prefix data.
	exhausted bool
	// started distinguishes initial Next from subsequent advancement.
	started bool
	// closed releases the cursor without changing an existing error.
	closed bool
	// current is the caller-visible merged record.
	current *Record
	// err is the first storage or lifetime failure.
	err error
}

// Err reports a cursor failure or its transaction's invalid lifetime.
func (i *iterator) Err() error {
	if i.err != nil {
		return i.err
	}
	if !i.closed {
		return i.tx.check(i.ctx)
	}
	return nil
}

// Valid reports whether a merged entry is available.
func (i *iterator) Valid() bool {
	return !i.closed && i.current != nil && i.Err() == nil
}

// Key returns the current full key until the next cursor operation.
func (i *iterator) Key() []byte {
	if !i.Valid() {
		return nil
	}
	return i.current.Key
}

// Value returns the copied immutable record value for the current entry.
func (i *iterator) Value() ([]byte, error) {
	if err := i.Err(); err != nil {
		return nil, err
	}
	if !i.Valid() {
		return nil, nil
	}
	return i.current.Value, nil
}

// ValueCopy copies the current value into reusable caller-owned storage.
func (i *iterator) ValueCopy(dst []byte) ([]byte, error) {
	value, err := i.Value()
	if err != nil || !i.Valid() {
		return nil, err
	}
	return append(dst[:0], value...), nil
}

// Next starts or advances the ordered merge.
func (i *iterator) Next() bool {
	if !i.started {
		return i.seek(nil) == nil && i.Valid()
	}
	return i.advance()
}

// Seek positions at the first matching key at or beyond the supplied boundary.
func (i *iterator) Seek(key []byte) error {
	workload.Record{Op: workload.OpSeek, ID: i.id, Parent: i.tx.id, Key: key}.Log(i.ctx)
	return i.seek(key)
}

// seek positions the merge without recording a workload operation.
func (i *iterator) seek(key []byte) error {
	if i.closed {
		return kvtx.ErrDiscarded
	}
	if err := i.tx.check(i.ctx); err != nil {
		i.err = err
		return err
	}
	i.started, i.exhausted = true, false
	i.current, i.committed = nil, nil
	i.boundary, i.exclusive = bytes.Clone(key), false
	if key == nil {
		if i.reverse {
			i.boundary = prefixEnd(i.prefix)
			i.exclusive = i.boundary != nil
		} else {
			i.boundary = bytes.Clone(i.prefix)
		}
	}
	i.pendingIndex = sort.Search(len(i.pending), func(j int) bool {
		comparison := bytes.Compare(i.pending[j].Key, i.boundary)
		if i.reverse || i.exclusive {
			return comparison > 0
		}
		return comparison >= 0
	})
	if i.reverse {
		if i.boundary == nil {
			i.pendingIndex = len(i.pending)
		}
		i.pendingIndex--
		if i.exclusive && i.pendingIndex >= 0 && bytes.Equal(i.pending[i.pendingIndex].Key, i.boundary) {
			i.pendingIndex--
		}
	}
	i.advance()
	return i.Err()
}

// advance chooses the next visible record, with pending mutations winning ties.
func (i *iterator) advance() bool {
	i.current = nil
	if i.closed || i.err != nil {
		return false
	}
	if err := i.tx.check(i.ctx); err != nil {
		i.err = err
		return false
	}
	for {
		if err := i.fill(); err != nil {
			i.err = err
			return false
		}
		var pending, committed *Record
		if i.pendingIndex >= 0 && i.pendingIndex < len(i.pending) {
			pending = i.pending[i.pendingIndex]
		}
		if len(i.committed) != 0 {
			committed = i.committed[0]
		}
		if pending == nil && committed == nil {
			return false
		}
		usePending := committed == nil
		comparison := 0
		if pending != nil && committed != nil {
			comparison = bytes.Compare(pending.Key, committed.Key)
			usePending = comparison <= 0
			if i.reverse {
				usePending = comparison >= 0
			}
		}
		if pending == nil {
			usePending = false
		}
		if committed != nil && (!usePending || (pending != nil && comparison == 0)) {
			i.boundary, i.exclusive = committed.Key, true
			i.committed = i.committed[1:]
		}
		if usePending {
			i.current = pending
			if i.reverse {
				i.pendingIndex--
			} else {
				i.pendingIndex++
			}
		} else {
			i.current = committed
		}
		if !i.current.Deleted {
			i.visited++
			return true
		}
		i.current = nil
	}
}

// fill copies the next partition from the transaction's committed snapshot.
func (i *iterator) fill() error {
	if len(i.committed) != 0 || i.exhausted {
		return nil
	}
	s, err := i.tx.readSnapshot(i.ctx)
	if err != nil {
		return err
	}
	records, err := s.seekEntries(i.ctx, i.boundary, i.exclusive, i.reverse)
	if err != nil {
		return err
	}
	for _, record := range records {
		if !bytes.HasPrefix(record.Key, i.prefix) {
			i.exhausted = true
			break
		}
		i.committed = append(i.committed, record)
	}
	if len(i.committed) == 0 {
		i.exhausted = true
	}
	return nil
}

// Close drops the copied partition and pending cursor state.
func (i *iterator) Close() {
	if !i.closed {
		workload.Record{Op: workload.OpIterEnd, ID: i.id, Parent: i.tx.id, Size: i.visited}.Log(i.ctx)
	}
	i.closed = true
	i.current, i.committed, i.pending = nil, nil, nil
}

// prefixEnd returns the exclusive upper bound, or nil for an unbounded prefix.
func prefixEnd(prefix []byte) []byte {
	end := bytes.Clone(prefix)
	for j := len(end) - 1; j >= 0; j-- {
		if end[j] != 255 {
			end[j]++
			return end[:j+1]
		}
	}
	return nil
}

// _ verifies the lazy iterator contract.
var _ kvtx.Iterator = (*iterator)(nil)
