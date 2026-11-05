package kvtx

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

// CopyProgress counts the work of a running CopyStore.
type CopyProgress struct {
	// Keys is the number of keys copied.
	Keys uint64
	// Bytes is the size of the keys and values copied.
	Bytes uint64
	// UnknownBlocks is the number of blocks without a child list that a
	// reachable copy's walk reached. The blocks they name are not reached.
	UnknownBlocks uint64
	// AbsentBlocks is the number of reached blocks a parent block names that
	// the source does not hold: holes, which name nothing a reader can reach.
	AbsentBlocks uint64
	// DroppedBlocks is the number of blocks a copy left out.
	DroppedBlocks uint64
	// DroppedRecords is the number of liveness records a reachable copy left
	// out.
	DroppedRecords uint64
	// DroppedBytes is the size of the keys left out. Their values are not
	// read.
	DroppedBytes uint64
}

// CopyStore writes the Volume store src into the empty store dst, every key
// as it is, so dst holds src's format and liveness; opening dst upgrades an
// older format. src is read in one snapshot and must have no writer. The
// format version is written last in a durable commit; a dst without it is an
// incomplete copy to discard. progress, when set, is called after each
// commit.
func CopyStore(ctx context.Context, src, dst kvtx.Store, keys *store_kvkey.KVKey, progress func(CopyProgress)) error {
	return CopyStoreSplitting(ctx, src, dst, keys, SplitOptions{}, progress)
}

// DropBlocks reports which of refs a copy leaves out, in the order of refs.
type DropBlocks func(ctx context.Context, refs []*block.BlockRef) ([]bool, error)

// KeptBlockKeys returns the extra keys a copy writes beside a block it keeps
// when drop leaves others out. value is the block's data. src is the copy's
// snapshot of the source.
type KeptBlockKeys func(ctx context.Context, src kvtx.Tx, ref *block.BlockRef, value []byte) ([]CopyKey, error)

// dropCheckBlocks bounds the blocks one DropBlocks call checks.
const dropCheckBlocks = 1000

// CopyStoreDropping copies src into dst like CopyStore, leaving out the bytes
// of the blocks drop reports. Every other key is copied as it is, the dropped
// blocks' child lists too, so their liveness is unchanged. drop sees the
// blocks in key order, up to dropCheckBlocks at a time, and a dropped block's
// value is never read.
func CopyStoreDropping(ctx context.Context, src, dst kvtx.Store, keys *store_kvkey.KVKey, drop DropBlocks, progress func(CopyProgress)) error {
	return CopyStoreSplitting(ctx, src, dst, keys, SplitOptions{Drop: drop}, progress)
}

// CopyStoreSplitting copies src into dst like CopyStoreDropping, with the
// extra keys and omissions of opts.
func CopyStoreSplitting(ctx context.Context, src, dst kvtx.Store, keys *store_kvkey.KVKey, opts SplitOptions, progress func(CopyProgress)) error {
	ctx = block.WithBackground(ctx)
	return copyStore(ctx, src, dst, keys, nil, opts, progress)
}

// CopyReachable writes the Volume store src into the empty store dst like
// CopyStore, keeping only what a complete collection would keep: the blocks a
// named root reaches through child lists, their child lists and proofs, the
// root table and every key outside the blocks and the liveness records. The
// copy keeps no count, journal or free queue, so dst opens unadopted and an
// adoption counts its holders.
//
// src must be at the current format. It refuses, before writing anything, a
// source whose walk reaches a block stored without a child list, unless
// opts.AllowUnknown is set: the blocks such a block names are not reached and
// may be live.
func CopyReachable(ctx context.Context, src, dst kvtx.Store, keys *store_kvkey.KVKey, opts CopyReachableOptions, progress func(CopyProgress)) error {
	// Require a source whose liveness the walk reads.
	ctx = block.WithBackground(ctx)
	version, err := readFormatVersion(ctx, src, keys)
	if err != nil {
		return err
	}
	if version != store_kvkey.FormatVersion {
		return fmt.Errorf("reachable copy needs store format version %d, found %d; open it to upgrade it first", store_kvkey.FormatVersion, version)
	}
	return copyStore(ctx, src, dst, keys, &opts, SplitOptions{}, progress)
}

// copyStore copies src into the empty store dst and records the format
// version last. With reachable set, it first walks from the roots and copies
// only what they keep.
func copyStore(
	ctx context.Context,
	src, dst kvtx.Store,
	keys *store_kvkey.KVKey,
	reachable *CopyReachableOptions,
	opts SplitOptions,
	progress func(CopyProgress),
) error {
	// Refuse a destination that already holds keys.
	if err := requireEmptyStore(ctx, dst); err != nil {
		return err
	}

	// Read the source in one snapshot.
	stx, err := src.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer stx.Discard()
	w := &copyWriter{store: dst, progress: progress, drop: opts.Drop, kept: opts.Kept, omit: opts.Omit, blockPrefix: keys.GetBlockFullPrefix()}

	// Walk the roots in the same snapshot, and refuse unknown blocks.
	if reachable != nil {
		rec := block_gc.NewRecords(stx, livenessPrefix)
		mark, err := block_gc.Reachable(ctx, rec, func(ctx context.Context, ref *block.BlockRef) (bool, error) {
			rm, err := ref.MarshalKey()
			if err != nil {
				return false, err
			}
			return stx.Exists(ctx, keys.GetBlockKey(rm))
		})
		if err != nil {
			return fmt.Errorf("walk from the named roots: %w", err)
		}
		w.mark = mark
		w.counts.AbsentBlocks = uint64(mark.Absent)   //nolint:gosec // counts are never negative.
		w.counts.UnknownBlocks = uint64(mark.Unknown) //nolint:gosec // counts are never negative.
		w.report()
		if mark.Unknown != 0 && !reachable.AllowUnknown {
			return fmt.Errorf("%d reached blocks have no child list, so the blocks they name may be live and would be dropped; sample %v", mark.Unknown, mark.UnknownSample)
		}
	}

	// Copy the keys.
	versionKey := keys.GetFormatVersionKey()
	version, found, err := stx.Get(ctx, versionKey)
	if err != nil {
		return err
	}
	version = bytes.Clone(version)
	if err := w.copyKeys(ctx, stx, versionKey); err != nil {
		return err
	}

	// Record the version last in a durable commit, which also makes the
	// ordered commits durable. A reachable copy starts unadopted.
	tx, err := dst.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if reachable != nil {
		state := &block_gc.LivenessState{Adoption: block_gc.Adoption_ADOPTION_UNADOPTED}
		if err := block_gc.NewRecords(tx, livenessPrefix).SetState(ctx, state); err != nil {
			return err
		}
	}
	if found {
		if err := tx.Set(ctx, versionKey, version); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// requireEmptyStore returns an error unless store holds no keys.
func requireEmptyStore(ctx context.Context, store kvtx.Store) error {
	// Count the keys in a snapshot.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	size, err := tx.Size(ctx)
	if err != nil {
		return err
	}
	if size != 0 {
		return errors.New("copy destination is not empty")
	}
	return nil
}

// copyWriter writes the destination of a CopyStore in ordered commits.
type copyWriter struct {
	// store is the destination.
	store kvtx.Store
	// progress receives the counts after each commit, when set.
	progress func(CopyProgress)
	// counts is the work committed so far.
	counts CopyProgress
	// mark holds what the roots reach in a reachable copy; nil copies every
	// key.
	mark *block_gc.Mark
	// blockPrefix is the prefix of the block keys.
	blockPrefix []byte
	// drop reports the blocks a dropping copy leaves out; nil keeps them.
	drop DropBlocks
	// kept returns the extra keys written beside a block a dropping copy
	// keeps; nil writes none.
	kept KeptBlockKeys
	// omit are the key prefixes the copy leaves out.
	omit [][]byte
	// held are the block keys awaiting a drop check, in key order.
	held [][]byte
}

// copyKeys copies every key of stx except versionKey, in commits of up to
// migrateBatchBytes.
func (w *copyWriter) copyKeys(ctx context.Context, stx kvtx.Tx, versionKey []byte) error {
	// Walk every key in order.
	it := stx.Iterate(ctx, nil, true, false)
	defer it.Close()
	if err := it.Seek(nil); err != nil {
		return err
	}

	// Fill each batch, seeking past an omitted prefix in one step.
	var batch []keyValue
	var size int
	for it.Valid() {
		key := it.Key()
		if prefix := w.omitted(key); prefix != nil {
			if err := it.Seek(prefixEnd(prefix)); err != nil {
				return err
			}
			continue
		}
		if !bytes.Equal(key, versionKey) {
			key = bytes.Clone(key)
			keep, err := w.keep(key)
			if err != nil {
				return err
			}

			// Hold a block for the drop check, writing the held blocks
			// before any later key so the destination fills in key order.
			held := keep && w.drop != nil && bytes.HasPrefix(key, w.blockPrefix)
			if held {
				w.held = append(w.held, key)
				if len(w.held) == dropCheckBlocks {
					batch, size, err = w.checkHeld(ctx, stx, batch, size)
				}
			} else if keep {
				batch, size, err = w.checkHeld(ctx, stx, batch, size)
			}
			if err != nil {
				return err
			}

			// Read the value only of a kept key: a store may read values
			// lazily, and most of a reachable copy's dropped bytes are
			// values.
			if keep && !held {
				value, err := it.ValueCopy(nil)
				if err != nil {
					return err
				}
				batch = append(batch, keyValue{key: key, value: value})
				size += len(key) + len(value)
			} else if !keep {
				w.counts.DroppedBytes += uint64(len(key)) //nolint:gosec // lengths are never negative.
			}
		}
		if size >= migrateBatchBytes {
			if err := w.commitKeys(ctx, batch, size); err != nil {
				return err
			}
			batch, size = batch[:0], 0
		}
		it.Next()
	}
	if err := it.Err(); err != nil {
		return err
	}
	batch, size, err := w.checkHeld(ctx, stx, batch, size)
	if err != nil {
		return err
	}
	return w.commitKeys(ctx, batch, size)
}

// omitted returns the omitted prefix key starts with, or nil.
func (w *copyWriter) omitted(key []byte) []byte {
	for _, prefix := range w.omit {
		if bytes.HasPrefix(key, prefix) {
			return prefix
		}
	}
	return nil
}

// checkHeld asks drop which held blocks the copy leaves out, and appends the
// rest to batch with their values read from stx.
func (w *copyWriter) checkHeld(ctx context.Context, stx kvtx.Tx, batch []keyValue, size int) ([]keyValue, int, error) {
	// Skip a batch that holds no blocks.
	if len(w.held) == 0 {
		return batch, size, nil
	}

	// Decode the held block keys.
	refs := make([]*block.BlockRef, len(w.held))
	for i, key := range w.held {
		refs[i] = &block.BlockRef{}
		if err := refs[i].UnmarshalVT(key[len(w.blockPrefix):]); err != nil {
			return nil, 0, fmt.Errorf("decode block key %q: %w", key, err)
		}
	}

	// Ask which of them the copy leaves out.
	dropped, err := w.drop(ctx, refs)
	if err != nil {
		return nil, 0, err
	}
	if len(dropped) != len(refs) {
		return nil, 0, fmt.Errorf("drop check answered %d of %d blocks", len(dropped), len(refs))
	}

	// Count the dropped blocks and read the values of the rest.
	for i, key := range w.held {
		if dropped[i] {
			w.counts.DroppedBlocks++
			w.counts.DroppedBytes += uint64(len(key)) //nolint:gosec // lengths are never negative.
			continue
		}
		value, found, err := stx.Get(ctx, key)
		if err != nil {
			return nil, 0, err
		}
		if !found {
			return nil, 0, fmt.Errorf("block key %q left the snapshot", key)
		}
		value = bytes.Clone(value)
		batch = append(batch, keyValue{key: key, value: value})
		size += len(key) + len(value)
		if w.kept == nil {
			continue
		}
		extra, err := w.kept(ctx, stx, refs[i], value)
		if err != nil {
			return nil, 0, err
		}
		for _, k := range extra {
			batch = append(batch, keyValue{key: k.Key, value: k.Value})
			size += len(k.Key) + len(k.Value)
		}
	}
	w.held = w.held[:0]
	return batch, size, nil
}

// keep reports whether the copy writes key, counting a dropped key. A
// reachable copy keeps the root table, the child lists and proofs of reached
// blocks, the reached blocks and every key outside the blocks and the
// liveness records.
func (w *copyWriter) keep(key []byte) (bool, error) {
	// A full copy keeps every key.
	if w.mark == nil {
		return true, nil
	}

	// Keep the root table and the reached blocks' child lists and proofs.
	if record, ok := bytes.CutPrefix(key, livenessPrefix); ok {
		keep, err := w.keepRecord(record)
		if err == nil && !keep {
			w.counts.DroppedRecords++
		}
		return keep, err
	}

	// Keep a reached block and every key outside the blocks.
	data, ok := bytes.CutPrefix(key, w.blockPrefix)
	if !ok {
		return true, nil
	}
	ref := &block.BlockRef{}
	if err := ref.UnmarshalVT(data); err != nil {
		return false, fmt.Errorf("decode block key %q: %w", key, err)
	}
	if w.mark.Has(ref) {
		return true, nil
	}
	w.counts.DroppedBlocks++
	return false, nil
}

// keepRecord reports whether a reachable copy keeps the liveness record whose
// key after the prefix is record.
func (w *copyWriter) keepRecord(record []byte) (bool, error) {
	if len(record) == 0 {
		return false, nil
	}
	switch record[0] {
	case block_gc.RootRecordKind:
		return true, nil
	case block_gc.ChildrenRecordKind, block_gc.ProofRecordKind:
		ref, rest, err := block_gc.ParseBlockKey(record[1:])
		if err != nil || len(rest) != 0 {
			return false, fmt.Errorf("liveness record %x names no block", record)
		}
		return w.mark.Has(ref), nil
	}
	return false, nil
}

// commitKeys writes batch, holding size bytes, in one ordered commit.
func (w *copyWriter) commitKeys(ctx context.Context, batch []keyValue, size int) error {
	// Open the commit when there is work.
	if len(batch) == 0 {
		return nil
	}
	tx, err := w.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Write the batch.
	for _, m := range batch {
		if err := tx.Set(ctx, m.key, m.value); err != nil {
			return err
		}
	}
	if err := kvtx.CommitOrdered(ctx, tx); err != nil {
		return err
	}

	// Count the committed work.
	w.counts.Keys += uint64(len(batch))
	w.counts.Bytes += uint64(size) //nolint:gosec
	w.report()
	return nil
}

// report sends the committed counts to the progress callback.
func (w *copyWriter) report() {
	if w.progress != nil {
		w.progress(w.counts)
	}
}

// prefixEnd returns the first key after every key with prefix, which must
// not end in 0xff.
func prefixEnd(prefix []byte) []byte {
	end := bytes.Clone(prefix)
	end[len(end)-1]++
	return end
}
