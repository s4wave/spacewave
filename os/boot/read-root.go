package boot

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

const (
	// imageBucket scopes this disposable image's blocks and durable roots.
	imageBucket = "os-image"
	// selectedImageRoot keeps the selected image even if the World later advances.
	selectedImageRoot = "selected-image"
)

// ReadImageRoot returns the selected root of the image store in dir, the root
// an image authorization signs, reading the store without changing a byte.
// It builds on every platform, so a host that signs or installs an image reads
// the same root a booting machine opens.
func ReadImageRoot(ctx context.Context, dir string) (_ *block.BlockRef, retErr error) {
	// Open the store read-only and inspect its selected head.
	db, err := s4db.Open(filepath.Join(dir, "image.s4wave"), s4db.Options{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, db.Close()) }()
	head, err := inspectImage(ctx, db)
	if err != nil {
		return nil, err
	}
	return head.GetRootRef(), nil
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

	// Read the selected root itself: it is both the head and its durable hold.
	ref, err := readBootRoot(ctx, db, imageBucket, selectedImageRoot)
	if err != nil {
		return nil, err
	}
	head := &bucket.ObjectRef{BucketId: imageBucket, RootRef: ref}
	store := store_kvtx.NewKVTx(keys, db, nil)
	if _, found, err := store.GetBlock(ctx, ref); err != nil || !found {
		return nil, errors.Join(errors.New("selected root missing; store preserved"), err)
	}
	return head, nil
}

// readBootRoot reads the authoritative held root in one immutable native snapshot.
// It cannot adopt an older head, initialize missing data or move a named root.
func readBootRoot(ctx context.Context, db *s4db.DB, bucket, name string) (*block.BlockRef, error) {
	// Decode just the indexed root record from the existing store.
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	entry, err := volume_kvtx.OpenRecords(tx).GetRoot(ctx, bucket, name)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, errors.New("boot root absent; store preserved")
	}

	// Decode the authoritative root reference without changing its hold.
	ref, _, err := block_gc.ParseBlockKey(entry.GetRef())
	if err != nil {
		return nil, err
	}
	if ref.GetEmpty() {
		return nil, errors.New("boot root empty; store preserved")
	}
	return ref, nil
}
