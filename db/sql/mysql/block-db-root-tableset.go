package mysql

import (
	"github.com/s4wave/spacewave/db/block"
	namedsbset "github.com/s4wave/spacewave/db/block/sbset"
)

// rootDbsSetContainer maps named ref slice to root dbs list.
type dbRootTablesSetContainer struct {
	r *DatabaseRoot
}

// newDbRootTableSetContainer builds a new db root table set container
func newDbRootTableSetContainer(r *DatabaseRoot, bcs *block.Cursor) *namedsbset.NamedSubBlockSet {
	if r == nil {
		return nil
	}
	return namedsbset.NewNamedSubBlockSet(&dbRootTablesSetContainer{r: r}, bcs)
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *dbRootTablesSetContainer) Get(i int) namedsbset.NamedSubBlock {
	tables := r.r.GetTables()
	if len(tables) == 0 || i >= len(tables) {
		return nil
	}
	return tables[i]
}

// Len returns the number of elements.
func (r *dbRootTablesSetContainer) Len() int {
	return len(r.r.GetTables())
}

// Set sets the value at the index.
func (r *dbRootTablesSetContainer) Set(i int, ref namedsbset.NamedSubBlock) {
	if i < 0 || i >= len(r.r.Tables) {
		return
	}
	v, ok := ref.(*DatabaseRootTable)
	if ok {
		r.r.Tables[i] = v
	}
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *dbRootTablesSetContainer) Truncate(nlen int) {
	olen := r.Len()
	if nlen < 0 || nlen >= olen {
		return
	}
	if nlen == 0 {
		r.r.Tables = nil
	} else {
		for i := nlen; i < olen; i++ {
			r.r.Tables[i] = nil
		}
		r.r.Tables = r.r.Tables[:nlen]
	}
}

// _ is a type assertion
var _ namedsbset.NamedSubBlockContainer = ((*dbRootTablesSetContainer)(nil))
