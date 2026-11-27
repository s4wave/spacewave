package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// openWorld opens a transaction on the World root at ref, writing through
// store, or the bucket's transaction store when store is nil, and decodes it.
func (a *LocalAuthority) openWorld(ctx context.Context, store block.StoreOps, ref *block.BlockRef) (*block.Transaction, *block.Cursor, *World, error) {
	tx, cursor := a.root.BuildTransactionAtRefWithStore(nil, ref, store)
	w, err := decodeWorld(ctx, cursor, ref)
	return tx, cursor, w, err
}

// readWorld opens the World root at ref in a read-only transaction, reading
// through store as openWorld does, and decodes it. A walk of its graph tree
// follows stored pages through ReadCursor, bounded by the decoded-block cache.
// On a writable transaction each page it loaded would stay on the cursor tree.
func (a *LocalAuthority) readWorld(ctx context.Context, store block.StoreOps, ref *block.BlockRef) (*block.Cursor, *World, error) {
	tx, cursor := a.root.BuildTransactionAtRefWithStore(nil, ref, store)
	tx.SetReadOnly()
	w, err := decodeWorld(ctx, cursor, ref)
	return cursor, w, err
}

// readStoredWorld reads and decodes ref directly from the authority's bucket,
// rather than the cursor's transaction store or a regeneration stage.
func (a *LocalAuthority) readStoredWorld(ctx context.Context, ref *block.BlockRef) (*block.Cursor, *World, error) {
	return a.readWorld(ctx, a.store, ref)
}

// readRegenRoots reads the base and built roots in order from the World's bucket.
func (a *LocalAuthority) readRegenRoots(ctx context.Context, baseName, builtName string) (base, built *block.BlockRef, err error) {
	base, err = a.store.GetRoot(ctx, baseName)
	if err != nil {
		return base, nil, err
	}
	built, err = a.store.GetRoot(ctx, builtName)
	return base, built, err
}

// releaseRegenRoots releases each nonempty regeneration root in one root change.
func (a *LocalAuthority) releaseRegenRoots(ctx context.Context, baseName, builtName string, base, built *block.BlockRef) error {
	// Free each name that holds a root.
	var roots []block.NamedRoot
	for _, root := range []block.NamedRoot{{Name: baseName, Previous: base}, {Name: builtName, Previous: built}} {
		if !root.Previous.GetEmpty() {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		return nil
	}
	return a.store.SetRoots(ctx, nil, roots)
}

// pinRegenBase holds the head as the base a regeneration reads.
func (a *LocalAuthority) pinRegenBase(ctx context.Context, name string, head *block.BlockRef) error {
	return a.store.SetRoots(ctx, nil, []block.NamedRoot{{Name: name, Ref: head}})
}

// pinRegenHead holds the head during a catch-up round and returns its release.
func (a *LocalAuthority) pinRegenHead(ctx context.Context, head *block.BlockRef) (func(), error) {
	return block.PinRoot(ctx, a.store, head)
}

// buildRegenRoot builds the pinned base under a stage and holds the result by
// name before releasing the stage. The build yields to this authority's writers.
func (a *LocalAuthority) buildRegenRoot(ctx context.Context, name string, base *block.BlockRef, cfg RegenConfig) error {
	// Write the build under a stage, held until the root holds it.
	store, release, err := block.OpenStage(ctx, a.store)
	if err != nil {
		return err
	}
	defer release()
	built, err := buildNodeGraphRoot(ctx, a.root, store, base, cfg, a.YieldToWriters)
	if err != nil {
		return err
	}
	return a.store.SetRoots(ctx, nil, []block.NamedRoot{{Name: name, Ref: built}})
}

// stageRegenRoots writes the regeneration's next roots through a stage and
// publishes those roots together before releasing the stage. write must not
// retain the staged store.
func (a *LocalAuthority) stageRegenRoots(ctx context.Context, write func(block.StoreOps) ([]block.NamedRoot, error)) error {
	// Keep the written blocks staged until their roots hold them.
	store, release, err := block.OpenStage(ctx, a.store)
	if err != nil {
		return err
	}
	defer release()
	roots, err := write(store)
	if err != nil {
		return err
	}
	return a.store.SetRoots(ctx, nil, roots)
}
