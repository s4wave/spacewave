package packmove

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"io"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// unpackedBatch is the block keys FindUnpacked reads from one snapshot and
// locates in one call.
const unpackedBatch = 4096

// BlockLocator reports which blocks a set of packs holds from an index of
// their keys, without reading a pack.
// block_store_s3.PackStore.LocateBlocks implements it.
type BlockLocator interface {
	// LocateBlocks reports, for each ref, whether a pack holds it.
	LocateBlocks(ctx context.Context, refs []*block.BlockRef) ([]bool, error)
}

// UnpackedProgress counts the work of FindUnpacked.
type UnpackedProgress struct {
	// Examined is the number of block keys read from the volume.
	Examined uint64
	// Missing is the number of block keys no pack holds, which the list
	// records.
	Missing uint64
	// Unparsed is the number of keys that are not a block ref. They are in
	// the list, so PackList records them as bad.
	Unparsed uint64
}

// FindUnpacked streams every block key of src, in key order, and writes to w
// each one packs does not locate: one hex-encoded key, relative to the block
// prefix, per line. The list is in key order, as PackList reads it.
//
// src is only read, one snapshot per batch, and may be shared with the
// process that writes it. progress, when set, receives the totals after each
// batch.
func FindUnpacked(ctx context.Context, src kvtx.Store, keys *store_kvkey.KVKey, packs BlockLocator, w io.Writer, progress func(UnpackedProgress)) (UnpackedProgress, error) {
	// Walk the block keys from the start.
	var p UnpackedProgress
	out := bufio.NewWriter(w)
	prefix := keys.GetBlockFullPrefix()
	var after []byte
	for {
		// Read the next batch of keys from a snapshot the writer did not pass.
		var batch [][]byte
		err := ReadStable(ctx, src, func(tx kvtx.Tx) error {
			var err error
			batch, err = readBlockKeys(ctx, tx, prefix, after, unpackedBatch)
			return err
		})
		if err != nil {
			return p, err
		}
		if len(batch) == 0 {
			return p, out.Flush()
		}
		after = batch[len(batch)-1]

		// Locate the keys that parse as block refs. List every other
		// one.
		var refs []*block.BlockRef
		var parsed []int
		listed := make([]bool, len(batch))
		for i, key := range batch {
			ref := &block.BlockRef{}
			if ref.UnmarshalVT(key) != nil || ref.GetHash().Validate() != nil {
				listed[i] = true
				p.Unparsed++
				continue
			}
			refs = append(refs, ref)
			parsed = append(parsed, i)
		}
		found, err := packs.LocateBlocks(ctx, refs)
		if err != nil {
			return p, errors.Wrap(err, "locate blocks")
		}
		for j, ok := range found {
			listed[parsed[j]] = !ok
		}

		// Write the listed keys in key order.
		for i, key := range batch {
			if !listed[i] {
				continue
			}
			p.Missing++
			if _, err := out.WriteString(hex.EncodeToString(key) + "\n"); err != nil {
				return p, err
			}
		}
		p.Examined += uint64(len(batch))
		if progress != nil {
			progress(p)
		}
	}
}

// readBlockKeys returns up to n block keys of tx after the key after, relative
// to prefix, in key order. A nil after starts at the first key.
func readBlockKeys(ctx context.Context, tx kvtx.Tx, prefix, after []byte, n int) ([][]byte, error) {
	// Seek past after.
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()
	var start []byte
	if after != nil {
		start = append(bytes.Clone(prefix), after...)
	}
	if err := it.Seek(start); err != nil {
		return nil, err
	}
	if start != nil && it.Valid() && bytes.Equal(it.Key(), start) {
		it.Next()
	}

	// Collect the keys that follow.
	var keys [][]byte
	for ; it.Valid() && len(keys) < n; it.Next() {
		keys = append(keys, bytes.Clone(it.Key()[len(prefix):]))
	}
	return keys, it.Err()
}
