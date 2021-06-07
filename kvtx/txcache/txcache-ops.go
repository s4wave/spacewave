package kvtx_txcache

import (
	"bytes"
	"sort"

	"github.com/aperturerobotics/hydra/kvtx"
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
	vj := s.opsSet[j]
	kj := s.opsKeys[j]
	s.opsSet[j] = s.opsSet[i]
	s.opsKeys[j] = s.opsKeys[i]
	s.opsSet[i] = vj
	s.opsKeys[i] = kj
}

// _ is a type assertion
var _ sort.Interface = ((*opsSorter)(nil))

// BuildOps returns a sorted set of operations.
func (t *TXCache) BuildOps(sorted bool) ([]Op, error) {
	t.mtx.RLock()
	snapRemove := t.remove.ReadOnlySnapshot()
	snapSet := t.set.ReadOnlySnapshot()
	t.mtx.RUnlock()

	opsSet := make([]Op, 0, snapSet.Size()+snapRemove.Size())
	var opsKeys [][]byte
	if sorted {
		opsKeys = make([][]byte, 0, cap(opsSet))
	}

	removeIter := snapRemove.Iterator(nil)
	for removed := range removeIter {
		removedKey := removed.Key
		opsSet = append(opsSet, func(ops kvtx.TxOps) error {
			return ops.Delete(removedKey)
		})
		if sorted {
			opsKeys = append(opsKeys, removedKey)
		}
	}

	setIter := snapSet.Iterator(nil)
	for added := range setIter {
		addedKey := added.Key
		if _, ok := snapRemove.Lookup(addedKey); ok {
			continue
		}
		addedVal := added.Value.([]byte)
		opsSet = append(opsSet, func(ops kvtx.TxOps) error {
			return ops.Set(addedKey, addedVal)
		})
		if sorted {
			opsKeys = append(opsKeys, added.Key)
		}
	}

	if sorted {
		sort.Sort(&opsSorter{opsSet: opsSet, opsKeys: opsKeys})
	}
	return opsSet, nil
}
