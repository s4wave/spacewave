package world_block_engine

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// OpenHeadOptions adjusts OpenHead.
type OpenHeadOptions struct {
	// Initial returns the root of a new World, for a World with no head.
	Initial func(context.Context) (*block.BlockRef, error)
	// RecoverMissing replaces an adopted head whose root block is missing
	// with a new World instead of failing.
	RecoverMissing bool
}

// OpenHead returns the head the authority holds, adopting the head an older
// engine kept when the World root is free, or creating the World when there
// is none. It then drops what the older engine kept, so no World keeps a
// second hold on its tree and no dead record remains.
//
// The adoption is not a migration: a World runs it the first time it opens on
// this engine. A World never opened on it keeps its old retained root, a held
// root of its bucket that holds its tree, so the collector keeps the tree with
// frees on. The one gap is a record an older engine wrote after the retained
// root it then failed to move, which no root holds until the open adopts it.
func OpenHead(
	ctx context.Context,
	le *logrus.Entry,
	authority *world_block.LocalAuthority,
	cursor *bucket_lookup.Cursor,
	legacy LegacyHead,
	opts OpenHeadOptions,
) (world.Head, error) {
	// Read the head the authority holds.
	head, err := authority.Head(ctx)
	if err != nil || !head.Root.GetEmpty() {
		if err == nil {
			err = legacy.drop(ctx, cursor)
		}
		return head, err
	}

	// Adopt the head the older engine kept, or create the World. The
	// adoption moves the root and releases the old one in one transaction.
	old, oldRoot, err := legacy.read(ctx, le, cursor)
	if err != nil {
		return world.Head{}, err
	}
	var release []block.NamedRoot
	if !oldRoot.GetEmpty() {
		release = append(release, block.NamedRoot{Name: legacy.RootName, Previous: oldRoot})
	}

	// Replace an old head whose blocks are missing when configured to.
	root := old
	if !root.GetEmpty() {
		found, err := cursor.GetBucket().GetBlockExists(ctx, root)
		switch {
		case err != nil:
			return world.Head{}, err
		case !found && !opts.RecoverMissing:
			return world.Head{}, errors.Wrap(block.ErrNotFound, root.MarshalString())
		case !found:
			le.WithField("world-root", root.MarshalString()).Warn("world head is missing blocks; rebuilding world")
			root = nil
		default:
			le.WithField("world-root", root.MarshalString()).Info("adopting the world head into the world root")
		}
	}

	// Install the old head, or a new World.
	if root.GetEmpty() {
		if root, err = opts.Initial(ctx); err != nil {
			return world.Head{}, err
		}
	}
	head, err = authority.Install(ctx, nil, root, release...)
	if err != nil {
		return world.Head{}, err
	}

	// Delete the old record.
	return head, legacy.drop(ctx, cursor)
}
