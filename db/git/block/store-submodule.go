package git_block

import (
	"errors"

	"github.com/go-git/go-git/v6/storage"
	"github.com/go-git/go-git/v6/storage/memory"
	"github.com/s4wave/spacewave/db/block"
)

// SetModuleReference sets the module reference to the Repo rooted at bcs.
func (r *Store) SetModuleReference(name string, bcs *block.Cursor) error {
	// Require a submodule name before storing its repository reference.
	if len(name) == 0 {
		return ErrReferenceNameEmpty
	}

	// Resolve the submodule name to its reference tree key.
	key, err := r.buildRefKey(name)
	if err != nil {
		return err
	}

	// Attach the submodule repository cursor to its reference record.
	modRefTree := r.modTree
	rootCs := modRefTree.GetCursor()
	refCs := rootCs.Detach(false)
	refCs.ClearAllRefs()
	refCs.SetBlock(NewSubmodule(name, bcs.GetRef()), true)
	refCs.SetRef(2, bcs)
	return modRefTree.SetCursorAtKey(r.ctx, key, refCs, false)
}

// LookupSubmodule looks up module reference by name.
// Returns nil, nil, nil if not found.
func (r *Store) LookupSubmodule(name string) (*Submodule, *block.Cursor, error) {
	// Require a submodule name before looking up its reference.
	if len(name) == 0 {
		return nil, nil, ErrReferenceNameEmpty
	}

	// Resolve the submodule name to its reference tree key.
	key, err := r.buildRefKey(name)
	if err != nil {
		return nil, nil, err
	}

	// Find the submodule reference cursor in the Store.
	modRefTree := r.modTree
	refCs, err := modRefTree.GetCursorAtKey(r.ctx, key)
	if err != nil {
		return nil, nil, err
	}
	if refCs == nil {
		return nil, nil, err
	}

	// Decode the submodule reference at its stored cursor.
	sub, err := block.UnmarshalBlock[*Submodule](r.ctx, refCs, NewSubmoduleBlock)
	return sub, refCs, err
}

// Module returns a Storer representing a submodule, if not exists returns a new
// empty Storer is returned
func (r *Store) Module(name string) (storage.Storer, error) {
	// Look up the submodule reference before opening its repository.
	subm, submCs, err := r.LookupSubmodule(name)
	if err != nil {
		return nil, err
	}

	// Initialize the repository root for a missing submodule.
	var repoRootCs *block.Cursor
	if subm == nil {
		// Create the missing submodule reference and verify it can be reopened.
		if err := r.SetModuleReference(name, nil); err != nil {
			return nil, err
		}
		subm, submCs, err = r.LookupSubmodule(name)
		if err != nil {
			return nil, err
		}
		if subm == nil || submCs == nil {
			return nil, errors.New("failed to create submodule")
		}

		// Initialize the new submodule repository root.
		repoRootCs = submCs.FollowRef(2, nil)
		subm.RepoRef = nil
		nrepo := NewRepo()
		repoRootCs.SetBlock(nrepo, true)
	}

	// Follow the stored repository root for an existing submodule.
	if repoRootCs == nil {
		repoRootCs = submCs.FollowRef(2, subm.GetRepoRef())
	}

	// Obtain the submodule reference store when the parent has one.
	var refStore ReferenceStore
	if r.refStore != nil {
		// TODO: when to call ClearSubmoduleStore?
		refStore, err = r.refStore.GetSubmoduleStore(name)
		if err != nil {
			return nil, err
		}
	}

	// Open and retain the submodule Store for ordered commits.
	sub, err := NewStore(r.ctx, r.btx, repoRootCs, &memory.IndexStorage{}, refStore)
	if err != nil {
		return nil, err
	}
	r.subStores = append(r.subStores, sub)
	return sub, nil
}

// _ is a type assertion
var (
	// ModuleStorer stores information about submodules.
	// Submodules are represented as references to other Repo DAGs.
	_ storage.ModuleStorer = (*Store)(nil)
)
