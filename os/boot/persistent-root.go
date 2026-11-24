//go:build linux

package boot

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/s4db"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/sirupsen/logrus"
)

// OpenRoot opens the image store in dir read-only and serves image, which the
// caller selected and the store must hold. It never creates a fixture.
//
// This reader runs no collector; the store's named roots hold every block of
// each revision, and a writer beside it never releases what they reach. The
// workload exits and the FUSE server joins before Close releases the store.
func OpenRoot(ctx context.Context, le *logrus.Entry, dir string, image *bucket.ObjectRef) (*Root, error) {
	// Refuse an unknown format or a root the store lacks, without writing.
	db, err := s4db.Open(filepath.Join(dir, "image.s4wave"), s4db.Options{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	if err := checkImageFormat(ctx, db); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	store := store_kvtx.NewKVTx(store_kvkey.NewDefaultKVKey(), db, nil)
	if _, found, err := store.GetBlock(ctx, image.GetRootRef()); err != nil || !found {
		return nil, errors.Join(errors.New("selected root missing; store preserved"), err, db.Close())
	}

	// Serve the tree through the image's transform.
	r, err := serveImage(ctx, le, store, image, db.Close, true)
	if err != nil {
		return nil, err
	}
	println("SPACEWAVE_OS_REOPEN root=" + image.GetRootRef().MarshalString())
	return r, nil
}

// openBuiltRoot serves the image a seed built in dir, before any card
// authorizes it, as the hosted probe and the seed's own check read it.
func openBuiltRoot(ctx context.Context, le *logrus.Entry, dir string) (*Root, error) {
	image, err := ReadImage(ctx, dir)
	if err != nil {
		return nil, err
	}
	return OpenRoot(ctx, le, dir, image)
}
