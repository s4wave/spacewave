//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// packTableRows are the row prefixes of a pack index that describe its packs
// rather than its blocks: each pack's entry, number and sweep state, and the
// next pack number.
var packTableRows = [][]byte{
	[]byte(indexEntryPrefix),
	[]byte(indexPackPrefix),
	[]byte(indexHoldPrefix),
	[]byte(indexNextKey),
}

// CopyPackTable copies the rows of a pack index that describe its packs from
// src to dst, both views under the index's prefix, and returns the rows
// copied. It leaves the h rows and the t/ rows, which hold nearly all of the
// index's bytes.
//
// A PackStore over the copy lists, holds and reads the same packs as one over
// src at the read src is, which is what a pack sweep's snapshot needs. It is
// not for block lookups: the p/ rows still say each indexed pack's keys are in
// h rows, which the copy lacks.
func CopyPackTable(ctx context.Context, src, dst kvtx.TxOps) (int, error) {
	n := 0
	for _, prefix := range packTableRows {
		err := src.ScanPrefix(ctx, prefix, func(key, value []byte) error {
			n++
			return dst.Set(ctx, bytes.Clone(key), bytes.Clone(value))
		})
		if err != nil {
			return n, errors.Wrapf(err, "copy the %q rows", prefix)
		}
	}
	return n, nil
}
