//go:build linux

package boot

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// inspectData validates the boot records through a read-only handle. Inspection
// costs fixed indexed lookups plus the World/UnixFS root paths, not a data walk.
// Descendant content is checked on access, without scanning every stored file.
func inspectData(ctx context.Context, le *logrus.Entry, path string) (retErr error) {
	// ReadOnly refuses an absent file and cannot initialize or recover its bytes.
	db, err := s4db.Open(path, s4db.Options{ReadOnly: true})
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, db.Close()) }()
	keys := store_kvkey.NewDefaultKVKey()
	store := store_kvtx.NewKVTx(keys, db, nil)

	// Reject any key format the writable Volume might otherwise migrate.
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	data, found, err := tx.Get(ctx, keys.GetFormatVersionKey())
	tx.Discard()
	if err != nil {
		return err
	}
	version, err := store_kvkey.ParseFormatVersion(data)
	if err != nil || !found || version != store_kvkey.FormatVersion {
		return errors.Join(errors.New("unsupported data Volume format; store preserved"), err)
	}

	// Require the existing identity and bucket before permitting writable open.
	key, err := store.LoadPeerPriv(ctx)
	if err != nil || key == nil {
		return errors.Join(errors.New("data Volume identity missing; store preserved"), err)
	}
	conf, err := store.GetBucketConfig(ctx, dataBucket)
	if err != nil || conf.GetId() != dataBucket {
		return errors.Join(errors.New("data bucket missing; store preserved"), err)
	}

	// The ordinary World authority keeps its head in a held root, not a second
	// metadata record. Read that root without adopting or repairing anything.
	ref, err := readBootRoot(ctx, db, dataBucket, world_block.WorldRootName("os-data"))
	if err != nil {
		return err
	}
	head := &bucket.ObjectRef{BucketId: dataBucket, RootRef: ref}
	if _, found, err := store.GetBlock(ctx, ref); err != nil || !found {
		return errors.Join(errors.New("data root missing; store preserved"), err)
	}

	// Validate the World root using an immutable production transaction.
	cursor := bucket_lookup.NewCursor(ctx, nil, le, nil, store, nil, head,
		&bucket.BucketOpArgs{BucketId: dataBucket}, nil)
	defer cursor.Release()
	state, err := world_block.BuildWorldStateFromCursor(ctx, le, false, cursor,
		world.NewWorldStorageFromCursor(cursor), world.NewSchema(unixfs_world.UnixfsTypes).LookupOp, false)
	if err != nil {
		return err
	}
	snapshot := world_block.NewTx(state)
	defer snapshot.Discard()

	// Require a readable UnixFS directory before any writable open can run.
	fsc := unixfs_world.NewFSCursorWithContext(ctx, le, snapshot, dataRootKey,
		unixfs_world.FSType_FSType_FS_NODE, nil, false)
	handle, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		fsc.Release()
		return err
	}
	defer handle.Release()
	info, err := handle.GetFileInfo(ctx)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("data filesystem root is not a directory; store preserved")
	}
	return nil
}
