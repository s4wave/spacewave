package block_store_s3

import (
	"context"
	"math"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
)

// ScanMeasurementPacks streams pack numbers, IDs and lengths from a frozen
// local index. It never loads the bucket or a pack tail.
func ScanMeasurementPacks(ctx context.Context, store kvtx.Store, yield func(uint32, string, uint64) error) error {
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	return tx.ScanPrefix(ctx, []byte(indexPackPrefix), func(key, value []byte) error {
		// Match the pack's number to its entry in the same snapshot.
		row, err := unmarshalPackRow(value)
		if err != nil {
			return err
		}
		id := string(key[len(indexPackPrefix):])
		data, found, err := tx.Get(ctx, []byte(indexEntryPrefix+id))
		if err != nil {
			return err
		}
		if !found {
			return errors.New("measurement index has a pack without an entry")
		}

		// Pass only the pack identity and denominator to the disk catalog.
		entry := &packfile.PackfileEntry{}
		if err := entry.UnmarshalVT(data); err != nil {
			return err
		}
		return yield(row.num, id, entry.GetSizeBytes())
	})
}

// LocateMeasurementBlock reads only the frozen index's extent row. Missing
// rows and rows without extents are unresolved, never evidence of a true hole.
func LocateMeasurementBlock(ctx context.Context, store kvtx.Store, ref *block.BlockRef) (uint32, int64, int, bool, error) {
	// Read the one hash row without opening a PackStore or its background jobs.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return 0, 0, 0, false, err
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, hashRowKey(packfile.BlockKey(ref.GetHash())))
	if err != nil {
		return 0, 0, 0, false, err
	}

	// Refuse malformed or overflowing extents before a range request.
	row, valid := parseHashValue(data)
	if !found || !valid || !row.extent {
		return 0, 0, 0, false, nil
	}
	if row.off > math.MaxInt64 {
		return 0, 0, 0, false, errors.New("measurement extent overflows")
	}
	off := int64(row.off)
	if row.size == 0 || int64(row.size) > math.MaxInt64-off {
		return 0, 0, 0, false, errors.New("measurement extent overflows")
	}
	return row.num, off, int(row.size), true, nil
}
