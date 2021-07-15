package git_block

import (
	"errors"

	"github.com/aperturerobotics/hydra/block"
	"github.com/go-git/go-git/v5/storage"
)

// SetModuleReference sets the module reference to the Repo rooted at bcs.
func (r *Store) SetModuleReference(name string, bcs *block.Cursor) error {
	if len(name) == 0 {
		return ErrReferenceNameEmpty
	}

	key, err := r.buildRefKey(name)
	if err != nil {
		return err
	}

	modRefTree := r.modTree
	rootCs := modRefTree.GetCursor()
	refCs := rootCs.Detach(false)
	refCs.ClearAllRefs()
	refCs.SetBlock(NewSubmodule(name, bcs.GetRef()), true)
	refCs.SetRef(2, bcs)
	return modRefTree.SetCursorAtKey(key, refCs, false)
}

// LookupSubmodule looks up module reference by name.
// Returns nil, nil, nil if not found.
func (r *Store) LookupSubmodule(name string) (*Submodule, *block.Cursor, error) {
	if len(name) == 0 {
		return nil, nil, ErrReferenceNameEmpty
	}

	key, err := r.buildRefKey(name)
	if err != nil {
		return nil, nil, err
	}

	modRefTree := r.modTree
	refCs, err := modRefTree.GetCursorAtKey(key)
	if err != nil {
		return nil, nil, err
	}
	if refCs == nil {
		return nil, nil, err
	}

	subBlki, err := refCs.Unmarshal(NewSubmoduleBlock)
	if err != nil {
		return nil, refCs, err
	}
	sub, ok := subBlki.(*Submodule)
	if !ok {
		err = block.ErrUnexpectedType
	}
	return sub, refCs, err
}

// Module returns a Storer representing a submodule, if not exists returns a new
// empty Storer is returned
func (r *Store) Module(name string) (storage.Storer, error) {
	subm, submCs, err := r.LookupSubmodule(name)
	if err != nil {
		return nil, err
	}
	var repoRootCs *block.Cursor
	if subm == nil {
		// create submodule
		// use a somewhat round-about method to double-check our work
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
		// initialize the submodule storer
		repoRootCs = submCs.FollowRef(2, nil)
		subm.RepoRef = nil
		nrepo := NewRepo()
		repoRootCs.SetBlock(nrepo, true)
	}

	if repoRootCs == nil {
		repoRootCs = submCs.FollowRef(2, subm.GetRepoRef())
	}

	return NewStore(r.ctx, r.btx, repoRootCs, r.ConfigStorer, r.IndexStorer)
}

// _ is a type assertion
var (
	// ModuleStorer stores information about submodules.
	// Submodules are represented as references to other Repo DAGs.
	_ storage.ModuleStorer = ((*Store)(nil))
)
