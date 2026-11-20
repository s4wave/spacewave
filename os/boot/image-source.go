package boot

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// ImageSource is a RevisionSource over another image store, read-only, such
// as the store an image Step built. Its records come from one snapshot; it
// changes no byte of the store.
type ImageSource struct {
	// db is the read-only store.
	db *s4db.DB
	// tx is the snapshot that reads child lists.
	tx kvtx.Tx
	// records reads child lists from tx.
	records *block_gc.Records
	// blocks reads block data.
	blocks blockReader
}

// OpenImageSource opens the image store in dir as a revision source and
// returns it with the root its seed built. Close releases it.
func OpenImageSource(ctx context.Context, dir string) (_ *ImageSource, _ *block.BlockRef, retErr error) {
	// Open the store read-only in this build's format.
	db, err := s4db.Open(filepath.Join(dir, "image.s4wave"), s4db.Options{ReadOnly: true})
	if err != nil {
		return nil, nil, err
	}
	defer func() {
		if retErr != nil {
			retErr = errors.Join(retErr, db.Close())
		}
	}()
	if err := checkImageFormat(ctx, db); err != nil {
		return nil, nil, err
	}

	// Read the built root and hold the snapshot the walk reads.
	root, err := readBootRoot(ctx, db, imageBucket, builtImageRoot)
	if err != nil {
		return nil, nil, err
	}
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, err
	}
	return &ImageSource{
		db:      db,
		tx:      tx,
		records: volume_kvtx.OpenRecords(tx),
		blocks:  store_kvtx.NewKVTx(store_kvkey.NewDefaultKVKey(), db, nil),
	}, root, nil
}

// GetBlock returns a stored block and its child list.
func (s *ImageSource) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, []*block.BlockRef, error) {
	// Read the data and the child list; the store must hold both.
	data, found, err := s.blocks.GetBlock(ctx, ref)
	if err != nil {
		return nil, nil, err
	}
	children, listed, err := s.records.GetChildren(ctx, ref)
	if err != nil {
		return nil, nil, err
	}
	if !found || !listed {
		return nil, nil, errors.New("image source lacks block " + ref.MarshalString())
	}
	return data, children, nil
}

// Close releases the snapshot and the store.
func (s *ImageSource) Close() error {
	s.tx.Discard()
	return s.db.Close()
}
