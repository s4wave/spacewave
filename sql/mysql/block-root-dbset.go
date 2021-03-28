package mysql

import (
	"github.com/aperturerobotics/hydra/block"
	namedsbset "github.com/aperturerobotics/hydra/block/sbset"
)

// rootDbsSetContainer maps named ref slice to root dbs list.
type rootDbsSetContainer struct {
	r *Root
}

// newRootDbsSetContainer builds a new named ref slice subobject
func newRootDbsSetContainer(r *Root, bcs *block.Cursor) *namedsbset.NamedSubBlockSet {
	if r == nil {
		return nil
	}
	return namedsbset.NewNamedSubBlockSet(&rootDbsSetContainer{r: r}, bcs)
}

// Get returns the value at the index.
//
// Return nil if out of bounds, etc.
func (r *rootDbsSetContainer) Get(i int) namedsbset.NamedSubBlock {
	dbs := r.r.GetDatabases()
	if len(dbs) == 0 || i >= len(dbs) {
		return nil
	}
	return dbs[i]
}

// Len returns the number of elements.
func (r *rootDbsSetContainer) Len() int {
	return len(r.r.GetDatabases())
}

// Set sets the value at the index.
func (r *rootDbsSetContainer) Set(i int, ref namedsbset.NamedSubBlock) {
	if i < 0 || i >= len(r.r.Databases) {
		return
	}
	v, ok := ref.(*RootDb)
	if ok {
		r.r.Databases[i] = v
	}
}

// Truncate reduces the length to the given len.
//
// If nlen >= len, does nothing.
func (r *rootDbsSetContainer) Truncate(nlen int) {
	olen := r.Len()
	if nlen < 0 || nlen >= olen {
		return
	}
	if nlen == 0 {
		r.r.Databases = nil
	} else {
		for i := nlen; i < olen; i++ {
			r.r.Databases[i] = nil
		}
		r.r.Databases = r.r.Databases[:nlen]
	}
}

// _ is a type assertion
var _ namedsbset.NamedSubBlockContainer = ((*rootDbsSetContainer)(nil))
