package git_block

import (
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
	hydra_git "github.com/aperturerobotics/hydra/git"
	"github.com/aperturerobotics/hydra/kvtx"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/storage"
)

// Store contains a open handle to a git repository.
type Store struct {
	config.ConfigStorer
	storer.IndexStorer

	ctx       context.Context
	ctxCancel context.CancelFunc
	btx       *block.Transaction
	bcs       *block.Cursor
	root      *Repo

	refTree kvtx.BlockTx
	modTree kvtx.BlockTx
	objTree kvtx.BlockTx
}

// NewStore constructs a new repo handle.
// btx can be nil to indicate a read-only tree.
// bcs is located at the root of the repo (the Repo block).
func NewStore(
	ctx context.Context,
	btx *block.Transaction,
	bcs *block.Cursor,
	configStore config.ConfigStorer,
	indexStore storer.IndexStorer,
) (*Store, error) {
	rdr := &Store{
		ConfigStorer: configStore,
		IndexStorer:  indexStore,
	}
	rdr.btx, rdr.bcs = btx, bcs
	rdr.ctx, rdr.ctxCancel = context.WithCancel(ctx)
	if err := rdr.setBlockTransaction(btx, bcs); err != nil {
		return nil, err
	}
	return rdr, nil
}

// GetReadOnly returns if the state is read-only.
func (r *Store) GetReadOnly() bool {
	return r.btx == nil
}

// GetRoot returns the root object.
// Note: this should be treated as read-only.
func (r *Store) GetRoot() *Repo {
	return r.root
}

// GetCursor returns the underlying root cursor.
func (r *Store) GetCursor() *block.Cursor {
	return r.bcs
}

// GetRef returns the root reference.
func (r *Store) GetRef() *block.BlockRef {
	return r.bcs.GetRef()
}

// Commit commits the current pending changes to the block transaction.
func (r *Store) Commit() error {
	_, bcs, err := r.btx.Write(true)
	if err != nil {
		return err
	}
	return r.setBlockTransaction(r.btx, bcs)
}

// Close closes the store, canceling the context.
func (r *Store) Close() error {
	r.ctxCancel()
	return nil
}

// buildEncodedObjectTree builds the encoded object tree handle.
func (r *Store) buildEncodedObjectTree() (kvtx.BlockTx, *block.Cursor, error) {
	encStore, storeCs, err := r.root.FollowEncodedObjectStore(r.bcs)
	if err != nil {
		return nil, nil, err
	}
	v, err := encStore.BuildObjectTree(storeCs)
	if err != nil {
		return nil, nil, err
	}
	return v, storeCs, nil
}

// buildRefTree builds the reference tree handle.
func (r *Store) buildRefTree() (kvtx.BlockTx, *block.Cursor, error) {
	encStore, storeCs, err := r.root.FollowReferencesStore(r.bcs)
	if err != nil {
		return nil, nil, err
	}
	v, err := encStore.BuildRefTree(storeCs)
	if err != nil {
		return nil, nil, err
	}
	return v, storeCs, nil
}

// buildModRefTree builds the sub-module references tree
func (r *Store) buildModRefTree() (kvtx.BlockTx, *block.Cursor, error) {
	encStore, storeCs, err := r.root.FollowModuleReferencesStore(r.bcs)
	if err != nil {
		return nil, nil, err
	}
	v, err := encStore.BuildModRefTree(storeCs)
	if err != nil {
		return nil, nil, err
	}
	return v, storeCs, nil
}

// setBlockTransaction sets the root block transaction and cursor.
func (r *Store) setBlockTransaction(btx *block.Transaction, bcs *block.Cursor) error {
	root, err := bcs.Unmarshal(NewRepoBlock)
	if err != nil {
		return err
	}
	if bcs.GetRef().GetEmpty() && root == nil {
		// initialize new repo
		root = NewRepo()
		bcs.SetBlock(root, true)
	}
	rootVal, ok := root.(*Repo)
	if !ok {
		return block.ErrUnexpectedType
	}
	r.root = rootVal
	r.objTree, _, err = r.buildEncodedObjectTree()
	if err != nil {
		return err
	}
	r.refTree, _, err = r.buildRefTree()
	if err != nil {
		return err
	}
	r.modTree, _, err = r.buildModRefTree()
	if err != nil {
		return err
	}
	r.btx, r.bcs = btx, bcs
	return nil
}

// _ is a type assertion
var (
	_ io.Closer = ((*Store)(nil))
	// this covers all storage interfaces
	_ storage.Storer = ((*Store)(nil))
	// covers Hydra git storage interface
	_ hydra_git.Storer = ((*Store)(nil))
)
