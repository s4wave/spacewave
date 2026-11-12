package world_block_engine

import (
	"context"
	"encoding/base64"

	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/object"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
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

// LegacyHead is where an older engine kept a World's head before the head
// moved to the World's root: a HeadState record in an object store, and a
// root of the same bucket named by LegacyRootName.
type LegacyHead struct {
	// Store is the object store, or nil when the World had none.
	Store object.ObjectStore
	// Key is the HeadState record's key in Store.
	Key []byte
	// RootName is the name of the old root, from LegacyRootName.
	RootName string
	// Decode decodes a stored record, or is nil for one stored as it is.
	Decode func([]byte) ([]byte, error)
}

// read reads the head an older engine kept: the HeadState record, which that
// engine read as current, and the old root. When both are set and differ, it
// logs both and returns the record's.
func (l LegacyHead) read(ctx context.Context, le *logrus.Entry, cursor *bucket_lookup.Cursor) (*block.BlockRef, *block.BlockRef, error) {
	// Read the HeadState record.
	var recorded *block.BlockRef
	if l.Store != nil {
		state, found, err := l.load(ctx)
		if err != nil {
			return nil, nil, err
		}
		if found {
			recorded = state.GetHeadRef().GetRootRef()
		}
	}

	// Read the old root.
	oldRoot, err := cursor.GetBucket().GetRoot(ctx, l.RootName)
	if err != nil {
		return nil, nil, err
	}

	// Prefer the record, which the older engine read as its head.
	switch {
	case recorded.GetEmpty():
		return oldRoot, oldRoot, nil
	case !oldRoot.GetEmpty() && !oldRoot.EqualsRef(recorded):
		le.
			WithField("head-state", recorded.MarshalString()).
			WithField("retained-root", oldRoot.MarshalString()).
			Warn("world head record and retained root disagree; adopting the head record")
	}
	return recorded, oldRoot, nil
}

// drop releases the old root and deletes the HeadState record once the World
// root holds the head. An open interrupted between the adoption and the
// delete finishes the delete at the next open.
func (l LegacyHead) drop(ctx context.Context, cursor *bucket_lookup.Cursor) error {
	// Release the old root when an earlier open left it.
	store := cursor.GetBucket()
	oldRoot, err := store.GetRoot(ctx, l.RootName)
	if err != nil {
		return err
	}
	if !oldRoot.GetEmpty() {
		if err := store.SetRoots(ctx, nil, []block.NamedRoot{{Name: l.RootName, Previous: oldRoot}}); err != nil {
			return err
		}
	}

	// Delete the HeadState record.
	if l.Store == nil {
		return nil
	}
	return l.delete(ctx)
}

// load reads the HeadState record.
func (l LegacyHead) load(ctx context.Context) (*HeadState, bool, error) {
	// Read the committed record through a read snapshot.
	ktx, err := l.Store.NewTransaction(ctx, false)
	if err != nil {
		return nil, false, err
	}
	defer ktx.Discard()
	data, found, err := ktx.Get(ctx, l.Key)
	if err != nil || !found {
		return nil, false, err
	}

	// Decode the storage transform before parsing the record.
	if l.Decode != nil {
		if data, err = l.Decode(data); err != nil {
			return nil, false, err
		}
	}
	s := &HeadState{}
	if err := s.UnmarshalVT(data); err != nil {
		return nil, true, err
	}
	return s, true, nil
}

// delete deletes the HeadState record, if any.
func (l LegacyHead) delete(ctx context.Context) error {
	// Read the record before writing, so an open with no record writes
	// nothing.
	ktx, err := l.Store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer ktx.Discard()
	_, found, err := ktx.Get(ctx, l.Key)
	if err != nil || !found {
		return err
	}

	// Delete it.
	if err := ktx.Delete(ctx, l.Key); err != nil {
		return err
	}
	return ktx.Commit(ctx)
}

// legacyHead returns where an older engine kept the head of this controller's
// World, whose record is in stateStore.
func (c *Controller) legacyHead(stateStore object.ObjectStore, stateStoreID string) LegacyHead {
	legacy := LegacyHead{Store: stateStore, Key: c.objectStoreHeadKey(), RootName: retainedRootName(stateStoreID, c.objectStoreHeadKeyPrefix())}
	if !c.conf.GetStateTransformConf().GetEmpty() {
		legacy.Decode = c.stateXfrm.DecodeBlock
	}
	return legacy
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
	worldRoot := world_block.NewWorld(c.conf.GetFormat(), c.conf.GetDisableChangelog())
	worldRoot.GraphSettings = c.conf.GetGraphSettings().CloneVT()
	bcs.ClearAllRefs()
	bcs.SetBlock(worldRoot, true)
	root, _, err := btx.Write(ctx, true)
	return root, err
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
