package block_store_writeback

import (
	"context"
	"slices"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

// A bounded pass, EvictBounded, drops a local value only after it fetched the
// value's remote pack in the same batch and verified every value the pack
// holds, and only within a byte budget persisted per budget id. EvictBudget.tla
// models the decision; its invariants name the rules the code keeps:
//
//   - DropVerified: a value leaves only on this batch's fetch of its pack, a
//     fetch that found every value of the pack hashing to its key.
//   - BudgetDurable: the bytes a batch may drop are subtracted from the
//     persisted budget in a commit before any tombstone; the queue commit
//     after the tombstones returns what it did not drop. A crash between the
//     two loses the reservation, never refills it.
//   - NoBudgetNoDrop: a batch reads the budget at its start; with no valid
//     budget the pass ends before any fetch.
//   - SameIdNoRefill: a seen id's bytes left are min(left, the budget's
//     bytes) at each batch, so rewriting the budget with the same id never
//     raises them; only a new id grants new bytes.
//
// Two rules sit outside the model. A dropped value's only copy is its pack,
// so the pass drops nothing while EvictBound.RemoteDeletes names a delete
// path that could remove a verified pack. And a value of the fetched pack
// with no entry in the batch drops ahead only when the local index places it
// in that pack, since a read follows the index to the pack it names.
//
// A value drops ahead only with a queue entry of its own at an epoch of the
// batch's entries for that pack, and the queue commit deletes that entry and
// subtracts its recorded size, so evictable-bytes stays the summed size of
// the queue's entries. A value with no such entry, such as one held from
// before the queue or between an enable and its backfill, is not dropped:
// the pass keeps a value it cannot account for, and the walk that meets its
// entry, if it has one, fetches the pack again.

// evictBudgetPrefix prefixes the bytes left of each seen budget id in the
// meta store: evict-budget/<id> = decimal bytes.
const evictBudgetPrefix = "evict-budget/"

// ErrEvictUnbounded is returned by EvictBounded without a budget source, a
// verifier or a remote delete check.
var ErrEvictUnbounded = errors.New("bounded eviction needs a budget, a verifier and a remote delete check")

// EvictVerifier fetches and verifies the remote's packs for a bounded pass.
type EvictVerifier interface {
	// LocatePacks names the pack the local index places each block in. The
	// answer only groups the blocks; it proves nothing.
	LocatePacks(ctx context.Context, hashes []*hash.Hash) ([]EvictPackLocation, error)
	// VerifyPack fetches pack id of the recorded size from the remote
	// itself, never from a local copy, parses the pack's index from the
	// fetched bytes and hashes every value against its key. It returns every
	// value, or a non-empty reason when any check fails, one of
	// EvictRefusalLabels; err is for a failure that ends the pass, such as a
	// canceled context.
	VerifyPack(ctx context.Context, id string, size int64) (values []EvictVerifiedValue, reason string, err error)
}

// EvictBound bounds an eviction pass.
type EvictBound struct {
	// Budget returns the budget in force, read live at every batch; false,
	// or a budget that is not valid, means no budget, and nothing drops.
	Budget func() (EvictBudget, bool)
	// Verifier fetches and verifies the remote's packs.
	Verifier EvictVerifier
	// RemoteDeletes names a delete path enabled in the process that could
	// remove a verified pack from the remote, such as compaction or reclaim,
	// or returns "" when none is. It is read at the start of the pass and
	// before each pack; while it names one, nothing drops.
	RemoteDeletes func() string
}

// readInt reads a decimal key, returning found false when it is absent.
func readInt(ctx context.Context, tx kvtx.Tx, key []byte) (int64, bool, error) {
	value, found, err := tx.Get(ctx, key)
	if err != nil || !found {
		return 0, false, err
	}
	n, err := strconv.ParseInt(string(value), 10, 64)
	return n, err == nil, err
}

// setInt writes a decimal key.
func setInt(ctx context.Context, tx kvtx.Tx, key []byte, n int64) error {
	return tx.Set(ctx, key, []byte(strconv.FormatInt(n, 10)))
}

// EvictBounded runs one bounded eviction pass, one at a time with Evict's: it
// advances the epoch, then walks the queue oldest epoch first and drops local
// values until it has dropped maxBytes, reaches the new epoch, or the budget
// is spent or gone. keep and admit are as Evict's.
//
// Each batch groups its entries by the pack the local index places them in
// and, pack by pack: reserves min(bytes left, the pack's recorded size) of
// the budget in a commit; fetches the pack whole from the remote and verifies
// every value, as EvictVerifier.VerifyPack describes; tombstones the pack's
// values that are held locally, have no upload marker and are not kept, up
// to the reservation, including values whose queue entries lie outside the
// batch when the local index places them in the pack and an entry of theirs
// sits at an epoch of the batch's entries for it, in writes of at most the
// adaptive batch limit; then, in one commit, deletes the batch's dropped
// entries and those of the values dropped ahead, moves the entries
// it did not drop to the new epoch, corrects evictable-bytes and returns the
// unspent reservation. A pack that fails verification drops nothing; its entries
// move to the new epoch and the pack is skipped for the rest of the pass, so
// a bad oldest pack cannot hold the walk. An entry whose value is already
// gone is deleted, without charging the budget or counting as evicted. While
// bound.RemoteDeletes names a delete path, the pass drops nothing and sets the
// result's Halted.
func (s *Store) EvictBounded(
	ctx context.Context,
	bound EvictBound,
	maxBytes int64,
	keep func(*hash.Hash) bool,
	admit EvictAdmit,
) (EvictResult, error) {
	// Refuse a pass without a budget source or a verifier, and commit
	// ordered in the background.
	var result EvictResult
	if bound.Budget == nil || bound.Verifier == nil || bound.RemoteDeletes == nil {
		return result, ErrEvictUnbounded
	}
	ctx = kvtx.WithBackgroundWriteContext(kvtx.WithOrderedCommitContext(ctx))

	// Serialize the pass with other passes.
	release, err := s.evictMtx.Lock(ctx)
	if err != nil {
		return result, err
	}
	defer release()

	// End before any write while a remote delete could remove a verified
	// pack.
	if path := bound.RemoteDeletes(); path != "" {
		result.Halted = path
		return result, nil
	}

	// End before any write without a budget.
	if budget, ok := bound.Budget(); !ok || !budget.valid() {
		return result, nil
	}

	// Advance the epoch, so entries this pass queues sort after the walk.
	epoch, err := s.advanceEvictEpoch(ctx)
	if err != nil {
		return result, err
	}

	// Evict in batches until the byte target, the new epoch or the budget.
	pass := &boundPass{s: s, bound: bound, epoch: epoch, maxBytes: maxBytes, keep: keep, skip: make(map[string]string), result: &result}
	var last int64
	for result.EvictedBytes < maxBytes {
		if admit != nil {
			if err := admit(ctx, result.EvictedBytes-last); err != nil {
				return result, err
			}
		}
		last = result.EvictedBytes
		next, done, err := pass.batch(ctx, s.evictFrom)
		if err != nil {
			return result, err
		}
		s.evictFrom = next
		if done {
			return result, nil
		}
	}
	return result, nil
}

// boundPass is the state of one bounded pass.
type boundPass struct {
	s        *Store
	bound    EvictBound
	epoch    uint64
	maxBytes int64
	keep     func(*hash.Hash) bool
	// skip maps each pack that failed verification in this pass to its
	// reason.
	skip   map[string]string
	result *EvictResult
}

// entryFate is what a bounded batch does with a queue entry.
type entryFate int

const (
	// fateLeave leaves the entry for a later batch.
	fateLeave entryFate = iota
	// fateGone deletes the entry of a value already gone.
	fateGone
	// fateDrop deletes the entry of a value the batch dropped.
	fateDrop
	// fateRequeue moves the entry to the pass epoch.
	fateRequeue
)

// batch runs one bounded batch from the key from. Returns the key the next
// batch starts from and true when the pass ends: the walk reached the pass
// epoch, or the budget is spent or gone.
func (p *boundPass) batch(ctx context.Context, from []byte) ([]byte, bool, error) {
	// Scan the batch; an empty one ends the pass.
	s := p.s
	scan, err := s.scanEvictBatch(ctx, p.epoch, p.maxBytes, p.result.EvictedBytes, from)
	if err != nil {
		return nil, false, err
	}
	batch := scan.batch
	if len(batch) == 0 {
		return scan.next, true, nil
	}

	// Ask the local values which blocks they hold, and the local index which
	// pack each is placed in.
	refs := make([]*block.BlockRef, len(batch))
	hashes := make([]*hash.Hash, len(batch))
	for i, entry := range batch {
		refs[i], hashes[i] = block.NewBlockRef(entry.hash), entry.hash
	}
	held, err := s.values.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return nil, false, errors.Wrap(err, "check local values")
	}
	locs, err := p.bound.Verifier.LocatePacks(ctx, hashes)
	if err != nil {
		return nil, false, errors.Wrap(err, "locate remote packs")
	}

	// Settle the entries no fetch decides, and group the rest by pack in
	// the order the walk met them.
	fates := make([]entryFate, len(batch))
	refused := make(map[string]int64)
	var order []string
	groups := make(map[string][]int)
	for i := range batch {
		id, skipped := locs[i].ID, ""
		if id != "" {
			skipped = p.skip[id]
		}
		switch {
		case !held[i]:
			fates[i] = fateGone
		case scan.marked[i]:
			fates[i] = fateRequeue
			refused[EvictRefusedMarked]++
		case id == "":
			fates[i] = fateRequeue
			refused[EvictRefusedUnlocated]++
		case skipped != "":
			fates[i] = fateRequeue
			refused[skipped]++
		default:
			if groups[id] == nil {
				order = append(order, id)
			}
			groups[id] = append(groups[id], i)
		}
	}
	p.result.Refused += countRefused(refused)
	if err := p.commit(ctx, batch, fates, nil, "", 0, 0); err != nil {
		return nil, false, err
	}
	s.addEvictBoundStatus(0, false, 0, refused, 0)

	// Drop pack by pack, ending at the byte target or the budget.
	for _, id := range order {
		if p.result.EvictedBytes >= p.maxBytes {
			return from, false, nil
		}
		group := groups[id]
		stop, err := p.pack(ctx, batch, group, id, locs[group[0]].Size)
		if err != nil || stop {
			return from, stop, err
		}
	}
	return scan.next, scan.done, nil
}

// countRefused sums refusal counts.
func countRefused(refused map[string]int64) int {
	var n int64
	for _, v := range refused {
		n += v
	}
	return int(n)
}

// pack drops what it may of pack id, of recorded size, for the batch entries
// in group, as EvictBounded describes. Returns true when the budget is spent
// or gone, which ends the pass.
func (p *boundPass) pack(ctx context.Context, batch []evictEntry, group []int, id string, size int64) (bool, error) {
	// Read the budget in force; without one nothing drops.
	s := p.s
	budget, ok := p.bound.Budget()
	if !ok || !budget.valid() {
		return true, nil
	}

	// Drop nothing once a remote delete could remove a verified pack.
	if path := p.bound.RemoteDeletes(); path != "" {
		p.result.Halted = path
		return true, nil
	}

	// Reserve up to the pack's recorded size of the bytes left, before any
	// tombstone.
	budgetKey := []byte(evictBudgetPrefix + budget.ID)
	var resv, left int64
	err := retryBusy(ctx, func() error {
		return kvtx.RunOptimisticTransaction(ctx, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
			// A new id starts at its bytes; a seen id never rises past them.
			cur, found, err := readInt(ctx, tx, budgetKey)
			if err != nil {
				return err
			}
			if !found {
				cur = budget.Bytes
			}
			cur = max(min(cur, budget.Bytes), 0)
			resv = min(cur, size)
			left = cur - resv
			return setInt(ctx, tx, budgetKey, left)
		}, kvtx.RetryInvalidSnapshot)
	})
	if err != nil {
		return false, errors.Wrap(err, "reserve eviction budget")
	}
	s.addEvictBoundStatus(left, true, 0, nil, 0)
	if resv == 0 {
		return true, nil
	}

	// Fetch and verify the pack whole. A failed pack drops nothing, and is
	// skipped for the rest of the pass.
	fates := make([]entryFate, len(batch))
	values, reason, err := p.bound.Verifier.VerifyPack(ctx, id, size)
	if err != nil {
		return false, errors.Wrap(err, "verify remote pack")
	}
	if reason != "" {
		label := refusalLabel(reason)
		p.skip[id] = label
		for _, i := range group {
			fates[i] = fateRequeue
		}
		p.result.Refused += len(group)
		if err := p.commit(ctx, batch, fates, nil, budget.ID, resv, 0); err != nil {
			return false, err
		}
		s.addEvictBoundStatus(left+resv, true, 0, map[string]int64{label: int64(len(group))}, 0)
		return false, nil
	}

	// Size the verified values.
	var verified int64
	sizes := make(map[string]int64, len(values))
	for _, v := range values {
		sizes[v.Hash.MarshalString()] = v.Size
		verified += v.Size
	}

	// Take as candidates the group's entries the pack holds.
	refused := make(map[string]int64)
	type candidate struct {
		hash  *hash.Hash
		size  int64
		entry int
		// ahead is the queue key of a value outside the batch, nil for a
		// batch entry.
		ahead []byte
	}
	var cands []candidate
	for _, i := range group {
		vs, ok := sizes[batch[i].hash.MarshalString()]
		if !ok {
			fates[i] = fateRequeue
			refused[EvictRefusedAbsent]++
			continue
		}
		cands = append(cands, candidate{hash: batch[i].hash, size: vs, entry: i})
	}

	// Name the rest of the pack's values: those with no entry in the batch.
	inBatch := make(map[string]bool, len(batch))
	for _, entry := range batch {
		inBatch[entry.hash.MarshalString()] = true
	}
	var rest []EvictVerifiedValue
	for _, v := range values {
		if !inBatch[v.Hash.MarshalString()] {
			rest = append(rest, v)
		}
	}

	// Add those the local index places in this pack and that have an entry
	// of their own at an epoch of the group's.
	if len(rest) != 0 {
		// A value also held in another pack reads through the one its index
		// row names, which this fetch did not prove.
		rhashes := make([]*hash.Hash, len(rest))
		for i, v := range rest {
			rhashes[i] = v.Hash
		}
		rlocs, err := p.bound.Verifier.LocatePacks(ctx, rhashes)
		if err != nil {
			return false, errors.Wrap(err, "locate remote packs")
		}
		var located []EvictVerifiedValue
		for i, v := range rest {
			if rlocs[i].ID == id {
				located = append(located, v)
			}
		}

		// A value without an entry the commit can delete stays.
		keys, err := s.aheadEntries(ctx, batch, group, located)
		if err != nil {
			return false, err
		}
		for i, v := range located {
			if keys[i] != nil {
				cands = append(cands, candidate{hash: v.Hash, size: v.Size, entry: -1, ahead: keys[i]})
			}
		}
	}

	// Read which candidates are held now.
	crefs := make([]*block.BlockRef, len(cands))
	for i, c := range cands {
		crefs[i] = block.NewBlockRef(c.hash)
	}
	held, err := s.values.GetBlockExistsBatch(ctx, crefs)
	if err != nil {
		return false, errors.Wrap(err, "check local values")
	}

	// Read the candidates' upload markers.
	chashes := make([]*hash.Hash, len(cands))
	for i, c := range cands {
		chashes[i] = c.hash
	}
	marked, err := s.markedBatch(ctx, chashes)
	if err != nil {
		return false, err
	}

	// Take those held, with no marker, not kept, up to the reservation.
	var drop []*block.PutBatchEntry
	var ahead [][]byte
	var spent int64
	for i, c := range cands {
		switch {
		case !held[i]:
			if c.entry >= 0 {
				fates[c.entry] = fateGone
			}
		case marked[i]:
			if c.entry >= 0 {
				fates[c.entry] = fateRequeue
				refused[EvictRefusedMarked]++
			}
		case p.keep != nil && p.keep(c.hash):
			if c.entry >= 0 {
				fates[c.entry] = fateRequeue
				p.result.Requeued++
			}
		case spent+c.size > resv:
			if c.entry >= 0 {
				fates[c.entry] = fateRequeue
			}
		default:
			drop = append(drop, &block.PutBatchEntry{Ref: crefs[i], Tombstone: true})
			spent += c.size
			if c.entry >= 0 {
				fates[c.entry] = fateDrop
			} else {
				ahead = append(ahead, c.ahead)
			}
		}
	}
	p.result.Refused += countRefused(refused)

	// Tombstone the values in writes of at most the adaptive limit, each
	// timed for the next, so no background write holds a whole pack.
	if len(drop) != 0 {
		for rest := drop; len(rest) != 0; {
			bounds := loadEvictBatch()
			limit := bounds.clamp(s.evictLimit)
			slice := rest[:min(limit, len(rest))]
			var took time.Duration
			err := retryBusy(ctx, func() error {
				start := time.Now()
				_, err := s.values.PutBlockBatch(ctx, slice)
				took = time.Since(start)
				return err
			})
			if err != nil {
				return false, errors.Wrap(err, "evict blocks")
			}
			s.evictLimit = nextEvictLimit(bounds, limit, len(slice), took)
			rest = rest[len(slice):]
		}
		p.result.Evicted += len(drop)
		p.result.EvictedBytes += spent
	}

	// Commit the queue changes with the refund of the unspent reservation.
	if err := p.commit(ctx, batch, fates, ahead, budget.ID, resv-spent, spent); err != nil {
		return false, err
	}
	s.addEvictBoundStatus(left+resv-spent, true, verified, refused, 0)
	return left+resv-spent == 0, nil
}

// markedBatch reports for each hash whether a marker awaits its upload.
func (s *Store) markedBatch(ctx context.Context, hashes []*hash.Hash) ([]bool, error) {
	marked := make([]bool, len(hashes))
	err := retryBusy(ctx, func() error {
		return kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
			for i, h := range hashes {
				found, err := tx.Exists(ctx, markedKey(h))
				if err != nil {
					return err
				}
				marked[i] = found
			}
			return nil
		})
	})
	if err != nil {
		return nil, errors.Wrap(err, "read upload markers")
	}
	return marked, nil
}

// aheadEntries returns for each value the key of its queue entry at an epoch
// of the group's batch entries, or nil when it has none there. One point read
// per value and distinct epoch, a single epoch for a pack uploaded in one
// commit, in one read transaction. The pass holds evictMtx, and only a pass
// or a disable, which takes evictMtx, deletes an entry, so a key found here
// stands until the pass commits.
func (s *Store) aheadEntries(ctx context.Context, batch []evictEntry, group []int, values []EvictVerifiedValue) ([][]byte, error) {
	// Name the epochs the group's entries sit at.
	var epochs []uint64
	for _, i := range group {
		if !slices.Contains(epochs, batch[i].epoch) {
			epochs = append(epochs, batch[i].epoch)
		}
	}

	// Find each value's entry at one of them.
	keys := make([][]byte, len(values))
	err := retryBusy(ctx, func() error {
		return kvtx.RunTransaction(ctx, false, s.readTx, func(ctx context.Context, tx kvtx.Tx) error {
			for i, v := range values {
				keys[i] = nil
				for _, epoch := range epochs {
					key := evictKey(epoch, v.Hash)
					found, err := tx.Exists(ctx, key)
					if err != nil {
						return err
					}
					if found {
						keys[i] = key
						break
					}
				}
			}
			return nil
		})
	})
	if err != nil {
		return nil, errors.Wrap(err, "read eviction queue entries")
	}
	return keys, nil
}

// commit applies fates to the batch's entries, deletes the entries ahead of
// the values dropped ahead of the walk, and returns refund bytes to budget
// id, in one metadata commit, then projects the evictable and evicted bytes.
// Each deleted entry subtracts its recorded size, so evictable-bytes stays
// the summed size of the queue's entries.
func (p *boundPass) commit(ctx context.Context, batch []evictEntry, fates []entryFate, ahead [][]byte, id string, refund, evicted int64) error {
	// Skip a commit that changes nothing.
	s := p.s
	changes := len(ahead) != 0 || refund != 0
	for _, f := range fates {
		changes = changes || f != fateLeave
	}
	if !changes {
		return nil
	}

	// Commit the changes, recounting on each attempt.
	var delta, droppedAhead int64
	err := retryBusy(ctx, func() error {
		return kvtx.RunOptimisticTransaction(ctx, s.writeTx, func(ctx context.Context, tx kvtx.Tx) error {
			// Settle each walked entry.
			delta, droppedAhead = 0, 0
			for i, entry := range batch {
				if fates[i] == fateLeave {
					continue
				}
				if fates[i] == fateRequeue {
					added, err := queueEvict(ctx, tx, p.epoch, entry.hash, entry.size)
					if err != nil {
						return err
					}
					delta += added
				}
				if err := tx.Delete(ctx, entry.key); err != nil {
					return err
				}
				delta -= entry.size
			}

			// Delete the entries of the values dropped ahead of the walk.
			for _, key := range ahead {
				size, found, err := readInt(ctx, tx, key)
				if err != nil {
					return err
				}
				if !found {
					continue
				}
				if err := tx.Delete(ctx, key); err != nil {
					return err
				}
				droppedAhead++
				delta -= size
			}

			// Return the unspent reservation.
			if id != "" && refund != 0 {
				key := []byte(evictBudgetPrefix + id)
				cur, _, err := readInt(ctx, tx, key)
				if err != nil {
					return err
				}
				if err := setInt(ctx, tx, key, cur+refund); err != nil {
					return err
				}
			}
			return addEvictableBytes(ctx, tx, delta)
		}, kvtx.RetryInvalidSnapshot)
	})
	if err != nil {
		return errors.Wrap(err, "update eviction queue")
	}
	s.addEvictStatus(delta, evicted)
	if droppedAhead != 0 {
		s.addEvictBoundStatus(0, false, 0, nil, droppedAhead)
	}
	return nil
}
