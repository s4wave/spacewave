package git_block

import (
	"container/list"
	"context"
	"io"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/cache"
	"github.com/go-git/go-git/v6/plumbing/storer"
	"github.com/go-git/go-git/v6/storage"
	"github.com/s4wave/spacewave/db/block"
	hydra_git "github.com/s4wave/spacewave/db/git"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Store contains a open handle to a git repository.
type Store struct {
	storer.IndexStorer

	ctx       context.Context
	ctxCancel context.CancelFunc
	btx       *block.Transaction
	bcs       *block.Cursor
	root      *Repo
	refStore  ReferenceStore

	refTree  kvtx.BlockTx
	modTree  kvtx.BlockTx
	objTree  kvtx.BlockTx
	packTree kvtx.BlockTx
	// packCache retains decoded indexes for the Store lifetime.
	packCache map[plumbing.Hash]*storePackCacheEntry
	// packLRU orders open data readers from newest to oldest.
	packLRU *list.List
	// objectCache is shared by all pack readers in this Store.
	objectCache *cache.ObjectLRU

	// Bulk mode state: objects are written to KV via per-object
	// mini-transactions, then IAVL trees are built bottom-up at Commit.
	// Active when storeOps is non-nil (i.e., write mode).

	// storeOps is the block store for creating mini-transactions.
	storeOps block.StoreOps
	// bulkXfrm is the block transformer for mini-transactions.
	bulkXfrm block.Transformer
	// bulkPutOpts is the put options for mini-transactions.
	bulkPutOpts *block.PutOpts
	// objIndex maps git hash -> persisted BlockRef for bulk-written objects.
	objIndex map[plumbing.Hash]*block.BlockRef
	// objKeys accumulates (iavl_key, BlockRef) for the object IAVL tree.
	objKeys []bulkEntry
	// subStores tracks sub-stores created via Module() for ordered commit.
	subStores []*Store
}

// NewStore constructs a new repo handle.
// btx can be nil to indicate a read-only tree.
// bcs is located at the root of the repo (the Repo block).
// refStore can be nil
func NewStore(
	ctx context.Context,
	btx *block.Transaction,
	bcs *block.Cursor,
	indexStore storer.IndexStorer,
	refStore ReferenceStore,
) (*Store, error) {
	// Initialize the Store context and its repository transaction.
	rdr := &Store{IndexStorer: indexStore, refStore: refStore}
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
// When btx is nil (cursor owned by external transaction), Commit still
// builds IAVL trees from bulk-written objects on the cursor. The caller
// is responsible for writing the external transaction.
func (r *Store) Commit() error {
	// Build IAVL trees from bulk-written objects and wire into Repo.
	if r.storeOps != nil {
		if err := r.bulkCommit(); err != nil {
			return err
		}
	}

	// Leave external transactions for their caller to persist.
	if r.btx == nil {
		return nil
	}

	// Persist the Store transaction and reopen its repository state.
	_, bcs, err := r.btx.Write(r.ctx, true)
	if err != nil {
		return err
	}
	return r.setBlockTransaction(r.btx, bcs)
}

// Close closes the store, canceling the context.
func (r *Store) Close() error {
	// Close cached packfile readers and cancel the Store context.
	for _, entry := range r.packCache {
		if entry.pack != nil {
			_ = entry.pack.Close()
		}
	}
	r.packCache = nil
	r.ctxCancel()
	return nil
}

// buildEncodedObjectTree builds the encoded object tree handle.
func (r *Store) buildEncodedObjectTree() (kvtx.BlockTx, *block.Cursor, error) {
	// Open the repository object store and its loose object tree.
	encStore, storeCs, err := r.root.FollowEncodedObjectStore(r.ctx, r.bcs)
	if err != nil {
		return nil, nil, err
	}
	v, err := encStore.BuildObjectTree(r.ctx, storeCs)
	if err != nil {
		return nil, nil, err
	}
	return v, storeCs, nil
}

// buildPackfileTree builds the packfile metadata tree handle.
func (r *Store) buildPackfileTree() (kvtx.BlockTx, *block.Cursor, error) {
	// Open the repository object store and its packfile metadata tree.
	encStore, storeCs, err := r.root.FollowEncodedObjectStore(r.ctx, r.bcs)
	if err != nil {
		return nil, nil, err
	}
	v, err := encStore.BuildPackfileTree(r.ctx, storeCs)
	if err != nil {
		return nil, nil, err
	}
	return v, storeCs, nil
}

// buildRefTree builds the reference tree handle.
func (r *Store) buildRefTree() (kvtx.BlockTx, *block.Cursor, error) {
	// Open the repository reference store and its reference tree.
	encStore, storeCs, err := r.root.FollowReferencesStore(r.ctx, r.bcs)
	if err != nil {
		return nil, nil, err
	}
	v, err := encStore.BuildRefTree(r.ctx, storeCs)
	if err != nil {
		return nil, nil, err
	}
	return v, storeCs, nil
}

// buildModRefTree builds the sub-module references tree
func (r *Store) buildModRefTree() (kvtx.BlockTx, *block.Cursor, error) {
	// Open the repository submodule store and its reference tree.
	encStore, storeCs, err := r.root.FollowModuleReferencesStore(r.ctx, r.bcs)
	if err != nil {
		return nil, nil, err
	}
	v, err := encStore.BuildModRefTree(r.ctx, storeCs)
	if err != nil {
		return nil, nil, err
	}
	return v, storeCs, nil
}

// setBlockTransaction sets the root block transaction and cursor.
func (r *Store) setBlockTransaction(btx *block.Transaction, bcs *block.Cursor) error {
	// Close packfile readers from the previous Store transaction.
	for _, entry := range r.packCache {
		if entry.pack != nil {
			if err := entry.pack.Close(); err != nil {
				return err
			}
		}
	}

	// Decode the repository root for the new Store transaction.
	root, err := UnmarshalRepo(r.ctx, bcs)
	if err != nil {
		return err
	}
	r.root = root

	// Open the loose object tree for the repository root.
	r.objTree, _, err = r.buildEncodedObjectTree()
	if err != nil {
		return err
	}

	// Open the packfile metadata tree for the repository root.
	r.packTree, _, err = r.buildPackfileTree()
	if err != nil {
		return err
	}

	// Open the reference tree for the repository root.
	r.refTree, _, err = r.buildRefTree()
	if err != nil {
		return err
	}

	// Open the submodule reference tree for the repository root.
	r.modTree, _, err = r.buildModRefTree()
	if err != nil {
		return err
	}

	// Initialize packfile reader tracking for the new transaction.
	r.btx, r.bcs = btx, bcs
	r.packCache = make(map[plumbing.Hash]*storePackCacheEntry)
	r.packLRU = list.New()

	// Match go-git's 96 MiB default once per Store, rather than once per pack.
	r.objectCache = cache.NewObjectLRU(cache.DefaultMaxSize)

	// Initialize bulk object writing for the new Store transaction.
	r.initBulkMode()
	return nil
}

// _ is a type assertion
var (
	_ io.Closer = (*Store)(nil)
	// this covers all storage interfaces
	_ storage.Storer = (*Store)(nil)
	// covers Hydra git storage interface
	_ hydra_git.Storer = (*Store)(nil)
)
