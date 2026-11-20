package provider_local

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
)

// accountWorldRoot is the named root that holds the account World copy.
const accountWorldRoot = "account-world"

// copyWorldGraph copies the World graph under root from src into store under a
// stage, then moves the account World root onto it. It skips each subtree the stage
// holds, as block.StageCopy describes. visited observes each block the copy
// reads; missing, when set, accepts a block no source serves as a hole, which
// leaves its ancestors unproved and the root unmoved.
//
// Unless mode is full, the copy leaves the chunks of linked files where they
// are: it counts as present a block a linked file on this Device serves, and
// one that a Device of this account answers linked, and writes neither.
//
// Invariant: the copy skips a block only when a linked file is its only copy
// on a Device that still holds it. A Volume copy is never skipped. Both sides
// read every Volume tier before the files: this Device's copy chain reads the
// files after its Volume tiers (linkedThen), and a peer answers linked only
// when its serve reads found no copy in its local Volume store and read the
// block from a file whose bytes it verified against the ref (the DEX
// responder). The copy asks for linked answers only from the unrevoked
// sessions of this account (readLinkedCopy), and a linked answer from any
// other peer fails that peer's request, so the copy writes another peer's
// data or leaves a hole, never a skip a member could cause.
func copyWorldGraph(
	ctx context.Context,
	store, src block.StoreOps,
	root *block.BlockRef,
	visited func(*block.BlockRef, []byte),
	missing func(*block.BlockRef),
	mode linkedCopy,
) error {
	// Copy under a stage until the root holds the copy.
	c, err := block.OpenStageCopy(ctx, store, mode.recheck)
	if err != nil {
		return err
	}
	defer c.Release()

	// Leave linked chunks unwritten unless the copy is full, taking linked
	// answers only from the peers mode admits.
	opts := block.GraphCopyOptions{Visited: visited, CountLinked: !mode.full}
	if mode.peers != nil {
		ctx = dex_solicit.WithLinkedAnswers(ctx, mode.peers)
	}

	// Accept a block no source serves as a hole.
	var holes int
	if missing != nil {
		opts.Missing = func(_ context.Context, ref *block.BlockRef) error {
			holes++
			missing(ref)
			return nil
		}
	}
	if err := c.Copy(ctx, src, root, opts); err != nil {
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
// A store that holds no roots frees nothing by liveness, so the copy needs
// no hold there.
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
