//go:build !tinygo

package block_store_s3

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
)

// IndexHashRow is a decoded h row of a pack index, for offline readers of an
// index copy that open no PackStore.
type IndexHashRow struct {
	// Num is the number of the pack the row names.
	Num uint32
	// Off and Size are the block's bytes in the pack, when Extent is set.
	Off  uint64
	Size uint32
	// Extent reports that the row holds the block's extent; a row of the pack
	// number alone does not.
	Extent bool
}

// IndexHashRowKey returns the key of the h row of a block key, relative to
// the index prefix.
func IndexHashRowKey(key []byte) []byte {
	return hashRowKey(key)
}

// ParseIndexHashRow decodes an h row value; ok is false for a malformed one.
func ParseIndexHashRow(value []byte) (IndexHashRow, bool) {
	v, ok := parseHashValue(value)
	return IndexHashRow{Num: v.num, Off: v.off, Size: v.size, Extent: v.extent}, ok
}

// ReadIndexPackIDs reads the p rows of a pack index in tx and returns the
// pack ids of each pack number they give, in id order. An index written
// before its pack table loaded may give one number to several packs.
func ReadIndexPackIDs(ctx context.Context, tx kvtx.Tx) (map[uint32][]string, error) {
	ids := make(map[uint32][]string)
	err := tx.ScanPrefix(ctx, []byte(indexPackPrefix), func(key, value []byte) error {
		row, err := unmarshalPackRow(value)
		if err != nil {
			return err
		}
		ids[row.num] = append(ids[row.num], string(key[len(indexPackPrefix):]))
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "read pack table")
	}
	return ids, nil
}

// ReadIndexEntry reads the e row of pack id in tx, or nil when it has none.
func ReadIndexEntry(ctx context.Context, tx kvtx.Tx, id string) (*packfile.PackfileEntry, error) {
	return getEntry(ctx, tx, id)
}
