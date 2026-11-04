//go:build linux

package boot

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/volume"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

const (
	// imageBucket scopes this disposable image's blocks and durable roots.
	imageBucket = "os-image"
	// imageHeadStore scopes the ordinary generated World HeadState.
	imageHeadStore = "os-image"
	// imageHeadKey matches the engine controller's default head key.
	imageHeadKey = "world-head"
	// selectedImageRoot keeps the selected image even if the World later advances.
	selectedImageRoot = "selected-image"
)

// SeedRoot creates an absent disposable directory and publishes its complete
// image once from an independent probe executable and static BusyBox input.
// Failure leaves the partial store for inspection, never a reset. The caller
// supplies a BusyBox build implementing the fixed applets in tools.go; seeding
// verifies stored bytes and layout, not Linux execution or applet compatibility.
//
// Invariants: Sync fences referenced blocks before the head CAS; head and durable
// retention commit together. Opening never seeds or migrates an unknown store.
// The backing store and serving runtime stay bootstrap-side. Workloads exit and
// the FUSE server joins before readers and physical storage close.
func SeedRoot(ctx context.Context, le *logrus.Entry, dir string, executable, busybox io.Reader) (retErr error) {
	// Retained images always include explicit tools, never a partial probe image.
	if executable == nil || busybox == nil {
		return errors.New("seed requires independent executable and static BusyBox inputs")
	}

	// Exclusive directory creation prevents replacing anyone's existing store.
	if err := os.Mkdir(dir, 0o700); err != nil {
		return err
	}
	r, err := openSeedStorage(ctx, le, dir)
	if err != nil {
		return errors.Join(errors.New("seed storage"), err)
	}
	defer func() { retErr = errors.Join(retErr, r.Close()) }()

	// Ordinary World commits durably prepare blocks, but do not select an image.
	if err := r.buildImage(ctx, le, executable, busybox); err != nil {
		return errors.Join(errors.New("seed content"), err)
	}
	if _, err := r.engine.Sync(ctx); err != nil {
		return errors.Join(errors.New("seed fence"), err)
	}

	// Publish the completed image after the engine fence, retaining both names.
	if err := publishImageHead(ctx, r.publisher, r.engine.GetRootRef()); err != nil {
		return errors.Join(errors.New("select image"), err)
	}

	// Report only a fenced head, protected by a durable name and a read snapshot.
	if err := r.pin(ctx, le); err != nil {
		return errors.Join(errors.New("seed snapshot"), err)
	}
	println("SPACEWAVE_OS_SEEDED volume=" + r.volume.GetID() + " root=" + r.engine.GetRootRef().MarshalB58())
	return nil
}

// OpenRoot opens only a committed selected image. It never creates a fixture.
func OpenRoot(ctx context.Context, le *logrus.Entry, dir string) (_ *Root, retErr error) {
	// Own one read-only handle through inspection, serving and ordered cleanup.
	// This isolated image has no writer or collector. Its durable selected root
	// protects descendants without active reader-pin writes or a controller.
	db, err := s4db.Open(filepath.Join(dir, "image.s4wave"), s4db.Options{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	r := &Root{releaseStorage: db.Close, tools: true}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, r.Close())
		}
	}()

	// Inspect the selected head and retention without recovery or migration.
	head, err := inspectImage(ctx, db)
	if err != nil {
		return nil, err
	}

	// Derive the real Volume ID with the existing stored identity encoding.
	store := store_kvtx.NewKVTx(store_kvkey.NewDefaultKVKey(), db, nil)
	key, err := store.LoadPeerPriv(ctx)
	if err != nil || key == nil {
		return nil, errors.Join(errors.New("selected Volume identity missing; store preserved"), err)
	}
	id, err := peer.IDFromPublicKey(key.GetPublic())
	if err != nil {
		return nil, err
	}
	r.volumeID = volume.NewVolumeID(volume_s4db.ControllerID, id)
	r.head = head

	// A bare read-only store has no RootRetainer. Build the ordinary immutable
	// World transaction directly, without a writable engine or background actor.
	cursor := bucket_lookup.NewCursor(ctx, nil, le, nil, store, nil, head,
		&bucket.BucketOpArgs{BucketId: imageBucket, VolumeId: r.volumeID}, nil)
	state, err := world_block.BuildWorldStateFromCursor(ctx, le, false, cursor,
		world.NewWorldStorageFromCursor(cursor), world.NewSchema(unixfs_world.UnixfsTypes).LookupOp, false)
	if err != nil {
		cursor.Release()
		return nil, err
	}
	r.snapshot = world_block.NewTx(state)
	r.releaseStorage = func() error { cursor.Release(); return db.Close() }
	if err := r.openHandle(ctx, le); err != nil {
		return nil, err
	}
	println("SPACEWAVE_OS_REOPEN volume=" + r.volumeID + " root=" + head.MarshalB58())
	return r, nil
}

// inspectImage rejects unsupported, unselected and missing-root stores without
// modifying a byte. Only this experiment's current format is eligible for open.
func inspectImage(ctx context.Context, db *s4db.DB) (*bucket.ObjectRef, error) {
	// Reject key layouts that NewVolume would otherwise migrate.
	keys := store_kvkey.NewDefaultKVKey()
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	data, found, err := tx.Get(ctx, keys.GetFormatVersionKey())
	tx.Discard()
	if err != nil {
		return nil, err
	}
	version, err := store_kvkey.ParseFormatVersion(data)
	if err != nil || !found || version != store_kvkey.FormatVersion {
		return nil, errors.Join(errors.New("unsupported experiment Volume format; store preserved"), err)
	}

	// Read the existing generated head through the ordinary ObjectStore prefix.
	store := store_kvtx.NewKVTx(keys, db, nil)
	metadata, release, err := store.AccessObjectStore(ctx, imageHeadStore, nil)
	if err != nil {
		return nil, err
	}
	defer release()

	// Require a selected, valid root whose own block is already stored.
	head, err := readImageHead(ctx, metadata)
	if err != nil {
		return nil, err
	}
	if head.GetBucketId() != imageBucket || head.GetRootRef().GetEmpty() {
		return nil, errors.New("invalid selected image head; store preserved")
	}
	if err := head.Validate(); err != nil {
		return nil, err
	}
	if _, found, err := store.GetBlock(ctx, head.GetRootRef()); err != nil || !found {
		return nil, errors.Join(errors.New("selected root missing; store preserved"), err)
	}

	// A successful head write alone cannot prove GC retention was enabled.
	graph := volume_kvtx.OpenRefGraph(db)
	for _, name := range []string{world_block.RetainedRootName(imageHeadStore, []byte(imageHeadKey)), selectedImageRoot} {
		refs, err := graph.GetOutgoingRefs(ctx, volume_kvtx.RootOwnerIRI(imageBucket, name))
		if err != nil || !slices.Equal(refs, []string{block_gc.BlockIRI(head.GetRootRef())}) {
			return nil, errors.Join(errors.New("selected root is not durably retained; store preserved"), err)
		}
	}
	return head, nil
}

// readImageHead decodes the engine's existing durable head record.
func readImageHead(ctx context.Context, store object.ObjectStore) (*bucket.ObjectRef, error) {
	// Retain one read scope while decoding the committed head.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, []byte(imageHeadKey))
	if err != nil || !found {
		return nil, errors.Join(errors.New("selected image head absent; store preserved"), err)
	}
	head := &world_block_engine.HeadState{}
	if err := head.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return head.GetHeadRef(), nil
}

// openSeedStorage composes the writable Volume, tracked bucket and construction
// World only after SeedRoot acquired an absent disposable directory.
func openSeedStorage(ctx context.Context, le *logrus.Entry, dir string) (_ *Root, retErr error) {
	// Close partial acquisitions without deleting the retained database.
	r := &Root{}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, r.Close())
		}
	}()
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{
		Path: filepath.Join(dir, "image.s4wave"), SingleProcess: true,
	})
	if err != nil {
		return nil, err
	}
	r.volume = vol

	// Reuse the real bucket capability, including GC ownership and reader pins.
	// The hour-long sweep interval keeps this bounded boot out of maintenance.
	ctrl := volume_controller.NewController(le,
		&volume_controller.Config{DisablePeer: true, GcIntervalDur: "1h"}, nil,
		controller.NewInfo(volume_s4db.ControllerID, volume_s4db.Version, "OS image"),
		func(context.Context, *logrus.Entry) (volume.Volume, error) { return vol, nil })
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan error, 1)
	go func() { done <- ctrl.Execute(runCtx) }()
	r.releaseStorage = func() error {
		cancel()
		err := <-done
		if errors.Is(err, context.Canceled) {
			err = nil
		}

		// Execute performed the physical close; collect its cached result.
		return errors.Join(err, ctrl.Close(), vol.Close())
	}
	if _, err := ctrl.GetVolume(ctx); err != nil {
		return nil, err
	}

	// Only the exclusive seed creates bucket metadata.
	if _, _, _, err := vol.ApplyBucketConfig(ctx, &bucket.Config{Id: imageBucket, Rev: 1}); err != nil {
		return nil, err
	}
	handle, release, err := ctrl.BuildBucketAPI(ctx, imageBucket)
	if err != nil {
		return nil, err
	}
	closeStorage := r.releaseStorage
	r.releaseStorage = func() error { release(); return closeStorage() }
	bkt := handle.GetBucket()
	publisher, ok := bkt.(block.AtomicPublisher)
	if !ok || !block.SupportsRootRetention(bkt) || !publisher.SupportsAtomicPublication() ||
		publisher.AtomicPublicationVolumeID() != vol.GetID() {
		return nil, errors.New("image requires tracked atomic publication on its Volume")
	}

	// Keep construction heads process-local; only SeedRoot selects the final one.
	r.publisher = publisher
	head := &bucket.ObjectRef{BucketId: imageBucket}
	cursor := bucket_lookup.NewCursor(ctx, nil, le, nil, bkt, nil, head,
		&bucket.BucketOpArgs{BucketId: imageBucket, VolumeId: vol.GetID()}, nil)
	r.engine, err = world_block.NewEngine(ctx, le, cursor, world.NewSchema(unixfs_world.UnixfsTypes).LookupOp, nil, false)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// publishImageHead selects the already-fenced image once, using the engine's
// generated HeadState and ordinary durable publication/retention transaction.
func publishImageHead(ctx context.Context, publisher block.AtomicPublisher, next *bucket.ObjectRef) error {
	return publisher.PublishAtomic(ctx, &block.AtomicPublication{
		RootName: world_block.RetainedRootName(imageHeadStore, []byte(imageHeadKey)),
		Root:     next.GetRootRef(),
		Roots:    []block.NamedRoot{{Name: selectedImageRoot, Ref: next.GetRootRef()}},
		Heads: []*block.AtomicHeadUpdate{{
			ObjectStoreID: imageHeadStore, Key: []byte(imageHeadKey),
			Replace: func(_ context.Context, _ []byte, found bool) ([]byte, error) {
				// The seed expects an absent head; preserve any existing selection.
				if found {
					return nil, coord.ErrStaleGeneration
				}
				return (&world_block_engine.HeadState{HeadRef: next}).MarshalVT()
			},
		}},
	})
}
