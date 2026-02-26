package git_world

import (
	"github.com/aperturerobotics/hydra/block"
	git_block "github.com/aperturerobotics/hydra/git/block"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/pkg/errors"
)

// IsNil returns if the object is nil.
func (h *HeadRefStore) IsNil() bool {
	return h == nil
}

// Validate validates the head ref store.
func (h *HeadRefStore) Validate() error {
	if len(h.GetHeadRef().GetName()) != 0 {
		if err := h.GetHeadRef().Validate(); err != nil {
			return errors.Wrap(err, "head_ref")
		}
	}
	submodules := h.GetSubmodules()
	var prevName string
	for idx, subm := range submodules {
		submName := subm.GetSubmoduleName()
		if submName == "" {
			return errors.Errorf("submodules[%d]: empty submodule name", idx)
		}
		if err := subm.Validate(); err != nil {
			return errors.Wrapf(err, "submodules[%d]: %s", idx, submName)
		}
		if prevName != "" && submName <= prevName {
			return errors.Errorf("submodules[%d]: must be sorted: %s <= %s", idx, submName, prevName)
		}
		prevName = submName
	}
	return nil
}

// GetName returns the name of the ref.
func (h *HeadRefStore) GetName() string {
	return h.GetSubmoduleName()
}

// SetReference sets the HEAD reference.
//
// ignores any non-HEAD reference
// returns set, error. if !set, will use default store logic.
func (h *HeadRefStore) SetReference(bcs *block.Cursor, ref *plumbing.Reference) (set bool, err error) {
	if ref.Name() != plumbing.HEAD {
		return false, nil
	}

	nref, err := git_block.NewReference(ref)
	if err != nil {
		return false, err
	}
	sblk := bcs.FollowSubBlock(2)
	h.HeadRef = nref
	sblk.SetBlock(nref, true)
	return true, nil
}

// GetReference returns the reference by name.
//
// if nil, nil is returned, uses default store logic.
func (h *HeadRefStore) GetReference(ref plumbing.ReferenceName) (*plumbing.Reference, error) {
	if ref != plumbing.HEAD || h.GetHeadRef().GetName() == "" {
		return nil, nil
	}

	return h.GetHeadRef().ToReference()
}

// GetSubmoduleStore returns the refs store for a submodule.
func (h *HeadRefStore) GetSubmoduleStore(bcs *block.Cursor, name string) (*HeadRefStore, *block.Cursor, error) {
	subStoreBcs := bcs.FollowSubBlock(3)
	hrsSet := NewHeadRefStoreSet(&h.Submodules, subStoreBcs)
	var nsbHrs *HeadRefStore
	nsb, nsbCs, nsbFound := hrsSet.LookupByName(name)
	if nsbFound {
		var ok bool
		nsbHrs, ok = nsb.(*HeadRefStore)
		if !ok {
			return nil, nil, block.ErrUnexpectedType
		}
	} else {
		// alloc new head ref store set for the submodule
		nsbHrs = &HeadRefStore{SubmoduleName: name}
		h.Submodules = append(h.Submodules, nsbHrs)
		nsbCs = subStoreBcs.FollowSubBlock(uint32(len(h.Submodules) - 1)) //nolint:gosec
		hrsSet.SortNamedRefs()
	}

	// return the store
	return nsbHrs, nsbCs, nil
}

// ClearSubmoduleStore is called if a submodule is deleted.
//
// should not return an error if not found.
func (h *HeadRefStore) ClearSubmoduleStore(bcs *block.Cursor, name string) error {
	subStoreBcs := bcs.FollowSubBlock(3)
	hrsSet := NewHeadRefStoreSet(&h.Submodules, subStoreBcs)
	_, _, _ = hrsSet.DeleteByName(name)
	return nil
}

// _ is a type assertion
var (
	_ block.NamedSubBlock = ((*HeadRefStore)(nil))
)
