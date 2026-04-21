package mysql

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/sbset"
)

// tableRowColumnSet holds the table row column set.
type tableRowColumnSet struct {
	r *TableRow
}

// newTableRowColumnSetContainer builds a new column set container.
//
// bcs should be located at the table row, if set.
func newTableRowColumnSetContainer(r *TableRow, bcs *block.Cursor) *sbset.SubBlockSet {
	if r == nil {
		return nil
	}
	if bcs != nil {
		bcs = bcs.FollowSubBlock(1)
	}
	return sbset.NewSubBlockSet(&tableRowColumnSet{r: r}, bcs)
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *tableRowColumnSet) Get(i int) block.SubBlock {
	cols := r.r.GetColumns()
	if len(cols) == 0 || i >= len(cols) {
		return nil
	}
	return cols[i]
}

// Len returns the number of elements.
func (r *tableRowColumnSet) Len() int {
	return len(r.r.GetColumns())
}

// Set sets the value at the index.
func (r *tableRowColumnSet) Set(i int, ref block.SubBlock) {
	if i < 0 || i >= len(r.r.GetColumns()) {
		return
	}
	v, ok := ref.(*TableColumn)
	if ok {
		r.r.Columns[i] = v
	}
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *tableRowColumnSet) Truncate(nlen int) {
	olen := r.Len()
	if nlen < 0 || nlen >= olen {
		return
	}
	if nlen == 0 {
		r.r.Columns = nil
	} else {
		for i := nlen; i < olen; i++ {
			r.r.Columns[i] = nil
		}
		r.r.Columns = r.r.Columns[:nlen]
	}
}

// _ is a type assertion
var _ sbset.SubBlockContainer = ((*tableRowColumnSet)(nil))
