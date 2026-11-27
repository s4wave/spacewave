//go:build js

package main

import (
	"context"
	"time"

	"github.com/pkg/errors"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	unixfs_sdk "github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/volume"
	volume_browser "github.com/s4wave/spacewave/db/volume/browser"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// runWorldInitUnixFS initializes and reads an empty UnixFS root through the product volume.
func runWorldInitUnixFS(ctx context.Context, c *config) error {
	// Commit an empty UnixFS root in a world on the volume.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return err
	}
	defer vol.Close()
	le := logrus.NewEntry(logrus.New())
	ws, releaseWorld, err := initUnixFSWorld(ctx, le, c, vol, vol)
	if err != nil {
		return err
	}
	defer releaseWorld()

	// Read the root back and check that it is empty.
	handle, _, releaseHandle, err := openUnixFSHandle(ctx, le, ws, vol.GetPeerID())
	if err != nil {
		return err
	}
	defer releaseHandle()
	var entries []string
	if err := handle.ReaddirAll(ctx, 0, func(ent unixfs_sdk.FSCursorDirent) error {
		entries = append(entries, ent.GetName())
		return nil
	}); err != nil {
		return errors.Wrap(err, "read unixfs root")
	}
	if len(entries) != 0 {
		return errors.Errorf("unixfs root entries = %v, want empty", entries)
	}
	return nil
}

// initUnixFSWorld builds a writable world state on the bucket c.root+"/world"
// of bkt and commits an empty UnixFS root named "files" in it. The release
// discards the state and its cursor.
func initUnixFSWorld(
	ctx context.Context,
	le *logrus.Entry,
	c *config,
	vol volume.Volume,
	bkt bucket.BucketOps,
) (*world_block.WorldState, func(), error) {
	// Build the world state on a cursor at the bucket's empty root.
	bucketID := c.root + "/world"
	cursor := bucket_lookup.NewCursor(
		ctx,
		nil,
		le,
		nil,
		bkt,
		nil,
		&bucket.ObjectRef{BucketId: bucketID},
		&bucket.BucketOpArgs{BucketId: bucketID, VolumeId: vol.GetID()},
		nil,
	)
	ws, err := world_block.BuildWorldStateFromCursor(
		ctx,
		le,
		true,
		cursor,
		world.NewWorldStorageFromCursor(cursor),
		lookupSpaceOp,
		false,
	)
	if err != nil {
		cursor.Release()
		return nil, nil, errors.Wrap(err, "build world state")
	}
	release := func() {
		ws.Discard()
		cursor.Release()
	}

	// Commit the UnixFS root.
	if _, _, err := space_world_ops.InitUnixFS(ctx, ws, vol.GetPeerID(), "files", time.Now()); err != nil {
		release()
		return nil, nil, errors.Wrap(err, "init unixfs")
	}
	if err := ws.Commit(ctx); err != nil {
		release()
		return nil, nil, errors.Wrap(err, "commit initial world state")
	}
	return ws, release, nil
}

// openUnixFSHandle opens a handle on the UnixFS root "files" in ws and returns
// the World cursor it reads through. The release frees the handle and its
// cursor.
func openUnixFSHandle(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (*unixfs_sdk.FSHandle, *unixfs_world.FSCursor, func(), error) {
	// Follow the root, then open a handle on the cursor.
	fsCursor, err := unixfs_world.FollowUnixfsRef(
		ctx,
		le,
		ws,
		&unixfs_world.UnixfsRef{
			ObjectKey: "files",
			FsType:    unixfs_world.FSType_FSType_FS_NODE,
		},
		sender,
		true,
	)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "follow unixfs")
	}
	handle, err := unixfs_sdk.NewFSHandle(fsCursor)
	if err != nil {
		fsCursor.Release()
		return nil, nil, nil, errors.Wrap(err, "open fs handle")
	}
	return handle, fsCursor, func() {
		handle.Release()
		fsCursor.Release()
	}, nil
}

// runWorldCrashRecovery checks that every world commit survives a crash: a
// commit stores its blocks and swaps the World root in one transaction, so a
// teardown without Sync loses nothing, and a reopened engine lands on the
// last commit.
func runWorldCrashRecovery(ctx context.Context, c *config) error {
	// Commit obj-a and obj-b, and tear down without a Sync.
	headB, err := writeWorldCommits(ctx, c)
	if err != nil {
		return err
	}

	// Reopen at the World root and check that it is the last commit.
	recovered, err := openWorldEngine(ctx, c)
	if err != nil {
		return err
	}
	defer recovered.release()
	if !objectRefsEqual(recovered.engine.GetRootRef(), headB) {
		return errors.Errorf("OPFS recovery root=%v want last commit=%v", recovered.engine.GetRootRef(), headB)
	}

	// Check that both objects recovered.
	tx, err := recovered.engine.NewTransaction(ctx, false)
	if err != nil {
		return errors.Wrap(err, "open OPFS recovery read transaction")
	}
	defer tx.Discard()
	for _, key := range []string{"opfs-crash-obj-a", "opfs-crash-obj-b"} {
		if _, found, err := tx.GetObject(ctx, key); err != nil {
			return errors.Wrapf(err, "read %s after OPFS recovery", key)
		} else if !found {
			return errors.Errorf("OPFS recovery lost the committed %s", key)
		}
	}
	return nil
}

// writeWorldCommits commits obj-a and obj-b, checking after each that the
// World root holds the commit, then tears the engine down without a Sync. It
// returns the last head.
func writeWorldCommits(ctx context.Context, c *config) (*bucket.ObjectRef, error) {
	// Open the engine.
	writer, err := openWorldEngine(ctx, c)
	if err != nil {
		return nil, err
	}
	defer writer.release()

	// Commit each object. The root must hold the commit when
	// createWorldObject returns.
	for _, key := range []string{"opfs-crash-obj-a", "opfs-crash-obj-b"} {
		if err := createWorldObject(ctx, writer, key); err != nil {
			return nil, err
		}
		head, err := writer.authority.Head(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "read OPFS world head")
		}
		if !head.Root.EqualsRef(writer.engine.GetRootRef().GetRootRef()) {
			return nil, errors.Errorf("OPFS world root does not hold the commit of %s", key)
		}
	}
	return writer.engine.GetRootRef(), nil
}

// createWorldObject commits one named object through the world transaction interface.
func createWorldObject(ctx context.Context, h *worldEngine, key string) error {
	// Create the object in a write transaction and commit it.
	tx, err := h.engine.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrapf(err, "open OPFS writer for %q", key)
	}
	if _, err := tx.CreateObject(ctx, key, nil); err != nil {
		tx.Discard()
		return errors.Wrapf(err, "create OPFS world object %q", key)
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.Wrapf(err, "commit OPFS world object %q", key)
	}
	return nil
}

// worldEngine owns a volume, cursor, authority and world engine.
type worldEngine struct {
	// vol owns the persisted volume.
	vol *volume_browser.Volume
	// cursor pins the world root used to construct the engine.
	cursor *bucket_lookup.Cursor
	// authority holds the World's head as a root of the volume.
	authority *world_block.LocalAuthority
	// engine owns the world transaction lifecycle.
	engine *world_block.Engine
}

// openWorldEngine mounts a world at the head its volume root holds.
func openWorldEngine(ctx context.Context, c *config) (*worldEngine, error) {
	// Open the volume and a cursor on the World's bucket.
	vol, err := openVolume(ctx, c)
	if err != nil {
		return nil, err
	}
	h := &worldEngine{vol: vol}
	bucketID := c.root + "/world-coord-bucket"
	le := logrus.NewEntry(logrus.New())
	h.cursor = bucket_lookup.NewCursor(
		ctx,
		nil,
		le,
		nil,
		vol,
		nil,
		&bucket.ObjectRef{BucketId: bucketID},
		&bucket.BucketOpArgs{BucketId: bucketID, VolumeId: vol.GetID()},
		nil,
	)

	// Read the head the World's root holds.
	h.authority = world_block.NewLocalAuthority(le, h.cursor, world_block.WorldRootName(c.root), nil, coord.Scope{})
	head, err := h.authority.Head(ctx)
	if err != nil {
		h.release()
		return nil, errors.Wrap(err, "read OPFS world head")
	}

	// Open the engine at the head.
	h.cursor.SetRootRef(head.Root)
	engine, err := world_block.NewEngine(ctx, le, h.cursor, lookupSpaceOp, h.authority, false)
	if err != nil {
		h.release()
		return nil, errors.Wrap(err, "open OPFS world engine")
	}
	h.engine = engine
	return h, nil
}

// objectRefsEqual compares optional object references by value.
func objectRefsEqual(a, b *bucket.ObjectRef) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.EqualsRef(b)
}

// release closes the world engine before releasing its cursor and volume.
func (h *worldEngine) release() {
	if h.engine != nil {
		_ = h.engine.Close()
	}
	if h.cursor != nil {
		h.cursor.Release()
	}
	if h.vol != nil {
		_ = h.vol.Close()
	}
}
