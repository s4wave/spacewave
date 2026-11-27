package mysql

import (
	"context"

	"github.com/aperturerobotics/util/csync"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// storeRoot is the committed root of a Mysql store and the gate that
// serializes access to it. A write transaction holds the gate for writing from
// open until it commits or discards, so commits advance the root one at a time.
type storeRoot struct {
	// rmtx is the gate: shared by readers, exclusive to a writer.
	rmtx csync.RWMutex
	// cursor points at the committed root.
	cursor RootCursor
	// commitFn confirms each new root before the cursor advances, or is nil.
	commitFn CommitFn
}

// lock takes the gate for reading or writing and returns its release.
func (r *storeRoot) lock(ctx context.Context, write bool) (func(), error) {
	return r.rmtx.Lock(ctx, write)
}

// getRef returns a copy of the committed root reference.
func (r *storeRoot) getRef(ctx context.Context) (*bucket.ObjectRef, error) {
	// Read the root only while this caller holds the gate.
	release, err := r.lock(ctx, false)
	if err != nil {
		return nil, err
	}
	defer release()
	return r.cursor.GetRef().Clone(), nil
}

// setRef switches the cursor to an externally advanced root.
func (r *storeRoot) setRef(ctx context.Context, ref *bucket.ObjectRef) error {
	// Serialize root replacement with transactions without trapping canceled callers.
	release, err := r.lock(ctx, true)
	if err != nil {
		return err
	}
	defer release()
	r.cursor.SetRootRef(ref.GetRootRef().Clone())
	return nil
}

// fetch opens a block transaction at the committed root and decodes the root
// block. A read transaction's block transaction is read-only, so it shares
// decoded blocks with other readers instead of cloning them. The caller holds
// the gate.
func (r *storeRoot) fetch(ctx context.Context, write bool) (
	rn *Root,
	btx *block.Transaction,
	bcs *block.Cursor,
	err error,
) {
	btx, bcs = r.cursor.BuildTransaction(nil)
	if !write {
		btx.SetReadOnly()
	}
	rn, err = block.UnmarshalBlock[Root](ctx, bcs)
	return
}

// commit confirms the written root res through commitFn, then advances the
// cursor to it. An empty root commits an empty reference. When commitFn fails
// the cursor stays at the previous root. The caller holds the gate for
// writing.
func (r *storeRoot) commit(res *block.BlockRef, empty bool) error {
	// Build the next root reference from the committed one, which GetRef clones.
	nextRef := r.cursor.GetRef()
	if !empty {
		nextRef.RootRef = res.Clone()
	} else {
		nextRef.RootRef = nil
	}

	// Confirm it, then advance.
	if commitFn := r.commitFn; commitFn != nil {
		if err := commitFn(nextRef); err != nil {
			return err
		}
	}
	r.cursor.SetRootRef(res.Clone())
	return nil
}
