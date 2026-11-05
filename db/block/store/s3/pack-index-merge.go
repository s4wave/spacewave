//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
)

// mergeEntryBatch bounds the e/ and p/ rows of one merge write transaction.
const mergeEntryBatch = 256

// ReadFunc runs fn on a read transaction of a store. A reader beside another
// process's writer may run fn more than once, so fn resets its results on
// entry and does not use the transaction after it returns.
type ReadFunc func(ctx context.Context, fn func(tx kvtx.Tx) error) error

// MergeProgress counts the work of MergePackIndex.
type MergeProgress struct {
	// Packs is the packs added to the destination or completed in it.
	Packs uint64
	// Keys is the h rows written.
	Keys uint64
	// Scanned is the h rows read from the source.
	Scanned uint64
}

// MergePackIndex adds the packs the pack index read through src holds to the
// pack index in dst, both under the same store prefix, as a union.
//
// A pack dst lacks gets the next number of dst and its e/ and p/ rows. A pack
// dst lists but has not indexed, which src has indexed, keeps its dst number.
// The h rows of those packs are copied with their numbers mapped, and each
// pack is marked indexed only after all its rows are committed, so an
// interrupted merge leaves its packs unindexed and a rerun, or the indexer of
// the pack store, completes them. A pack dst has indexed is left as it is. t/
// rows are a cache and are not copied.
//
// An h row names one pack that holds the block, so a row copied over another
// row names a pack dst also holds. A key whose row src rewrote for a later
// pack during the merge reads as unlocated in dst, which a lookup answers from
// the pack filters. src is read in short snapshots, and dst must not have
// another writer.
func MergePackIndex(ctx context.Context, dst kvtx.Store, src ReadFunc, progress func(MergeProgress)) (MergeProgress, error) {
	// Read the destination's table and the source's.
	var p MergeProgress
	into := newPackIndex(dst)
	if _, _, err := into.load(ctx); err != nil {
		return p, errors.Wrap(err, "load the destination index")
	}
	var from map[string]packRow
	err := src(ctx, func(tx kvtx.Tx) error {
		from = make(map[string]packRow)
		return tx.ScanPrefix(ctx, []byte(indexPackPrefix), func(key, value []byte) error {
			row, err := unmarshalPackRow(value)
			if err != nil {
				return err
			}
			from[string(key[len(indexPackPrefix):])] = row
			return nil
		})
	})
	if err != nil {
		return p, errors.Wrap(err, "read the source pack table")
	}

	// Choose the packs to copy: those dst lacks, and those only src indexed.
	var added []string
	numbers := make(map[uint32]packRow)
	for id, row := range from {
		have, ok := into.rows[id]
		switch {
		case !ok:
			added = append(added, id)
		case !have.indexed && row.indexed:
			numbers[row.num] = have
		}
	}
	slices.Sort(added)

	// Add the packs dst lacks, unindexed, numbering them in dst.
	for batch := range slices.Chunk(added, mergeEntryBatch) {
		var entries []*packfile.PackfileEntry
		err := src(ctx, func(tx kvtx.Tx) error {
			entries = entries[:0]
			for _, id := range batch {
				data, ok, err := tx.Get(ctx, []byte(indexEntryPrefix+id))
				if err != nil {
					return err
				}
				entry := &packfile.PackfileEntry{}
				if !ok || entry.UnmarshalVT(slices.Clone(data)) != nil || entry.GetId() != id {
					continue
				}
				entries = append(entries, entry)
			}
			return nil
		})
		if err != nil {
			return p, errors.Wrap(err, "read the source entries")
		}
		tx, err := dst.NewTransaction(ctx, true)
		if err != nil {
			return p, err
		}
		for _, entry := range entries {
			row := into.nextRow(entry.GetId(), false)
			if err := putPackRows(ctx, tx, entry, row); err != nil {
				tx.Discard()
				return p, err
			}
			if src := from[entry.GetId()]; src.indexed {
				numbers[src.num] = row
			}
		}
		err = tx.Commit(ctx)
		tx.Discard()
		if err != nil {
			return p, err
		}
	}

	// Copy the h rows of the chosen packs with their numbers mapped.
	if len(numbers) != 0 {
		if err := copyHashRows(ctx, dst, src, numbers, &p, progress); err != nil {
			return p, err
		}
	}

	// Mark the packs indexed now that all their rows are durable.
	rows := make([]packRow, 0, len(numbers))
	for _, row := range numbers {
		rows = append(rows, row)
	}
	for batch := range slices.Chunk(rows, mergeEntryBatch) {
		tx, err := dst.NewTransaction(ctx, true)
		if err != nil {
			return p, err
		}
		for _, row := range batch {
			row.indexed = true
			if err := tx.Set(ctx, []byte(indexPackPrefix+into.ids[row.num]), row.marshal()); err != nil {
				tx.Discard()
				return p, err
			}
		}
		err = tx.Commit(ctx)
		tx.Discard()
		if err != nil {
			return p, err
		}
		p.Packs += uint64(len(batch))
	}
	if progress != nil {
		progress(p)
	}
	return p, nil
}

// copyHashRows streams the h rows of src in key order, indexBatchKeys per
// snapshot, and writes each row that names a pack of numbers to dst with the
// pack's dst number.
func copyHashRows(ctx context.Context, dst kvtx.Store, src ReadFunc, numbers map[uint32]packRow, p *MergeProgress, progress func(MergeProgress)) error {
	// Track the source cursor and the destination pack number for each row.
	type hashRow struct {
		key   []byte
		value hashValue
	}
	prefix := []byte(indexHashPrefix)
	var after []byte
	for {
		// Read the next rows from one snapshot, starting again from after when
		// the reader retries a torn snapshot.
		var batch []hashRow
		var scanned uint64
		var next []byte
		err := src(ctx, func(tx kvtx.Tx) error {
			// Seek past the last row the previous batch read.
			batch, scanned, next = batch[:0], 0, after
			it := tx.Iterate(ctx, prefix, true, false)
			defer it.Close()
			if err := it.Seek(after); err != nil {
				return err
			}
			if after != nil && it.Valid() && bytes.Equal(it.Key(), after) {
				it.Next()
			}

			// Read up to a batch of rows.
			for ; it.Valid() && scanned < indexBatchKeys; it.Next() {
				// Keep the row when it names a chosen pack.
				scanned++
				next = bytes.Clone(it.Key())
				value, err := it.Value()
				if err != nil {
					return err
				}
				parsed, ok := parseHashValue(value)
				if !ok {
					continue
				}
				if row, ok := numbers[parsed.num]; ok {
					parsed.num = row.num
					batch = append(batch, hashRow{key: next, value: parsed})
				}
			}
			return it.Err()
		})
		if err != nil {
			return errors.Wrap(err, "read the source h rows")
		}

		// Record the scanned rows and stop when the source is exhausted.
		after = next
		if scanned == 0 {
			return nil
		}
		p.Scanned += scanned

		// Write the mapped rows in one transaction.
		if len(batch) != 0 {
			tx, err := dst.NewTransaction(ctx, true)
			if err != nil {
				return err
			}
			for _, row := range batch {
				if err := tx.Set(ctx, row.key, row.value.marshal()); err != nil {
					tx.Discard()
					return err
				}
			}
			err = tx.Commit(ctx)
			tx.Discard()
			if err != nil {
				return err
			}
			p.Keys += uint64(len(batch))
		}

		// Report the completed batch.
		if progress != nil {
			progress(*p)
		}
	}
}
