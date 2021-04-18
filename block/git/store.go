package git

import (
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/iavl"
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

	refTree *iavl.Tx
	modTree *iavl.Tx
	objTree *iavl.Tx
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
func (r *Store) buildEncodedObjectTree() (*iavl.Tx, *block.Cursor, error) {
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
func (r *Store) buildRefTree() (*iavl.Tx, *block.Cursor, error) {
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
func (r *Store) buildModRefTree() (*iavl.Tx, *block.Cursor, error) {
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
)
