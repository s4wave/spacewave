package git_world

import (
	"github.com/aperturerobotics/hydra/block"
	git_block "github.com/aperturerobotics/hydra/git/block"
	"github.com/go-git/go-git/v6/plumbing"
)

// HeadRefStoreCursor implements ReferencesStore with a HeadRefStore.
type HeadRefStoreCursor struct {
	bcs *block.Cursor
	hrs *HeadRefStore
}

// NewHeadRefStoreCursor builds an new worktree-backed ReferencesStore.
// Stores HEAD only.
func NewHeadRefStoreCursor(bcs *block.Cursor, hrs *HeadRefStore) *HeadRefStoreCursor {
	return &HeadRefStoreCursor{bcs: bcs, hrs: hrs}
}

// SetReference sets the HEAD reference.
//
// ignores any non-HEAD reference
// returns set, error. if !set, will use default store logic.
func (r *HeadRefStoreCursor) SetReference(ref *plumbing.Reference) (set bool, err error) {
	return r.hrs.SetReference(r.bcs, ref)
}

// GetReference returns the reference by name.
//
// if nil, nil is returned, uses default store logic.
func (r *HeadRefStoreCursor) GetReference(ref plumbing.ReferenceName) (*plumbing.Reference, error) {
	return r.hrs.GetReference(ref)
}

// GetSubmoduleStore returns the refs store for a submodule.
//
// Can return nil, nil to indicate none.
func (r *HeadRefStoreCursor) GetSubmoduleStore(name string) (git_block.ReferenceStore, error) {
	sstore, sstoreCs, err := r.hrs.GetSubmoduleStore(r.bcs, name)
	if err != nil || sstore == nil {
		return nil, err
	}
	return NewHeadRefStoreCursor(sstoreCs, sstore), nil
}

// ClearSubmoduleStore is called if a submodule is deleted.
//
// should not return an error if not found.
func (r *HeadRefStoreCursor) ClearSubmoduleStore(name string) error {
	return r.hrs.ClearSubmoduleStore(r.bcs, name)
}

// _ is a type assertion
var _ git_block.ReferenceStore = ((*HeadRefStoreCursor)(nil))
