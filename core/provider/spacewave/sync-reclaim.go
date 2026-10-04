package provider_spacewave

import (
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"golang.org/x/sync/errgroup"
)

const (
	// reclaimStateKey stores the ReclaimState of the last pass.
	reclaimStateKey = "sync/reclaim"
	// reclaimInterval is the longest time between passes, so a Space that
	// only deletes still reclaims.
	reclaimInterval = 7 * 24 * time.Hour
	// reclaimMinGrowth is the least catalog growth since the last pass that
	// starts a new one before reclaimInterval. The catalog must also grow by a
	// quarter.
	reclaimMinGrowth = 64 << 20
	// reclaimMaxPacks is how many packs one trash request may name per list.
	reclaimMaxPacks = 32
	// reclaimConcurrency bounds the concurrent block and pack index reads.
	reclaimConcurrency = 8
)

// reclaimPack is the liveness of the blocks of one catalog pack.
type reclaimPack struct {
	// entry is the catalog entry of the pack.
	entry *packfile.PackfileEntry
	// keys are the block keys the pack holds.
	keys []string
	// total is the value bytes of every block.
	total uint64
	// dead is the value bytes of the blocks no live root reaches.
	dead uint64
}

// ReclaimStorage runs a storage reclaim pass when it would pay for itself and
// returns the time the next pass comes due.
//
// The cloud cannot read the encrypted blocks, so the pass judges liveness
// here: it lists the catalog, places the fence, and walks every live root
// cache first. A missing block or unknown refs ends the pass with no change.
//
// A pack at least half dead moves to the trash, where it stays readable but
// no longer counts for upload dedup, so a writer that references one of its
// blocks again uploads the block. A later pass retires each trash pack older
// than packfile.TrashAge, after rescuing the blocks it holds that are live
// again and held by no other pack.
func (s *syncController) ReclaimStorage(ctx context.Context, fence bstore.ReclaimFence) (time.Time, error) {
	// A read-only replica of a public Space never writes the catalog.
	if s.skipPull {
		return time.Time{}, nil
	}

	// List the catalog and decide whether a pass is due.
	if err := s.PullNow(ctx); err != nil {
		return time.Time{}, err
	}
	state, err := s.readReclaimState(ctx)
	if err != nil {
		return time.Time{}, err
	}
	entries := s.reclaimEntries()
	now := time.Now()
	next, due := reclaimDue(state, entries, now)
	if !due {
		return next, nil
	}

	// Place the fence and walk the live roots.
	roots, err := fence(ctx, false)
	if err != nil {
		return time.Time{}, err
	}
	live, err := s.walkLive(ctx, roots)
	if err != nil {
		return time.Time{}, err
	}

	// Judge every listed pack, then retire the due trash and trash the
	// mostly dead packs.
	packs, err := s.judgePacks(ctx, entries, live)
	if err != nil {
		return time.Time{}, err
	}
	if err := s.sweepTrash(ctx, packs, live, now); err != nil {
		return time.Time{}, err
	}
	if err := s.markTrash(ctx, packs); err != nil {
		return time.Time{}, err
	}

	// Record the pass and report the next due time.
	state = &ReclaimState{CatalogBytes: catalogBytes(s.reclaimEntries()), PassedAtNanos: now.UnixNano()}
	if err := s.writeReclaimState(ctx, state); err != nil {
		return time.Time{}, err
	}
	next, _ = reclaimDue(state, s.reclaimEntries(), now)
	return next, nil
}

// reclaimEntries returns the committed catalog packs of the local manifest.
func (s *syncController) reclaimEntries() []*packfile.PackfileEntry {
	return slices.DeleteFunc(s.mfst.GetEntries(), func(entry *packfile.PackfileEntry) bool {
		return entry.GetSequence() == 0 || entry.IsSuperseded()
	})
}

// reclaimDue reports whether a pass is due now and when the next one comes
// due without further writes. A pass is due when trash is old enough to
// retire, when the catalog grew enough since the last pass, or when
// reclaimInterval passed.
func reclaimDue(state *ReclaimState, entries []*packfile.PackfileEntry, now time.Time) (time.Time, bool) {
	// Find the earliest time a pass is due.
	next := time.Unix(0, state.GetPassedAtNanos()).Add(reclaimInterval)
	if state.GetPassedAtNanos() == 0 {
		next = now
	}
	for _, entry := range entries {
		if entry.IsTrash() {
			if at := entry.GetTrashedAt().AsTime().Add(packfile.TrashAge); at.Before(next) {
				next = at
			}
		}
	}

	// Run early when the catalog grew enough.
	size, last := catalogBytes(entries), state.GetCatalogBytes()
	if size >= last+reclaimMinGrowth && size >= last+last/4 {
		next = now
	}
	return next, !now.Before(next)
}

// catalogBytes returns the stored bytes of entries.
func catalogBytes(entries []*packfile.PackfileEntry) uint64 {
	var size uint64
	for _, entry := range entries {
		size += entry.GetSizeBytes()
	}
	return size
}

// readReclaimState reads the record of the last pass, or an empty one.
func (s *syncController) readReclaimState(ctx context.Context) (*ReclaimState, error) {
	state := &ReclaimState{}
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) { return s.store.NewTransaction(ctx, false) },
		func(ctx context.Context, tx kvtx.Tx) error {
			data, found, err := tx.Get(ctx, []byte(reclaimStateKey))
			if err != nil || !found {
				return err
			}
			return state.UnmarshalVT(data)
		},
	)
	return state, errors.Wrap(err, "read reclaim state")
}

// writeReclaimState records the last pass.
func (s *syncController) writeReclaimState(ctx context.Context, state *ReclaimState) error {
	data, err := state.MarshalVT()
	if err != nil {
		return err
	}
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) { return s.store.NewTransaction(ctx, true) },
		func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, []byte(reclaimStateKey), data)
		},
	)
	return errors.Wrap(err, "write reclaim state")
}

// walkLive returns the block keys of every block under roots. It reads each
// block from the local cache first and falls back to the cloud.
func (s *syncController) walkLive(ctx context.Context, roots []*block.BlockRef) (map[string]struct{}, error) {
	live := make(map[string]struct{})
	src := &reclaimReadStore{StoreOps: s.upper, cloud: s.lower}
	err := block.WalkGraph(ctx, src, roots, reclaimConcurrency, func(ref *block.BlockRef, _ *block.StoredBlock) error {
		live[string(packfile.BlockKey(ref.GetHash()))] = struct{}{}
		return nil
	})
	return live, errors.Wrap(err, "walk live blocks")
}

// reclaimReadStore reads a block from the local cache and falls back to the
// cloud catalog.
type reclaimReadStore struct {
	block.StoreOps
	cloud block.StoreOps
}

// GetStoredBlock reads ref from the cache, then from the cloud.
func (r *reclaimReadStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := r.StoreOps.GetStoredBlock(ctx, ref)
	if stored != nil || err != nil && !errors.Is(err, block.ErrNotFound) {
		return stored, err
	}
	return r.cloud.GetStoredBlock(ctx, ref)
}

// judgePacks reads the key index of each entry and sums its live and dead
// value bytes against live.
func (s *syncController) judgePacks(ctx context.Context, entries []*packfile.PackfileEntry, live map[string]struct{}) ([]*reclaimPack, error) {
	// Read the indexes concurrently, each into its own slot.
	packs := make([]*reclaimPack, len(entries))
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(reclaimConcurrency)
	for i, entry := range entries {
		eg.Go(func() error {
			// Read the pack's key index.
			index, err := s.lower.ReadPackIndex(egCtx, entry.GetId(), int64(entry.GetSizeBytes())) //nolint:gosec // pack sizes are bounded by writer.DefaultMaxPackBytes.
			if err != nil {
				return err
			}

			// Sum the value bytes of every block and of the dead ones.
			pack := &reclaimPack{entry: entry, keys: make([]string, len(index))}
			for j, ie := range index {
				pack.keys[j] = string(ie.GetKey())
				pack.total += ie.GetSize()
				if _, ok := live[pack.keys[j]]; !ok {
					pack.dead += ie.GetSize()
				}
			}
			packs[i] = pack
			return nil
		})
	}
	return packs, errors.Wrap(eg.Wait(), "read pack indexes")
}

// sweepTrash retires the trash packs older than packfile.TrashAge. Before it
// retires them, it uploads the live blocks they hold that no current pack
// holds, as an ordinary flush would.
func (s *syncController) sweepTrash(ctx context.Context, packs []*reclaimPack, live map[string]struct{}, now time.Time) error {
	// Split the due trash from the packs that keep their blocks.
	held := make(map[string]struct{})
	var due []*reclaimPack
	for _, pack := range packs {
		switch {
		case !pack.entry.IsTrash():
			for _, key := range pack.keys {
				held[key] = struct{}{}
			}
		case !now.Before(pack.entry.GetTrashedAt().AsTime().Add(packfile.TrashAge)):
			due = append(due, pack)
		}
	}
	if len(due) == 0 {
		return nil
	}

	// Rescue and retire each group under the flush lock, so no merge or flush
	// pushes concurrently.
	s.flushMtx.Lock()
	defer s.flushMtx.Unlock()
	for group := range slices.Chunk(due, reclaimMaxPacks) {
		if err := s.rescueBlocks(ctx, group, live, held); err != nil {
			return err
		}
		retire := make([]string, len(group))
		for i, pack := range group {
			retire[i] = pack.entry.GetId()
		}
		if err := s.syncTrash(ctx, &packfile.TrashRequest{RetirePackIds: retire}); err != nil {
			return err
		}
	}
	return nil
}

// rescueBlocks uploads the live blocks of group that held lacks and adds them
// to held.
func (s *syncController) rescueBlocks(ctx context.Context, group []*reclaimPack, live, held map[string]struct{}) error {
	// Read the rescued blocks from each trash pack that has one.
	var rescued []packfile_store.PackBlock
	for _, pack := range group {
		if !slices.ContainsFunc(pack.keys, func(key string) bool { return isRescued(key, live, held) }) {
			continue
		}
		blocks, err := s.lower.ReadPackBlocks(ctx, pack.entry.GetId(), int64(pack.entry.GetSizeBytes())) //nolint:gosec // pack sizes are bounded by writer.DefaultMaxPackBytes.
		if err != nil {
			return errors.Wrapf(err, "read trash pack %s", pack.entry.GetId())
		}
		for _, b := range blocks {
			key := string(packfile.BlockKey(b.Hash))
			if isRescued(key, live, held) {
				held[key] = struct{}{}
				rescued = append(rescued, b)
			}
		}
	}

	// Push them in packs within the sync pack target and commit each.
	for len(rescued) != 0 {
		n, size := 0, int64(0)
		for n < len(rescued) && (n == 0 || size+int64(len(rescued[n].Block.GetData())) <= syncFlushMaxPackBytes) {
			size += int64(len(rescued[n].Block.GetData()))
			n++
		}
		chunk, err := s.preparePackBlocks(rescued[:n], nil)
		if err != nil {
			return err
		}
		if err := s.pushPreparedChunk(ctx, chunk); err != nil {
			return err
		}
		if err := s.applyManifestDelta(ctx, []*packfile.PackfileEntry{chunk.entry}, nil, 0); err != nil {
			return errors.Wrap(err, "applying rescue delta")
		}
		rescued = rescued[n:]
	}
	return nil
}

// isRescued reports that key is live and no current pack holds it.
func isRescued(key string, live, held map[string]struct{}) bool {
	_, isLive := live[key]
	_, isHeld := held[key]
	return isLive && !isHeld
}

// markTrash moves the current packs that are at least half dead to the trash.
func (s *syncController) markTrash(ctx context.Context, packs []*reclaimPack) error {
	// Collect the mostly dead current packs.
	var trash []string
	for _, pack := range packs {
		if !pack.entry.IsTrash() && pack.dead != 0 && pack.dead*2 >= pack.total {
			trash = append(trash, pack.entry.GetId())
		}
	}

	// Trash them in bounded requests.
	for group := range slices.Chunk(trash, reclaimMaxPacks) {
		if err := s.syncTrash(ctx, &packfile.TrashRequest{TrashPackIds: group}); err != nil {
			return err
		}
	}
	return nil
}

// syncTrash sends one trash request and pulls its result. A conflict means
// another writer changed the packs first, so the pull alone settles it.
func (s *syncController) syncTrash(ctx context.Context, req *packfile.TrashRequest) error {
	err := s.client.SyncTrash(ctx, s.resourceID, req)
	if err != nil && !clouderror.IsPackReplacementConflict(err) {
		return err
	}
	if err != nil {
		s.le.WithError(err).Debug("trash request lost a race, pulling")
	}
	return s.PullNow(ctx)
}
