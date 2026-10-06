//go:build !js && !wasip1

package volume_s4db

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"

	bdb "github.com/aperturerobotics/bbolt"
	bdb_errors "github.com/aperturerobotics/bbolt/errors"
	"github.com/s4wave/spacewave/db/s4db"
)

// boltMagic is the magic number in a bbolt meta page.
const boltMagic = 0xED0CDAED

// boltMagicOff is the file offset of the first meta page's magic number,
// after the page header.
const boltMagicOff = 16

// boltBucket is the bucket a bolt Volume keeps every key in.
var boltBucket = []byte("hydra")

// convertBatch is the number of value bytes copied per s4db commit.
const convertBatch = 64 << 20

// convertBolt replaces a bolt Volume file at path with an s4db file holding
// the same keys, so the Volume keeps its key, peer ID, and data. It does nothing
// when the file is absent or not a bolt file.
//
// The copy goes to a sibling file renamed over path once verified, so a
// crash leaves the bolt file in place for the next open to convert again.
func convertBolt(ctx context.Context, path string) error {
	// Read the magic number.
	isBolt, err := isBoltFile(path)
	if err != nil || !isBolt {
		return err
	}

	// Lock the bolt file against other converters. A converter that opens
	// the path after another replaced it, or waited for the lock while it
	// did, leaves the path alone.
	src, err := bdb.Open(path, 0o600, &bdb.Options{Exclusive: true})
	if errors.Is(err, bdb_errors.ErrInvalid) {
		return removeBoltLocks(path)
	}
	if err != nil {
		return err
	}
	defer src.Close()
	if same, err := isOpenFile(path, src); err != nil || !same {
		return err
	}

	// Copy the keys into a fresh sibling file and check it.
	tmp := path + ".converting"
	if err := os.Remove(tmp); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dst, err := s4db.Open(tmp, s4db.Options{})
	if err != nil {
		return err
	}
	err = copyBolt(ctx, src, dst)
	if err == nil {
		err = verifyBolt(ctx, src, dst)
	}
	if err := errors.Join(err, dst.Close()); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}

	// Replace the bolt file and drop its lock files.
	if err := src.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	if err := syncDir(filepath.Dir(path)); err != nil {
		return err
	}
	return removeBoltLocks(path)
}

// removeBoltLocks removes the lock files bbolt keeps beside path.
func removeBoltLocks(path string) error {
	for _, lock := range []string{path + "-lock", path + "-lock-coord"} {
		if err := os.Remove(lock); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// isBoltFile reports whether the file at path starts with a bbolt meta page.
func isBoltFile(path string) (bool, error) {
	// Open the file if it exists.
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()

	// Compare the magic number, treating a short file as another format.
	var magic [4]byte
	if _, err := f.ReadAt(magic[:], boltMagicOff); err != nil {
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		return false, err
	}
	return binary.LittleEndian.Uint32(magic[:]) == boltMagic, nil
}

// isOpenFile reports whether path still names the file db has open.
func isOpenFile(path string, db *bdb.DB) (bool, error) {
	err := db.ValidatePath()
	if errors.Is(err, bdb_errors.ErrLockFileChanged) {
		return false, nil
	}
	return err == nil, err
}

// copyBolt copies every key of the bolt Volume bucket into dst, committing
// once per convertBatch bytes.
func copyBolt(ctx context.Context, src *bdb.DB, dst *s4db.DB) error {
	return src.View(func(btx *bdb.Tx) error {
		// An empty bolt file has no bucket and nothing to copy.
		bkt := btx.Bucket(boltBucket)
		if bkt == nil {
			return nil
		}

		// Copy the keys in order, committing each full batch.
		c := bkt.Cursor()
		for k, v := c.First(); k != nil; {
			tx, err := dst.NewTransaction(ctx, true)
			if err != nil {
				return err
			}
			var size int
			for ; k != nil && size < convertBatch; k, v = c.Next() {
				if err := tx.Set(ctx, k, v); err != nil {
					tx.Discard()
					return err
				}
				size += len(v)
			}
			if err := tx.Commit(ctx); err != nil {
				return err
			}
		}
		return nil
	})
}

// verifyBolt checks that dst holds exactly the keys and values of the bolt
// Volume bucket.
func verifyBolt(ctx context.Context, src *bdb.DB, dst *s4db.DB) error {
	// Read dst from one snapshot.
	tx, err := dst.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	it := tx.Iterate(ctx, nil, true, false)
	defer it.Close()

	// Walk both stores in key order, comparing each entry.
	err = src.View(func(btx *bdb.Tx) error {
		bkt := btx.Bucket(boltBucket)
		if bkt == nil {
			return nil
		}
		return bkt.ForEach(func(k, v []byte) error {
			// Match the key, then the value.
			if !it.Next() || !bytes.Equal(it.Key(), k) {
				return errors.New("converted volume is missing a key")
			}
			got, err := it.Value()
			if err != nil {
				return err
			}
			if !bytes.Equal(got, v) {
				return errors.New("converted volume has a different value")
			}
			return nil
		})
	})
	if err != nil {
		return err
	}
	if it.Next() {
		return errors.New("converted volume has an extra key")
	}
	return it.Err()
}

// syncDir makes a rename in dir durable. Windows cannot sync a directory and
// orders the rename itself.
func syncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
