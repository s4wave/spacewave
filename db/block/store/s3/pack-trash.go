//go:build !tinygo

package block_store_s3

import (
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
)

// trashDir holds, under the object prefix, the entries and packs a sweep moved
// out of the store, until the operator purges them.
const trashDir = "trash/"

// TrashPackKey returns the object key of pack id in the trash under prefix.
func TrashPackKey(prefix, id string) string {
	return prefix + trashDir + packDir + id
}

// TrashEntryKey returns the object key of the entry of pack id in the trash
// under prefix.
func TrashEntryKey(prefix, id string) string {
	return prefix + trashDir + entryDir + id
}

// KeyHolders is a block key a removed pack holds, with the other packs a
// snapshot lists holding it.
type KeyHolders struct {
	// Key is the block key.
	Key []byte
	// Others are the other packs that hold it.
	Others []string
}

// BeginTrash moves the quarantined packs ids to the trashing state, before
// the command moves their entries and files to the trash. The fallback still
// reads a trashing pack, and records the read as a miss, which keeps Remove
// from removing it. A trashing pack stays as it is, so a repeated call
// changes nothing; a registered pack or one the table lacks fails the call
// before any row is written.
func (s *PackStore) BeginTrash(ctx context.Context, ids []string) error {
	// Refuse a store that holds nothing.
	if s.index == nil || !s.keepPacks {
		return errNoSweep
	}
	if err := s.loadEntries(ctx); err != nil {
		return err
	}
	release, err := s.holdMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Choose the quarantined packs, refusing any pack the sweep has not held.
	var move []string
	for _, id := range ids {
		state, ok := s.index.heldState(id)
		switch {
		case !ok:
			return errors.Errorf("trash pack %s: the index holds no such pack", id)
		case state == PackRegistered:
			return errors.Errorf("trash pack %s: it is not quarantined", id)
		case state == PackQuarantined && !slices.Contains(move, id):
			move = append(move, id)
		}
	}
	if len(move) == 0 {
		return nil
	}

	// Write their rows with no miss, then flip their state.
	now := time.Now().UnixNano()
	holds := make([]packHold, len(move))
	for i := range holds {
		holds[i] = packHold{state: PackTrashing, since: now}
	}
	if err := s.index.writeHolds(ctx, move, holds); err != nil {
		return err
	}
	s.setHolds(move, PackTrashing)
	s.le.WithField("packs", len(move)).Info("trashing packs")
	return nil
}

// Remove removes the trashing packs ids, whose entries and files the command
// moved to the trash. keys are the block keys the packs hold, each with the
// other packs a snapshot lists holding it: each h row that names a removed
// pack is pointed at the first of those the store holds, registered before
// held, or deleted when it holds none. The rows are repaired before the
// packs' own rows go, so a call killed between them is finished by repeating
// it. A pack the table lacks counts as removed. A pack that is not trashing,
// that a read missed while trashing, or whose entry or file is not in the
// trash fails the call before anything changes.
func (s *PackStore) Remove(ctx context.Context, ids []string, keys []KeyHolders) error {
	// Refuse a store that holds nothing.
	if s.index == nil || !s.keepPacks {
		return errNoSweep
	}
	if err := s.loadEntries(ctx); err != nil {
		return err
	}
	release, err := s.holdMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Choose the trashing packs the table still holds, refusing one a read
	// missed.
	var remove []string
	for _, id := range ids {
		state, ok := s.index.heldState(id)
		if !ok || slices.Contains(remove, id) {
			continue
		}
		if state != PackTrashing {
			return errors.Errorf("remove pack %s: it is %s, not trashing", id, state)
		}
		hold, _, err := s.index.readHold(ctx, id)
		if err != nil {
			return err
		}
		if hold.event != 0 {
			return errors.Errorf("remove pack %s: a read missed it while trashing; restore it", id)
		}
		remove = append(remove, id)
	}
	if len(remove) == 0 {
		return nil
	}

	// Check each pack's entry and file are in the trash and out of the store.
	for _, id := range remove {
		if err := s.checkTrashed(ctx, id); err != nil {
			return errors.Wrap(err, "remove pack "+id)
		}
	}

	// Repair the h rows, then drop the packs.
	if err := s.index.repairRows(ctx, remove, keys); err != nil {
		return errors.Wrap(err, "repair the h rows of the removed packs")
	}
	if err := s.index.remove(ctx, remove); err != nil {
		return err
	}
	s.updateEntries(nil, remove)
	s.le.WithField("packs", len(remove)).Info("removed trashed packs")
	return nil
}

// checkTrashed checks that the entry and file of pack id are in the trash and
// gone from their keys in the store, reading one byte of each.
func (s *PackStore) checkTrashed(ctx context.Context, id string) error {
	// Each key in the store must be gone.
	for _, key := range []string{s.prefix + entryDir + id, s.prefix + packDir + id} {
		if _, err := s.client.GetObjectRange(ctx, s.bucket, key, 0, 1); !errors.Is(err, ErrNotFound) {
			if err == nil {
				return errors.Errorf("%s is still in the store", key)
			}
			return err
		}
	}

	// Each key in the trash must be there.
	for _, key := range []string{TrashEntryKey(s.prefix, id), TrashPackKey(s.prefix, id)} {
		if _, err := s.client.GetObjectRange(ctx, s.bucket, key, 0, 1); err != nil {
			return errors.Wrap(err, "find "+key)
		}
	}
	return nil
}

// repairRows points each h row of keys that names one of the packs ids at the
// first other holder of the key the table holds, registered before held and
// none of ids, or deletes the row when there is none. A number another pack
// shares, as an index written before its table loaded gave out, is left on a
// row with no other holder, since the row may be that pack's. It writes in
// transactions of indexBatchKeys keys.
func (x *packIndex) repairRows(ctx context.Context, ids []string, keys []KeyHolders) error {
	// Read the removed packs' numbers, and the numbers another pack shares.
	x.mtx.RLock()
	gone := make(map[string]bool, len(ids))
	removed := make(map[uint32]bool, len(ids))
	for _, id := range ids {
		if row, ok := x.rows[id]; ok {
			gone[id], removed[row.num] = true, true
		}
	}
	shared := make(map[uint32]bool)
	for id, row := range x.rows {
		if removed[row.num] && !gone[id] {
			shared[row.num] = true
		}
	}
	x.mtx.RUnlock()

	// Repair a batch of keys in each transaction.
	for batch := range slices.Chunk(keys, indexBatchKeys) {
		if err := x.repairBatch(ctx, gone, removed, shared, batch); err != nil {
			return err
		}
	}
	return nil
}

// repairBatch repairs the h rows of keys in one transaction, as repairRows
// describes.
func (x *packIndex) repairBatch(ctx context.Context, gone map[string]bool, removed, shared map[uint32]bool, keys []KeyHolders) error {
	// Open the transaction.
	tx, err := x.store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Repoint or delete each row that names a removed pack.
	for _, k := range keys {
		rowKey := hashRowKey(k.Key)
		value, found, err := tx.Get(ctx, rowKey)
		if err != nil {
			return err
		}
		row, ok := parseHashValue(value)
		if !found || !ok || !removed[row.num] {
			continue
		}
		if err := x.repairRow(ctx, tx, rowKey, gone, shared[row.num], k.Others); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// repairRow points the h row at rowKey, which names a removed pack, at the
// first holder of others the table holds and gone lacks, or deletes it unless
// another pack shares the removed pack's number.
func (x *packIndex) repairRow(ctx context.Context, tx kvtx.Tx, rowKey []byte, gone map[string]bool, shared bool, others []string) error {
	// Choose the holder: a registered pack, else a held one.
	to, found := x.holderNum(gone, others)
	if found {
		return tx.Set(ctx, rowKey, hashValue{num: to}.marshal())
	}

	// With none, delete the row unless another pack shares the number.
	if shared {
		return nil
	}
	return tx.Delete(ctx, rowKey)
}

// holderNum returns the number of the first pack of others the table holds
// and gone lacks, preferring a registered pack to a held one.
func (x *packIndex) holderNum(gone map[string]bool, others []string) (uint32, bool) {
	// Return the first registered holder, keeping the first held one.
	x.mtx.RLock()
	defer x.mtx.RUnlock()
	var held uint32
	var anyHeld bool
	for _, id := range others {
		row, ok := x.rows[id]
		if !ok || gone[id] {
			continue
		}
		if !x.heldLocked(id) {
			return row.num, true
		}
		if !anyHeld {
			held, anyHeld = row.num, true
		}
	}
	return held, anyHeld
}
