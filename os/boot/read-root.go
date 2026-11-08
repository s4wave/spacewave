//go:build linux

package boot

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/s4db"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

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
