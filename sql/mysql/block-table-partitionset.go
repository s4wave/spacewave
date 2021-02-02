package mysql

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/sbset"
)

// tableRootPartitionSet holds the root partition set.
type tableRootPartitionSet struct {
	r *TableRoot
}

// newTableRootPartitionSetContainer builds a new db root table set container
func newTableRootPartitionSetContainer(r *TableRoot, bcs *block.Cursor) *sbset.SubBlockSet {
	if r == nil {
		return nil
	}
	return sbset.NewSubBlockSet(&tableRootPartitionSet{r: r}, bcs)
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *tableRootPartitionSet) Get(i int) block.SubBlock {
	parts := r.r.GetTablePartitions()
	if len(parts) == 0 || i >= len(parts) {
		return nil
	}
	return parts[i]
}

// Len returns the number of elements.
func (r *tableRootPartitionSet) Len() int {
	return len(r.r.GetTablePartitions())
}

// Set sets the value at the index.
func (r *tableRootPartitionSet) Set(i int, ref block.SubBlock) {
	if i < 0 || i >= len(r.r.GetTablePartitions()) {
		return
	}
	v, ok := ref.(*TablePartitionRoot)
	if ok {
		r.r.TablePartitions[i] = v
	}
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *tableRootPartitionSet) Truncate(nlen int) {
	olen := r.Len()
	if nlen < 0 || nlen >= olen {
		return
	}
	if nlen == 0 {
		r.r.TablePartitions = nil
	} else {
		for i := nlen; i < olen; i++ {
			r.r.TablePartitions[i] = nil
		}
		r.r.TablePartitions = r.r.TablePartitions[:nlen]
	}
}

// _ is a type assertion
var _ sbset.SubBlockContainer = ((*tableRootPartitionSet)(nil))
