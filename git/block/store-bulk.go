package git_block

import (
	"bytes"
	"iter"
	"sort"

	"github.com/aperturerobotics/hydra/block"
	kvtx_block_iavl "github.com/aperturerobotics/hydra/kvtx/block/iavl"
	"github.com/go-git/go-git/v5/plumbing"
)

// bulkEntry is a key-value pair for bulk IAVL tree construction.
type bulkEntry struct {
	key []byte
	ref *block.BlockRef
}

// initBulkMode initializes the bulk write state on the store.
// Called from setBlockTransaction.
func (r *Store) initBulkMode() {
	storeOps, _ := r.bcs.GetBlockStore()
	r.storeOps = storeOps
	r.objIndex = make(map[plumbing.Hash]*block.BlockRef)
	// Capture the transformer and putOpts from the cursor's transaction
	// so mini-transactions use the same encryption config.
	if tx := r.bcs.GetTransaction(); tx != nil {
		r.bulkXfrm = tx.GetTransformer()
		r.bulkPutOpts = tx.GetPutOpts()
	}
}

// lookupBulkObject looks up an object by hash in the bulk index.
// Returns the cursor for reading (in a temporary transaction) or nil if not found.
func (r *Store) lookupBulkObject(h plumbing.Hash) *block.Cursor {
	ref := r.objIndex[h]
	if ref == nil {
		return nil
	}
	// Create a lightweight read-only transaction to follow the persisted object.
	_, cs := block.NewTransaction(r.storeOps, r.bulkXfrm, ref, r.bulkPutOpts)
	return cs
}

// bulkSortedIter returns an iter.Seq2 over sorted bulk entries.
func bulkSortedIter(entries []bulkEntry) iter.Seq2[[]byte, *block.BlockRef] {
	sort.Slice(entries, func(i, j int) bool {
		return bytes.Compare(entries[i].key, entries[j].key) < 0
	})
	return func(yield func([]byte, *block.BlockRef) bool) {
		for _, e := range entries {
			if !yield(e.key, e.ref) {
				return
			}
		}
	}
}

// bulkBuildTree builds an IAVL tree bottom-up from accumulated entries.
// Returns the root Node (with child BlockRefs set) or nil if entries is empty.
func (r *Store) bulkBuildTree(entries []bulkEntry) (*kvtx_block_iavl.Node, error) {
	if len(entries) == 0 {
		return nil, nil
	}

	tx, rootCs, err := kvtx_block_iavl.BuildTree(r.storeOps, r.bulkXfrm, r.bulkPutOpts, bulkSortedIter(entries))
	if err != nil {
		return nil, err
	}

	// Write tree nodes to KV. Keep tree in memory (clearTree=false)
	// so we can extract the root Node with child BlockRefs applied.
	_, _, err = tx.Write(r.ctx, false)
	if err != nil {
		return nil, err
	}

	rootBlk, _ := rootCs.GetBlock()
	rootNode, ok := rootBlk.(*kvtx_block_iavl.Node)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return rootNode, nil
}

// bulkCommit builds IAVL trees from accumulated entries and updates the Repo block.
func (r *Store) bulkCommit() error {
	// Commit sub-stores first (they update their Repo cursors in btx).
	for _, sub := range r.subStores {
		if err := sub.Commit(); err != nil {
			return err
		}
	}

	// Build object IAVL tree from accumulated entries.
	objRoot, err := r.bulkBuildTree(r.objKeys)
	if err != nil {
		return err
	}

	// Update the IAVL root cursor in btx with the new tree root.
	// The objTree cursor chain exists from setBlockTransaction:
	//   Repo -> sub(3) EncodedObjectStore -> sub(1) KeyValueStore -> sub(2) IAVL root
	if objRoot != nil {
		iavlRootCs := r.objTree.GetCursor()
		iavlRootCs.ClearAllRefs()
		iavlRootCs.SetBlock(objRoot, true)
	}

	// Ref and mod trees are unchanged — they used the existing IAVL per-insert
	// path and are already in btx. Nothing to do for them here.

	return nil
}
