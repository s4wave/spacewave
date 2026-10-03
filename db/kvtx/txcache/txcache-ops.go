package kvtx_txcache

import (
	"bytes"
	"context"
	"sort"

	"github.com/s4wave/spacewave/db/kvtx"
)

// Op performs an operation on the TxOps
type Op func(ops kvtx.TxOps) error

// opsSorter sorts ops
type opsSorter struct {
	opsSet  []Op
	opsKeys [][]byte
}

// Len is the number of elements in the collection.
func (s *opsSorter) Len() int {
	return len(s.opsSet)
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (s *opsSorter) Less(i, j int) bool {
	return bytes.Compare(s.opsKeys[i], s.opsKeys[j]) == -1
}

// Swap swaps the elements with indexes i and j.
func (s *opsSorter) Swap(i, j int) {
	// Exchange each transaction operation together with its sorting key.
	vj := s.opsSet[j]
	kj := s.opsKeys[j]
	s.opsSet[j] = s.opsSet[i]
	s.opsKeys[j] = s.opsKeys[i]
	s.opsSet[i] = vj
	s.opsKeys[i] = kj
}

// _ is a type assertion
var _ sort.Interface = (*opsSorter)(nil)

// BuildOps returns a sorted set of operations.
func (t *TXCache) BuildOps(ctx context.Context, sorted bool) ([]Op, error) {
	// Reserve operation storage and optional keys for sorting the transaction changes.
	opsSet := make([]Op, 0, t.set.Len()+t.remove.Len())
	var opsKeys [][]byte
	if sorted {
		opsKeys = make([][]byte, 0, cap(opsSet))
	}

	// Build delete operations for the pending transaction tombstones.
	t.remove.Ascend(nil, func(item *cacheItem) bool {
		removedKey := item.key
		opsSet = append(opsSet, func(ops kvtx.TxOps) error {
			return ops.Delete(ctx, removedKey)
		})
		if sorted {
			opsKeys = append(opsKeys, removedKey)
		}
		return true
	})

	// Build write operations for pending values without tombstones.
	t.set.Ascend(nil, func(item *cacheItem) bool {
		// Exclude pending writes whose keys have transaction tombstones.
		addedKey := item.key
		if _, ok := t.remove.Get(&cacheItem{key: addedKey}); ok {
			return true
		}

		// Capture the pending value in a write operation and retain its sorting key.
		addedVal := item.val
		opsSet = append(opsSet, func(ops kvtx.TxOps) error {
			return ops.Set(ctx, addedKey, addedVal)
		})
		if sorted {
			opsKeys = append(opsKeys, item.key)
		}
		return true
	})

	// Order the transaction operations by key when requested.
	if sorted {
		sort.Sort(&opsSorter{opsSet: opsSet, opsKeys: opsKeys})
	}
	return opsSet, nil
}
