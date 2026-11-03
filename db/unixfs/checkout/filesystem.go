package unixfs_checkout

import (
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/pkg/errors"
)

// Discard deletes an immutable checkout whole or not at all. The caller must
// hold its exclusive DestinationLease. It first
// moves path to a staging name, which Windows refuses while the game or a
// host holds one of its files open, so a directory still in use stays
// complete instead of losing the files deleted before the one held. Deleting
// the staging copy may still stop partway; the next eviction finishes it.
func Discard(path string) error {
	staging := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+"-Discard-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	if err := os.Rename(path, staging); err != nil {
		return err
	}
	return os.RemoveAll(staging)
}

// markUsed records that a checkout returned directory, so eviction keeps the
// most recently used checkouts.
func markUsed(directory string) {
	now := time.Now()
	_ = os.Chtimes(directory, now, now)
}

// publish preserves the old directory until staging has passed validation.
// The destination lease excludes other checkout/eviction writers. A rename
// refusal keeps the complete old directory, including open files on Windows.
func publish(staging, directory string) error {
	// A new destination needs only the final rename.
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		return os.Rename(staging, directory)
	} else if err != nil {
		return err
	}

	// Retain the old tree until the new tree occupies the destination.
	previous := filepath.Join(filepath.Dir(directory), "."+filepath.Base(directory)+"-discard-"+strconv.FormatInt(time.Now().UnixNano(), 36))
	if err := os.Rename(directory, previous); err != nil {
		return err
	}
	if err := os.Rename(staging, directory); err != nil {
		if restore := os.Rename(previous, directory); restore != nil {
			return errors.Wrapf(err, "restore previous checkout: %v", restore)
		}
		return err
	}

	// Only the displaced immutable cache copy can be removed now.
	_ = os.RemoveAll(previous)
	return nil
}
