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
	os_image "github.com/s4wave/spacewave/os/image"
)

const (
	// imageBucket scopes the image store's blocks and durable roots.
	imageBucket = "os-image"
	// builtImageRoot names the image a seed built, through the block
	// imageBlock writes, until a card authorizes it as the accepted revision.
	builtImageRoot = "image"
)

// ReadImage returns the image a seed built in dir, the image an authorization
// signs, reading the store without changing a byte. It builds on every
// platform, so a host that signs or installs an image reads the same image a
// booting machine serves.
func ReadImage(ctx context.Context, dir string) (_ *bucket.ObjectRef, retErr error) {
	// Open the store read-only and read the built image.
	db, err := s4db.Open(filepath.Join(dir, "image.s4wave"), s4db.Options{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer func() { retErr = errors.Join(retErr, db.Close()) }()
	if err := checkImageFormat(ctx, db); err != nil {
		return nil, err
	}
	built, err := readBootRoot(ctx, db, imageBucket, builtImageRoot)
	if err != nil {
		return nil, err
	}
	return readImageBlock(ctx, store_kvtx.NewKVTx(store_kvkey.NewDefaultKVKey(), db, nil), built)
}

// readImageBlock reads and decodes the block that names an image, as
// imageBlock wrote it.
func readImageBlock(ctx context.Context, store blockReader, ref *block.BlockRef) (*bucket.ObjectRef, error) {
	// Read the block, refusing a store that lacks it.
	data, found, err := store.GetBlock(ctx, ref)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("image block " + ref.MarshalString() + " missing; store preserved")
	}

	// Decode the image and check that it names one.
	image := &bucket.ObjectRef{}
	if err := image.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return image, os_image.ValidateImage(image)
}

// readBootRoot reads a named root that must be held, in one immutable
// snapshot. It cannot adopt an older head, initialize data or move a root.
func readBootRoot(ctx context.Context, db *s4db.DB, bucket, name string) (*block.BlockRef, error) {
	// Read the name from one snapshot; a free name is an error here.
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	ref, err := readNamedRoot(ctx, volume_kvtx.OpenRecords(tx), bucket, name)
	if err == nil && ref == nil {
		err = errors.New("boot root " + name + " absent; store preserved")
	}
	return ref, err
}

// checkImageFormat refuses a store in any format but this one's, without
// modifying a byte, so nothing migrates or adopts an unknown store.
func checkImageFormat(ctx context.Context, db *s4db.DB) error {
	// Read the stored format version.
	keys := store_kvkey.NewDefaultKVKey()
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	data, found, err := tx.Get(ctx, keys.GetFormatVersionKey())
	tx.Discard()
	if err != nil {
		return err
	}

	// Accept only this build's format.
	version, err := store_kvkey.ParseFormatVersion(data)
	if err != nil || !found || version != store_kvkey.FormatVersion {
		return errors.Join(errors.New("unsupported image store format; store preserved"), err)
	}
	return nil
}

// readNamedRoot returns the block a named root holds, or nil when the name is
// free. It never adopts, initializes or moves a root.
func readNamedRoot(ctx context.Context, records *block_gc.Records, bucket, name string) (*block.BlockRef, error) {
	// Decode just the indexed root record.
	entry, err := records.GetRoot(ctx, bucket, name)
	if err != nil || entry == nil {
		return nil, err
	}
	ref, _, err := block_gc.ParseBlockKey(entry.GetRef())
	if err != nil {
		return nil, err
	}
	if ref.GetEmpty() {
		return nil, errors.New("named root " + name + " empty; store preserved")
	}
	return ref, nil
}
