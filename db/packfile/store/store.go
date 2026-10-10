package store

import (
	"context"
	"maps"
	"math"
	"slices"
	"sync"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/bloom"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/packfile"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/tidwall/btree"
	"golang.org/x/sync/errgroup"
)

// indexLoadConcurrency bounds the concurrent index loads of one batch probe.
const indexLoadConcurrency = 8

// Opener returns a per-pack access engine for a remote packfile of the
// given size.
//
// The size is taken from the manifest entry so the opener does not need to
// issue a separate metadata request. Implementations typically wrap a
// Transport via NewPackReader or NewHTTPRangeReader.
type Opener func(packID string, size int64) (*PackReader, error)

// IndexCache stores raw kvfile index-tail bytes per packfile.
//
// Implementations are expected to be durable (kvtx-backed) in production
// and ephemeral in tests. The engine parses and validates cached tail bytes
// into runtime-only index views before serving block data.
type IndexCache interface {
	// Get returns cached raw index-tail bytes for a packfile.
	Get(ctx context.Context, packID string) ([]byte, bool, error)
	// Set stores raw index-tail bytes for a packfile.
	Set(ctx context.Context, packID string, data []byte) error
}

// PackfileStore is a read-only block.StoreOps over a set of remote packfiles.
//
// The store fans reads out to per-pack engines: it handles manifest-wide
// concerns (bloom lookup, engine registry, writeback and index cache
// configuration, the shared resident budget) while the engines own per-pack
// spans, indexes, and writeback.
type PackfileStore struct {
	opener Opener
	cache  IndexCache

	// mtx guards store construction, configuration, and shutdown.
	mtx sync.Mutex
	// closed rejects operations after Close begins draining open readers.
	closed  bool
	engines map[string]*PackReader
	stats   packLookupStats
	notify  func()

	// bcast guards manifest, bloom, and close-completion state.
	bcast broadcast.Broadcast
	// closeComplete records that Close has released the store state.
	closeComplete bool

	// writebackCtx is the long-lived ctx used for async writebacks.
	// nil disables writeback.
	writebackCtx context.Context
	// writebackTarget receives verified cache copies when writeback is enabled.
	writebackTarget block.StoreOps
	// writebackWindow is the byte window for selecting neighbor blocks.
	writebackWindow int64
	// budget bounds the resident span bytes of every engine.
	budget *residentBudget

	// manifest orders immutable descriptors for lookup, guarded by bcast.
	manifest *btree.BTreeG[*manifestEntry]
	// manifestByID permits point replacement and removal, guarded by bcast.
	manifestByID map[string]*manifestEntry
}

// NewPackfileStore creates a new packfile store.
func NewPackfileStore(opener Opener, cache IndexCache) *PackfileStore {
	s := &PackfileStore{
		opener:          opener,
		cache:           cache,
		engines:         make(map[string]*PackReader),
		manifest:        newManifestTree(),
		manifestByID:    make(map[string]*manifestEntry),
		writebackCtx:    context.Background(),
		writebackWindow: defaultWritebackWindow,
		budget:          newResidentBudget(defaultResidentBudget),
	}
	return s
}

// Close cancels transport work and releases every open pack reader.
func (s *PackfileStore) Close() {
	// Fence new operations and detach the reader registry.
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		s.waitCloseComplete()
		return
	}
	s.closed = true
	engines := s.snapshotEnginesLocked()
	s.engines = nil
	s.mtx.Unlock()

	// Drain every reader outside the store mutex.
	for _, engine := range engines {
		if engine != nil {
			engine.Close()
		}
	}

	// Release store dependencies and publish close completion to waiting callers.
	s.mtx.Lock()
	s.opener = nil
	s.cache = nil
	s.writebackCtx = nil
	s.writebackTarget = nil
	s.notify = nil
	s.mtx.Unlock()
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.manifest = newManifestTree()
		s.manifestByID = nil
		s.closeComplete = true
		broadcast()
	})
}

func (s *PackfileStore) waitCloseComplete() {
	// Wait for the store to finish releasing its readers.
	for {
		// Read store close completion together with its next notification.
		var complete bool
		var waitCh <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			complete = s.closeComplete
			if !complete {
				waitCh = getWaitCh()
			}
		})

		// Finish once every store reader has drained.
		if complete {
			return
		}

		// Wait for the store to publish close completion.
		<-waitCh
	}
}

// SetWriteback enables co-block persistence to a target store.
//
// When a block is fetched from a remote packfile the engine also fetches
// every other block that fully fits within windowBytes of the target, and
// writes each fetched block to target in an asynchronous batch after
// verifying it. ctx scopes the background work. Pass nil target to disable
// persistence.
func (s *PackfileStore) SetWriteback(ctx context.Context, target block.StoreOps, windowBytes int64) {
	// Choose the default neighbor window for pack writeback.
	if windowBytes <= 0 {
		windowBytes = defaultWritebackWindow
	}

	// Save the writeback configuration and snapshot the open readers.
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		return
	}
	s.writebackCtx = ctx
	s.writebackTarget = target
	s.writebackWindow = windowBytes
	engines := s.snapshotEnginesLocked()
	s.mtx.Unlock()

	// Apply the writeback configuration to every open reader.
	for _, e := range engines {
		e.SetWriteback(ctx, target, windowBytes)
	}
}

// SetRangeCacheMaxBytes sets the resident-byte budget shared by every engine.
func (s *PackfileStore) SetRangeCacheMaxBytes(maxBytes int64) {
	s.budget.limit.Store(maxBytes)
	s.budget.reclaim()
}

// SetStatsChangedCallback sets a callback invoked after observable stats change.
func (s *PackfileStore) SetStatsChangedCallback(fn func()) {
	// Save the stats callback and snapshot the open readers.
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		return
	}
	s.notify = fn
	engines := s.snapshotEnginesLocked()
	s.mtx.Unlock()

	// Connect every open reader to the stats observer.
	for _, e := range engines {
		e.SetStatsChangedCallback(fn)
	}
}

// GetHashType returns the hash type for the store.
func (s *PackfileStore) GetHashType() hash.HashType {
	return hash.HashType_HashType_SHA256
}

// GetSupportedFeatures returns the native feature bitset.
func (s *PackfileStore) GetSupportedFeatures() block.StoreFeature {
	return 0
}

// BeginReadOperation returns the packfile store as the scoped read handle.
func (s *PackfileStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// GetBlock gets a block by reference from the packfile store.
func (s *PackfileStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	stored, err := s.GetStoredBlock(ctx, ref)
	return stored.GetData(), stored != nil, err
}

// GetStoredBlock gets a block with its refs from the packfile store.
//
// Each pack whose bloom filter may hold the block is consulted in manifest
// order. The first pack that finds the block returns it. Returns nil when no
// pack holds the block.
func (s *PackfileStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Trace the block lookup across candidate pack readers.
	ctx, task := trace.NewTask(ctx, "provider/spacewave/packfile/store/get-block")
	defer task.End()

	// Resolve the block reference to its pack index key.
	h := ref.GetHash()
	if h == nil {
		trace.Log(ctx, "result", "empty-hash")
		return nil, nil
	}
	if trace.IsEnabled() {
		trace.Log(ctx, "block-ref", ref.MarshalString())
	}
	key := packfile.BlockKey(h)

	// Read the block from the first candidate pack that contains it.
	var stored *block.StoredBlock
	var lookup packLookup
	err := s.probePacks(key, &lookup, func(eng *PackReader) (bool, error) {
		// Trace and read the block from this candidate pack.
		trace.Log(ctx, "pack-id", eng.packID)
		var err error
		stored, err = eng.getBlock(ctx, key, ref)
		return stored != nil, err
	})
	s.recordLookupStats(lookup)
	if err != nil {
		trace.Log(ctx, "result", "error")
		return nil, err
	}

	// Record whether the candidate packs supplied the requested block.
	trace.Logf(ctx, "candidate-packs", "%d", lookup.candidates)
	if stored == nil {
		trace.Log(ctx, "result", "miss")
		return nil, nil
	}
	trace.Log(ctx, "result", "hit")
	return stored, nil
}

// GetBlockExists reports whether a block exists in the store.
func (s *PackfileStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	// Resolve the block reference to its pack index key.
	h := ref.GetHash()
	if h == nil {
		return false, nil
	}
	key := packfile.BlockKey(h)

	// Probe candidate pack indexes and record the existence result.
	var lookup packLookup
	err := s.probePacks(key, &lookup, func(eng *PackReader) (bool, error) {
		return eng.getBlockExists(ctx, key)
	})
	s.recordLookupStats(lookup)
	return lookup.hit, err
}

// GetBlockExistsBatch checks whether each block exists. A manifest change
// that closes a probed reader restarts the batch on the new catalog.
func (s *PackfileStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	for {
		out, err := s.SnapshotManifest().GetBlockExistsBatch(ctx, refs)
		if !errors.Is(err, ErrPackReaderClosed) {
			return out, err
		}
	}
}

// getBlockExistsBatch shares one catalog snapshot across prefetch and all probes.
func (s *PackfileStore) getBlockExistsBatch(ctx context.Context, view *ManifestSnapshot, refs []*block.BlockRef) ([]bool, error) {
	// Group duplicate block references by their pack index key.
	out := make([]bool, len(refs))
	indexes := make(map[string][]int, len(refs))
	var keys []string
	for i, ref := range refs {
		// Ignore block references without a hash.
		h := ref.GetHash()
		if h == nil {
			continue
		}

		// Retain each unique index key and all matching result positions.
		key := string(packfile.BlockKey(h))
		if _, ok := indexes[key]; !ok {
			keys = append(keys, key)
		}
		indexes[key] = append(indexes[key], i)
	}

	// Return empty existence results when no reference has a hash.
	if len(keys) == 0 {
		return out, nil
	}

	// Load candidate pack indexes before probing the block keys.
	if err := s.loadCandidateIndexes(ctx, view, keys); err != nil {
		return nil, err
	}

	// Probe each unique key and account for the complete batch lookup.
	var lookup packLookup
	defer func() {
		s.recordLookupStats(lookup)
	}()
	for _, key := range keys {
		// Check whether a candidate pack contains this block key.
		var found bool
		err := s.probeCatalog(view, []byte(key), &lookup, func(eng *PackReader) (bool, error) {
			var err error
			found, err = eng.getBlockExists(ctx, []byte(key))
			return found, err
		})
		if err != nil {
			return nil, err
		}

		// Mark every duplicate reference when the block key exists.
		if found {
			for _, index := range indexes[key] {
				out[index] = true
			}
		}
	}
	return out, nil
}

// loadCandidateIndexes loads the index of every pack whose bloom filter may
// hold one of keys, indexLoadConcurrency at a time, so the probes that follow
// search resident indexes instead of loading them one after another.
func (s *PackfileStore) loadCandidateIndexes(ctx context.Context, view *ManifestSnapshot, keys []string) error {
	// Prepare bloom keys for the batch of block references.
	bloomKeys := make([]bloom.Key, len(keys))
	for i, key := range keys {
		bloomKeys[i] = bloom.NewKey([]byte(key))
	}

	// Select manifest packs whose bloom filters may contain the block keys.
	var candidates []*packfile.PackfileEntry
	view.entries.Scan(func(item *manifestEntry) bool {
		if item.filter == nil || slices.ContainsFunc(bloomKeys, item.filter.TestKey) {
			candidates = append(candidates, item.entry)
		}
		return true
	})
	if len(candidates) < 2 {
		return nil
	}

	// Load candidate pack indexes with bounded concurrency.
	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(indexLoadConcurrency)

	// Schedule an index load for each nonempty candidate pack.
	for _, entry := range candidates {
		// Validate the candidate pack size before opening its reader.
		size, err := manifestPackSize(entry)
		if err != nil {
			return err
		}
		if size <= 0 {
			continue
		}

		// Load the candidate index while holding its reader reference.
		eg.Go(func() error {
			// Acquire the candidate pack reader for the index load.
			eng, release, err := s.getOrOpenEngine(entry.GetId(), size, entry.GetBlockCount())
			if err != nil {
				return errors.Wrap(err, "opening packfile")
			}
			defer release()
			return eng.ensureIndexLoaded(ctx)
		})
	}
	return eg.Wait()
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (s *PackfileStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	// Resolve the block reference to its pack index key.
	h := ref.GetHash()
	if h == nil {
		return nil, nil
	}
	key := packfile.BlockKey(h)

	// Read block metadata from the first matching candidate pack.
	var stat *block.BlockStat
	var lookup packLookup
	err := s.probePacks(key, &lookup, func(eng *PackReader) (bool, error) {
		var err error
		stat, err = eng.statBlock(ctx, key, ref)
		return stat != nil, err
	})
	s.recordLookupStats(lookup)
	if err != nil {
		return nil, err
	}
	return stat, nil
}

// probePacks visits the engine of each pack whose bloom filter may hold key,
// in manifest order, until visit reports a hit. It accumulates the probe into
// lookup.
//
// A reader closes during a probe only when a manifest change removed its pack,
// such as a compaction merging it or a reclaim dropping it. The probe then
// restarts on the catalog that replaced it, which lists the merged pack or
// omits the dropped one. Store shutdown ends the restarts with
// ErrPackfileStoreClosed.
func (s *PackfileStore) probePacks(
	key []byte,
	lookup *packLookup,
	visit func(eng *PackReader) (bool, error),
) error {
	for {
		err := s.probeCatalog(s.SnapshotManifest(), key, lookup, visit)
		if !errors.Is(err, ErrPackReaderClosed) {
			return err
		}
	}
}

// probeCatalog visits a stable catalog without holding a store lock during I/O.
func (s *PackfileStore) probeCatalog(
	view *ManifestSnapshot,
	key []byte,
	lookup *packLookup,
	visit func(eng *PackReader) (bool, error),
) error {
	// Select catalog packs whose bloom filters may contain the block key.
	bloomKey := bloom.NewKey(key)
	var candidates []*packfile.PackfileEntry
	view.entries.Scan(func(item *manifestEntry) bool {
		if item.filter == nil || item.filter.TestKey(bloomKey) {
			candidates = append(candidates, item.entry)
		}
		return true
	})
	lookup.candidates += len(candidates)

	// Probe candidate pack readers until the requested block is found.
	for _, entry := range candidates {
		// Validate the candidate pack size before opening its reader.
		size, err := manifestPackSize(entry)
		if err != nil {
			return err
		}
		if size <= 0 {
			continue
		}

		// Borrow the candidate pack reader for this block probe.
		eng, release, err := s.getOrOpenEngine(entry.GetId(), size, entry.GetBlockCount())
		if err != nil {
			return errors.Wrap(err, "opening packfile")
		}

		// Probe the block and release the candidate reader before handling the result.
		lookup.opened++
		found, err := visit(eng)
		release()
		if err != nil {
			return err
		}

		// Record a successful block probe or count the negative pack.
		if found {
			lookup.hit = true
			return nil
		}
		lookup.negative++
	}
	return nil
}

// recordLookupStats adds one lookup to the store stats and notifies watchers.
func (s *PackfileStore) recordLookupStats(lookup packLookup) {
	// Accumulate the pack lookup totals under the store mutex.
	var notify func()
	s.mtx.Lock()
	s.stats.LookupCount++
	s.stats.CandidatePacks += uint64(lookup.candidates) //nolint:gosec // counts are non-negative loop lengths.
	s.stats.OpenedPacks += uint64(lookup.opened)        //nolint:gosec // counts are non-negative loop lengths.
	s.stats.NegativePacks += uint64(lookup.negative)    //nolint:gosec // counts are non-negative loop lengths.
	if lookup.hit {
		s.stats.TargetHits++
	}

	// Retain the latest lookup details and capture the stats observer.
	s.stats.LastCandidatePacks = lookup.candidates
	s.stats.LastOpenedPacks = lookup.opened
	s.stats.LastNegativePacks = lookup.negative
	s.stats.LastTargetHit = lookup.hit
	notify = s.notify
	s.mtx.Unlock()

	// Notify the stats observer after releasing the store mutex.
	if notify != nil {
		notify()
	}
}

// manifestPackSize validates the wire-sized pack length before it crosses
// into the int64-based range-reader API.
func manifestPackSize(entry *packfile.PackfileEntry) (int64, error) {
	size := entry.GetSizeBytes()
	if size > math.MaxInt64 {
		return 0, errors.Errorf("packfile %s size exceeds int64 range: %d", entry.GetId(), size)
	}
	return int64(size), nil //nolint:gosec // the MaxInt64 check above makes this conversion representable.
}

// PutBlock is not supported on a read-only store.
func (s *PackfileStore) PutBlock(_ context.Context, _ []byte, _ *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, block_store.ErrReadOnly
}

// PutBlockBatch is not supported on a read-only store.
func (s *PackfileStore) PutBlockBatch(_ context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	return nil, block_store.ErrReadOnly
}

// RmBlock is not supported on a read-only store.
func (s *PackfileStore) RmBlock(_ context.Context, _ *block.BlockRef) error {
	return block_store.ErrReadOnly
}

// Sync reports always-durable: the read-only packfile store holds no buffered
// writes.
func (s *PackfileStore) Sync(_ context.Context) (bool, error) {
	return true, nil
}

// UpdateManifest replaces a complete catalog, for startup and remote snapshots.
// Incremental changes use ApplyManifestDelta instead.
func (s *PackfileStore) UpdateManifest(entries []*packfile.PackfileEntry) {
	s.updateManifest(entries, nil, true)
}

// ApplyManifestDelta publishes accepted entries and removes only named packs.
func (s *PackfileStore) ApplyManifestDelta(entries []*packfile.PackfileEntry, removed []string) {
	s.updateManifest(entries, removed, false)
}

// updateManifest serializes publication and detaches affected readers atomically.
func (s *PackfileStore) updateManifest(entries []*packfile.PackfileEntry, removed []string, replace bool) {
	// Fence publication against shutdown and reader construction.
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		return
	}
	var evicted []*PackReader
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// A full snapshot alone computes the missing IDs.
		if replace {
			// Collect the active pack IDs in the replacement snapshot.
			present := make(map[string]bool, len(entries))
			for _, entry := range entries {
				present[entry.GetId()] = !entry.IsSuperseded()
			}

			// Remove catalog entries absent from the replacement snapshot.
			for id := range s.manifestByID {
				if !present[id] {
					removed = append(removed, id)
				}
			}

			// Remove cached readers absent from the replacement snapshot.
			for id := range s.engines {
				if !present[id] {
					removed = append(removed, id)
				}
			}
		}

		// Prepare catalog removal and reader retirement for a pack.
		remove := func(id string) {
			// Remove the pack descriptor from both catalog indexes.
			if old := s.manifestByID[id]; old != nil {
				s.manifest.Delete(old)
				delete(s.manifestByID, id)
			}

			// Detach the pack reader for draining outside the catalog locks.
			engine := s.engines[id]
			delete(s.engines, id)
			if engine != nil {
				evicted = append(evicted, engine)
			}
		}

		// Remove the packs explicitly retired by the manifest update.
		for _, id := range removed {
			remove(id)
		}

		// Publish the accepted pack descriptors into the catalog.
		for _, entry := range entries {
			// Ignore unnamed descriptors and remove superseded packs.
			id := entry.GetId()
			if id == "" {
				continue
			}
			if entry.IsSuperseded() {
				remove(id)
				continue
			}

			// Replace changed descriptors and retire readers with changed pack contents.
			old := s.manifestByID[id]
			if old != nil {
				// Retain an unchanged pack descriptor and its reader.
				if old.entry.EqualVT(entry) {
					continue
				}

				// Sequencing and bloom changes retain an immutable pack reader.
				if old.entry.GetSizeBytes() != entry.GetSizeBytes() || old.entry.GetBlockCount() != entry.GetBlockCount() {
					remove(id)
				}
				s.manifest.Delete(old)
			}

			// Index the accepted descriptor with its parsed bloom filter.
			item := parseManifestEntry(entry)
			s.manifestByID[id] = item
			s.manifest.Set(item)
		}

		// Wake catalog observers after publishing the manifest changes.
		broadcast()
	})
	s.mtx.Unlock()

	// Readers drain outside both catalog locks.
	for _, engine := range evicted {
		engine.Close()
	}
	s.notifyStatsChanged()
}

// notifyStatsChanged wakes observers after a committed store change.
func (s *PackfileStore) notifyStatsChanged() {
	// Capture the store stats observer under the mutex.
	s.mtx.Lock()
	notify := s.notify
	s.mtx.Unlock()

	// Notify the stats observer after releasing the store mutex.
	if notify != nil {
		notify()
	}
}

// getOrOpenEngine acquires a reader for the requested immutable pack metadata.
// Readers from an older snapshot are closed by the caller instead of being
// reinserted into the current catalog's cache after removal or replacement.
func (s *PackfileStore) getOrOpenEngine(packID string, size int64, blockCount uint64) (*PackReader, func(), error) {
	// Reject reader acquisition after store shutdown begins.
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		return nil, nil, ErrPackfileStoreClosed
	}

	// Check whether the requested pack metadata matches the current catalog.
	currentMatches := func() bool {
		entry := s.manifestByID[packID]
		return entry != nil && entry.entry.GetSizeBytes() == uint64(size) && entry.entry.GetBlockCount() == blockCount //nolint:gosec // callers skip non-positive sizes.
	}
	if eng := s.engines[packID]; eng != nil && currentMatches() {
		s.mtx.Unlock()
		return eng, func() {}, nil
	}

	// Snapshot the reader dependencies before opening outside the store mutex.
	opener, cache := s.opener, s.cache
	wbCtx, wbTarget, wbWindow := s.writebackCtx, s.writebackTarget, s.writebackWindow
	notify := s.notify
	s.mtx.Unlock()

	// Open outside the catalog lock, using the requested snapshot's metadata.
	eng, err := opener(packID, size)
	if err != nil {
		return nil, nil, err
	}
	eng.packID = packID
	eng.SetExpectedBlockCount(blockCount)
	eng.SetIndexCache(cache)
	eng.SetWriteback(wbCtx, wbTarget, wbWindow)
	eng.setBudget(s.budget)
	eng.SetStatsChangedCallback(notify)

	// Publish only into the matching current catalog; old snapshots borrow
	// a private reader and release it when their operation finishes.
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		eng.Close()
		return nil, nil, ErrPackfileStoreClosed
	}
	if !currentMatches() {
		s.mtx.Unlock()
		return eng, func() { eng.Close() }, nil
	}
	if existing := s.engines[packID]; existing != nil {
		s.mtx.Unlock()
		eng.Close()
		return existing, func() {}, nil
	}
	s.engines[packID] = eng
	s.mtx.Unlock()
	return eng, func() {}, nil
}

// snapshotEnginesLocked returns the open engines. Caller holds mtx.
func (s *PackfileStore) snapshotEnginesLocked() []*PackReader {
	return slices.Collect(maps.Values(s.engines))
}

// _ is a type assertion
var _ block.StoreOps = (*PackfileStore)(nil)
