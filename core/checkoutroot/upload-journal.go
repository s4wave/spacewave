//go:build !js

package checkoutroot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/protocol"
)

// uploadJournal records the directory of each named stage on the host state volume.
type uploadJournal struct {
	// store is owned by the daemon and held for this placement's execution.
	store kvtx.Store
	// prefix distinguishes this placement and configuration from other roots.
	prefix string
}

// newUploadJournal binds recovery to a single placement of a folder.
func newUploadJournal(store kvtx.Store, id protocol.ID) *uploadJournal {
	sum := sha256.Sum256([]byte(id))
	return &uploadJournal{store: store, prefix: "checkout-root/uploads/" + hex.EncodeToString(sum[:]) + "/"}
}

// record makes the stage discoverable before its publisher writes any payload bytes.
func (j *uploadJournal) record(ctx context.Context, name, directory string) error {
	// Commit the location durably before letting any bytes reach the stage.
	tx, err := j.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte(j.prefix+name), []byte(directory)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// forget drops a location once no owned stage remains there.
func (j *uploadJournal) forget(ctx context.Context, name string) error {
	// A crash can safely replay this removal because cleanup checks the lease.
	tx, err := j.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Delete(ctx, []byte(j.prefix+name)); err != nil {
		return err
	}
	return kvtx.CommitOrdered(ctx, tx)
}

// next reads one location and closes the snapshot before recovery touches the host filesystem.
func (j *uploadJournal) next(ctx context.Context, after string) (key, directory string, err error) {
	// Seek directly beyond the last row within this placement's key range.
	tx, err := j.store.NewTransaction(ctx, false)
	if err != nil {
		return "", "", err
	}
	defer tx.Discard()
	it := tx.Iterate(ctx, []byte(j.prefix), true, false)
	defer it.Close()
	if after == "" {
		if !it.Next() {
			return "", "", it.Err()
		}
	} else if err := it.Seek([]byte(after + "\x00")); err != nil {
		return "", "", err
	}
	if !it.Valid() {
		return "", "", it.Err()
	}

	// Copy the location before releasing the borrowed iterator values.
	value, err := it.Value()
	return string(it.Key()), string(value), err
}
