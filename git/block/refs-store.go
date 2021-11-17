package git_block

import "github.com/go-git/go-git/v5/plumbing"

// ReferenceStore stores and retrieves references, overriding the default store.
type ReferenceStore interface {
	// SetReference sets the reference.
	//
	// returns set, error. if !set, will use default store logic.
	SetReference(ref *plumbing.Reference) (set bool, err error)
	// GetReference returns the reference by name.
	//
	// if nil, nil is returned, uses default store logic.
	GetReference(ref plumbing.ReferenceName) (*plumbing.Reference, error)
	// GetSubmoduleStore returns the refs store for a submodule.
	//
	// Can return nil, nil to indicate none.
	GetSubmoduleStore(name string) (ReferenceStore, error)
	// ClearSubmoduleStore is called if a submodule is deleted.
	//
	// should not return an error if not found.
	ClearSubmoduleStore(name string) error
}
