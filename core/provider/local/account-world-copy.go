package provider_local

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// accountWorldRoot is the named root that holds the account World copy.
const accountWorldRoot = "account-world"

// copyWorldGraph copies the World graph under root into store under a stage,
// then moves the account World root onto it. It skips each subtree the stage
// holds, as block.StageCopy describes. visited observes each block the copy
// reads; missing, when set, accepts a block no source serves as a hole, which
// leaves its ancestors unproved and the root unmoved.
func copyWorldGraph(
	ctx context.Context,
	store block.StoreOps,
	root *block.BlockRef,
	visited func(*block.BlockRef, []byte),
	missing func(*block.BlockRef),
) error {
	// Copy under a stage until the root holds the copy.
	c, err := block.OpenStageCopy(ctx, store, false)
	if err != nil {
		return err
	}
	defer c.Release()
	var holes int
	opts := block.GraphCopyOptions{Visited: visited}
	if missing != nil {
		opts.Missing = func(_ context.Context, ref *block.BlockRef) error {
			holes++
			missing(ref)
			return nil
		}
	}
	if err := c.Copy(ctx, store, root, opts); err != nil {
		return err
	}
	if err := c.Flush(ctx); err != nil {
		return err
	}

	// Hold the World under its root unless the copy left a hole.
	if holes != 0 {
		return nil
	}
	return holdAccountWorld(ctx, store, root)
}

// holdAccountWorld moves the account World root onto the copied World. The
// copy routine is the root's only writer, so it reads the root it moves from;
// a move that loses a race returns ErrRootMoved and the next copy tries again.
// A store that holds no roots keeps every block it stores, so it needs no
// hold.
func holdAccountWorld(ctx context.Context, store block.StoreOps, root *block.BlockRef) error {
	// Read what the root holds.
	held, err := store.GetRoot(ctx, accountWorldRoot)
	if err != nil {
		return err
	}

	// Move it onto the copy.
	err = store.SetRoots(ctx, nil, []block.NamedRoot{{Name: accountWorldRoot, Ref: root, Previous: held}})
	if errors.Is(err, block.ErrRootsUnheld) {
		return nil
	}
	return err
}
