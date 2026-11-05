package world_block_engine

import (
	"context"
	"encoding/base64"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// defaultHeadStateKey is the default key an older engine kept its head under.
const defaultHeadStateKey = "world-head"

// retainedRootName returns the name of the root an older engine held its head
// under, in the World's bucket.
func retainedRootName(objectStoreID string, key []byte) string {
	return "world/" + base64.RawURLEncoding.EncodeToString([]byte(objectStoreID)) + "/" + base64.RawURLEncoding.EncodeToString(key)
}

// LegacyRootName returns the name of the root an older engine held a World's
// head under, for a World opened outside this controller that adopts the
// head the same way: objectStoreID and key name the old head record.
func LegacyRootName(objectStoreID, key string) string {
	return retainedRootName(objectStoreID, []byte(key))
}

// legacyHead is where an older engine kept a World's head before the head
// moved to the World's root: a HeadState record in an object store, and a
// root of the same bucket named by retainedRootName.
type legacyHead struct {
	// store is the object store, or nil when the World had none.
	store object.ObjectStore
	// key is the HeadState record's key in store.
	key []byte
	// rootName is the name of the old root, from retainedRootName.
	rootName string
}

// openHead returns the head the authority holds, adopting the head an older
// engine kept when the World root is free, or creating the World when there
// is none. It then drops what the older engine kept, so no World keeps a
// second hold on its tree and no dead record remains.
//
// The adoption is not a migration: a World runs it the first time it opens on
// this engine. A World never opened on it keeps its old retained root, a held
// root of its bucket that holds its tree, so the collector keeps the tree with
// frees on. The one gap is a record an older engine wrote after the retained
// root it then failed to move, which no root holds until the open adopts it.
func (c *Controller) openHead(
	ctx context.Context,
	authority *world_block.LocalAuthority,
	cursor *bucket_lookup.Cursor,
	legacy legacyHead,
) (world.Head, error) {
	// Read the head the authority holds.
	head, err := authority.Head(ctx)
	if err != nil || !head.Root.GetEmpty() {
		if err == nil {
			err = c.dropLegacyHead(ctx, cursor, legacy, head.Root)
		}
		return head, err
	}

	// Adopt the head the older engine kept, or create the World. The
	// adoption moves the root and releases the old one in one transaction.
	old, oldRoot, err := c.readLegacyHead(ctx, cursor, legacy)
	if err != nil {
		return world.Head{}, err
	}
	var release []block.NamedRoot
	if !oldRoot.GetEmpty() {
		release = append(release, block.NamedRoot{Name: legacy.rootName, Previous: oldRoot})
	}

	// Replace an old head whose blocks are missing when configured to.
	root := old
	if !root.GetEmpty() {
		found, err := cursor.GetBucket().GetBlockExists(ctx, root)
		switch {
		case err != nil:
			return world.Head{}, err
		case !found && !c.conf.GetRecoverMissingPersistedHead():
			return world.Head{}, errors.Wrap(block.ErrNotFound, root.MarshalString())
		case !found:
			c.le.WithField("world-root", root.MarshalString()).Warn("world head is missing blocks; rebuilding world")
			root = nil
		default:
			c.le.WithField("world-root", root.MarshalString()).Info("adopting the world head into the world root")
		}
	}

	// Install the old head, or a new World.
	if root.GetEmpty() {
		if root, err = c.initialRoot(ctx, cursor); err != nil {
			return world.Head{}, err
		}
	}
	head, err = authority.Install(ctx, nil, root, release...)
	if err != nil {
		return world.Head{}, err
	}

	// Delete the old record.
	return head, c.dropLegacyHead(ctx, cursor, legacy, head.Root)
}

// readLegacyHead reads the head an older engine kept: the HeadState record,
// which that engine read as current, and the old root. When both are set and
// differ, it logs both and returns the record's.
func (c *Controller) readLegacyHead(ctx context.Context, cursor *bucket_lookup.Cursor, legacy legacyHead) (*block.BlockRef, *block.BlockRef, error) {
	// Read the HeadState record.
	var recorded *block.BlockRef
	if legacy.store != nil {
		state, found, err := c.loadHeadState(ctx, legacy.store, legacy.key)
		if err != nil {
			return nil, nil, err
		}
		if found {
			recorded = state.GetHeadRef().GetRootRef()
		}
	}

	// Read the old root.
	oldRoot, err := cursor.GetBucket().GetRoot(ctx, legacy.rootName)
	if err != nil {
		return nil, nil, err
	}

	// Prefer the record, which the older engine read as its head.
	switch {
	case recorded.GetEmpty():
		return oldRoot, oldRoot, nil
	case !oldRoot.GetEmpty() && !oldRoot.EqualsRef(recorded):
		c.le.
			WithField("head-state", recorded.MarshalString()).
			WithField("retained-root", oldRoot.MarshalString()).
			Warn("world head record and retained root disagree; adopting the head record")
	}
	return recorded, oldRoot, nil
}

// dropLegacyHead releases the old root and deletes the HeadState record once
// the World root holds the head. An open interrupted between the adoption and
// the delete finishes the delete at the next open.
func (c *Controller) dropLegacyHead(ctx context.Context, cursor *bucket_lookup.Cursor, legacy legacyHead, head *block.BlockRef) error {
	// Release the old root when an earlier open left it.
	store := cursor.GetBucket()
	oldRoot, err := store.GetRoot(ctx, legacy.rootName)
	if err != nil {
		return err
	}
	if !oldRoot.GetEmpty() {
		if err := store.SetRoots(ctx, nil, []block.NamedRoot{{Name: legacy.rootName, Previous: oldRoot}}); err != nil {
			return err
		}
	}

	// Delete the HeadState record.
	if legacy.store == nil {
		return nil
	}
	return c.deleteHeadState(ctx, legacy.store, legacy.key)
}

// initialRoot returns the configured initial root, or writes a new empty
// World with the configured settings and returns its root.
func (c *Controller) initialRoot(ctx context.Context, cursor *bucket_lookup.Cursor) (*block.BlockRef, error) {
	// Use the configured initial root.
	if root := c.conf.GetInitHeadRef().GetRootRef(); !root.GetEmpty() {
		return root, nil
	}

	// Open an empty cursor on the World's bucket.
	c.le.Debug("no initial head reference provided, building new world")
	init := cursor.Clone()
	defer init.Release()
	init.SetRootRef(nil)

	// Write a new empty World. The authority's Install holds it.
	btx, bcs := init.BuildTransaction(nil)
	worldRoot := world_block.NewWorld(c.conf.GetDisableChangelog())
	worldRoot.GraphSettings = c.conf.GetGraphSettings().CloneVT()
	bcs.ClearAllRefs()
	bcs.SetBlock(worldRoot, true)
	root, _, err := btx.Write(ctx, true)
	return root, err
}

// loadHeadState reads the HeadState record at key.
func (c *Controller) loadHeadState(ctx context.Context, store object.ObjectStore, key []byte) (*HeadState, bool, error) {
	// Read the committed record through a read snapshot.
	ktx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, false, err
	}
	defer ktx.Discard()
	data, found, err := ktx.Get(ctx, key)
	if err != nil || !found {
		return nil, false, err
	}

	// Decode the configured storage transform before parsing the record.
	if !c.conf.GetStateTransformConf().GetEmpty() {
		if data, err = c.stateXfrm.DecodeBlock(data); err != nil {
			return nil, false, err
		}
	}
	s := &HeadState{}
	if err := s.UnmarshalVT(data); err != nil {
		return nil, true, err
	}
	return s, true, nil
}

// deleteHeadState deletes the HeadState record at key, if any.
func (c *Controller) deleteHeadState(ctx context.Context, store object.ObjectStore, key []byte) error {
	// Read the record before writing, so an open with no record writes
	// nothing.
	ktx, err := store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer ktx.Discard()
	_, found, err := ktx.Get(ctx, key)
	if err != nil || !found {
		return err
	}

	// Delete it.
	if err := ktx.Delete(ctx, key); err != nil {
		return err
	}
	return ktx.Commit(ctx)
}

// objectStoreHeadKey returns the HeadState record's key in the object store.
func (c *Controller) objectStoreHeadKey() []byte {
	if key := c.conf.GetObjectStoreHeadKey(); key != "" {
		return []byte(key)
	}
	return []byte(defaultHeadStateKey)
}

// objectStoreHeadKeyPrefix returns the record's key in the backing store,
// with the object store prefix, which the old root's name was made from.
func (c *Controller) objectStoreHeadKeyPrefix() []byte {
	prefix := []byte(c.conf.GetObjectStorePrefix())
	return append(prefix, c.objectStoreHeadKey()...)
}
