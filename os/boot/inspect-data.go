//go:build linux

package boot

import (
	"context"
	"errors"
	"slices"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
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

	// Decode the ordinary generated head; never supply an empty replacement.
	metadata, release, err := store.AccessObjectStore(ctx, dataHeadStore, nil)
	if err != nil {
		return err
	}
	defer release()
	head, err := readDataHead(ctx, metadata)
	if err != nil {
		return err
	}
	if head.GetBucketId() != dataBucket || head.GetRootRef().GetEmpty() {
		return errors.New("invalid data head; store preserved")
	}
	if err := head.Validate(); err != nil {
		return err
	}
	if _, found, err := store.GetBlock(ctx, head.GetRootRef()); err != nil || !found {
		return errors.Join(errors.New("data root missing; store preserved"), err)
	}

	// The head and its retention must describe the same publication domain.
	rootName := world_block.RetainedRootName(dataHeadStore, []byte(dataHeadKey))
	refs, err := volume_kvtx.OpenRefGraph(db).GetOutgoingRefs(ctx, volume_kvtx.RootOwnerIRI(dataBucket, rootName))
	if err != nil || !slices.Equal(refs, []string{block_gc.BlockIRI(head.GetRootRef())}) {
		return errors.Join(errors.New("data head is not durably retained; store preserved"), err)
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

// readDataHead reads the persisted generated HeadState without a recovery path.
func readDataHead(ctx context.Context, store object.ObjectStore) (*bucket.ObjectRef, error) {
	// Retain one read scope until the generated state has been decoded.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, []byte(dataHeadKey))
	if err != nil || !found {
		return nil, errors.Join(errors.New("data head absent; store preserved"), err)
	}
	state := &world_block_engine.HeadState{}
	if err := state.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return state.GetHeadRef(), nil
}
