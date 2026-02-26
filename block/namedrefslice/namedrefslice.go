package namedrefslice

import (
	"sort"
	"strings"

	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
)

// NamedBlockRef is a named block reference.
type NamedBlockRef interface {
	// GetName returns the name of the ref.
	GetName() string
	// GetRef returns the reference.
	GetRef() *block.BlockRef
	// SetRef sets the reference.
	SetRef(*block.BlockRef)
}

// NamedBlockRefSetContainer is a named block reference container.
type NamedBlockRefSetContainer interface {
	// Get returns the value at the index.
	//
	// Return nil if out of bounds, etc.
	Get(i int) NamedBlockRef
	// Len returns the number of elements.
	Len() int
	// Set sets the value at the index.
	Set(i int, r NamedBlockRef)
	// GetBlockRefCtor returns the block constructor for the referenced block.
	GetBlockRefCtor(i int) block.Ctor
}

// NamedBlockRefSet contains a set of block ref objects with name.
//
// The list is sorted / keyed by name, unique.
type NamedBlockRefSet struct {
	sl  NamedBlockRefSetContainer
	bcs *block.Cursor
}

// NewNamedBlockRefSet constructs a new NamedBlockRefSet from a slice pointer.
//
// also contains an optional block cursor
func NewNamedBlockRefSet(sl NamedBlockRefSetContainer, bcs *block.Cursor) *NamedBlockRefSet {
	return &NamedBlockRefSet{sl: sl, bcs: bcs}
}

// IsNil checks if the object is nil.
func (r *NamedBlockRefSet) IsNil() bool {
	return r == nil
}

// GetCursor returns the sub-block cursor located at r, if set.
func (r *NamedBlockRefSet) GetCursor() *block.Cursor {
	return r.bcs
}

// Len is the number of elements in the collection.
func (r *NamedBlockRefSet) Len() int {
	if r.sl == nil {
		return 0
	}
	return r.sl.Len()
}

// Less reports whether the element with
// index i should sort before the element with index j.
func (r *NamedBlockRefSet) Less(i, j int) bool {
	if r.sl == nil {
		return false
	}
	ls := r.sl.Len()
	if j >= ls {
		return true
	}
	if i >= ls {
		return false
	}
	iv := r.sl.Get(i)
	jv := r.sl.Get(j)
	if iv == nil && jv != nil {
		return false
	}
	if jv == nil && iv != nil {
		return true
	}
	return iv.GetName() < jv.GetName()
}

// Swap swaps the elements with indexes i and j.
func (r *NamedBlockRefSet) Swap(i, j int) {
	if r.sl == nil {
		return
	}
	ls := r.sl.Len()
	if i >= ls || j >= ls {
		return
	}
	iv := r.sl.Get(i)
	if iv == nil {
		return
	}
	jv := r.sl.Get(j)
	if jv == nil {
		return
	}
	// swap block cursor graph references
	if bcs := r.bcs; bcs != nil {
		ii := uint32(i) //nolint:gosec
		ir := bcs.FollowRef(ii, iv.GetRef())
		jj := uint32(j) //nolint:gosec
		jr := bcs.FollowRef(jj, jv.GetRef())
		bcs.SetRef(jj, ir)
		bcs.SetRef(ii, jr)
	}
	// swap positions in the slice
	r.sl.Set(i, jv)
	r.sl.Set(j, iv)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (r *NamedBlockRefSet) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	if id == 0 || r.sl == nil {
		return nil
	}

	idx := int(id)
	ol := r.sl.Len()
	sv := r.sl.Get(idx)
	if idx > ol || sv == nil {
		return errors.New("block ref index out of range")
	}
	sv.SetRef(ptr)
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (r *NamedBlockRefSet) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	if r.sl == nil {
		return nil, nil
	}
	ls := r.sl.Len()
	m := make(map[uint32]*block.BlockRef, ls)
	for i := range ls {
		sv := r.sl.Get(i)
		if sv == nil {
			return nil, errors.Errorf("entry at index %d was nil", i)
		}
		m[uint32(i)] = sv.GetRef() //nolint:gosec
	}
	return m, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (r *NamedBlockRefSet) GetBlockRefCtor(id uint32) block.Ctor {
	if r.sl == nil {
		return nil
	}
	ls := r.sl.Len()
	if id >= uint32(ls) { //nolint:gosec
		return nil
	}
	return r.sl.GetBlockRefCtor(int(id))
}

// LookupIndexByName looks up a database index by name.
//
// Returns either -1 or len(sl) if not found or nil.
func (r *NamedBlockRefSet) LookupIndexByName(name string) (idx int) {
	if r.sl == nil {
		return -1
	}
	ls := r.sl.Len()
	return sort.Search(ls, func(i int) bool {
		iv := r.sl.Get(i)
		if iv == nil {
			return true
		}
		return strings.Compare(iv.GetName(), name) >= 0
	})
}

// SortNamedRefs sorts by name.
func (r *NamedBlockRefSet) SortNamedRefs() {
	sort.Sort(r)
}

// LookupByName looks up a database by name.
//
// Returns bcs located at root_db.ref.
// Returns nil, false if not found.
// Returns nil block cursor if bcs is not set.
func (r *NamedBlockRefSet) LookupByName(name string) (NamedBlockRef, *block.Cursor, bool) {
	if r.sl == nil {
		return nil, nil, false
	}
	idx := r.LookupIndexByName(name)
	ls := r.sl.Len()
	if idx >= ls || idx <= 0 {
		return nil, nil, false
	}
	v := r.sl.Get(idx)
	if v.GetName() != name {
		return nil, nil, false
	}
	var nbcs *block.Cursor
	if r.bcs != nil {
		nbcs = r.bcs.FollowRef(uint32(idx), v.GetRef()) //nolint:gosec
	}
	return v, nbcs, true
}

// BlockPreWriteHook is called when writing the block.
func (r *NamedBlockRefSet) BlockPreWriteHook() error {
	if r != nil {
		r.SortNamedRefs()
	}
	return nil
}

// _ is a type assertion
var (
	_ block.SubBlock              = ((*NamedBlockRefSet)(nil))
	_ block.BlockWithRefs         = ((*NamedBlockRefSet)(nil))
	_ block.BlockWithPreWriteHook = ((*NamedBlockRefSet)(nil))
	_ sort.Interface              = ((*NamedBlockRefSet)(nil))
)
