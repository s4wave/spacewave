package store

import (
	"cmp"
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
)

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
// concerns (bloom pruning, engine registry, write-back/index cache
// configuration) while the engines own per-pack spans, block catalogs, and
// publication.
type PackfileStore struct {
	opener      Opener
	cache       IndexCache
	verifyQueue verifyExecutor

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
	// tuningOverrides are explicit per-engine tuning overrides.
	tuningOverrides engineTuningOverrides

	// manifest is the active manifest in lookup order, guarded by bcast.
	manifest []*packfile.PackfileEntry
	// filters holds the parsed bloom filter of each manifest entry at the same
	// index, guarded by bcast. A nil filter matches every key.
	filters []*bloom.Filter
}

// NewPackfileStore creates a new packfile store.
func NewPackfileStore(opener Opener, cache IndexCache) *PackfileStore {
	s := &PackfileStore{
		opener:          opener,
		cache:           cache,
		verifyQueue:     newDefaultVerifyExecutor(defaultVerifyConcurrency()),
		engines:         make(map[string]*PackReader),
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
	s.verifyQueue = nil
	s.writebackCtx = nil
	s.writebackTarget = nil
	s.notify = nil
	s.mtx.Unlock()
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.manifest = nil
		s.filters = nil
		s.closeComplete = true
		broadcast()
	})
}

func (s *PackfileStore) waitCloseComplete() {
	for {
		var complete bool
		var waitCh <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			complete = s.closeComplete
			if !complete {
				waitCh = getWaitCh()
			}
		})
		if complete {
			return
		}
		<-waitCh
	}
}

// SetWriteback enables co-block persistence to a target store.
//
// When a block is fetched from a remote packfile the engine also verifies
// every other block that fully fits within windowBytes of the target and
// writes those neighbors to target asynchronously. ctx scopes the
// background work. Pass nil target to disable persistence while keeping
// verification.
func (s *PackfileStore) SetWriteback(ctx context.Context, target block.StoreOps, windowBytes int64) {
	if windowBytes <= 0 {
		windowBytes = defaultWritebackWindow
	}
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
	for _, e := range engines {
		e.SetWriteback(ctx, target, windowBytes)
	}
}

// SetRangeCacheMaxBytes sets the resident-byte budget shared by every engine.
func (s *PackfileStore) SetRangeCacheMaxBytes(maxBytes int64) {
	s.budget.limit.Store(maxBytes)
	s.budget.reclaim()
}

// SetVerifyConcurrency replaces the shared verify/persist queue.
//
// Must be called before any reads begin; changing the queue while
// engines are servicing verify jobs is not supported.
func (s *PackfileStore) SetVerifyConcurrency(maxConcurrency int) error {
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		return ErrPackfileStoreClosed
	}
	if len(s.engines) != 0 {
		s.mtx.Unlock()
		return errors.New("SetVerifyConcurrency must be called before reads begin")
	}
	s.verifyQueue = newDefaultVerifyExecutor(maxConcurrency)
	s.mtx.Unlock()
	return nil
}

// SetStatsChangedCallback sets a callback invoked after observable stats change.
func (s *PackfileStore) SetStatsChangedCallback(fn func()) {
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		return
	}
	s.notify = fn
	engines := s.snapshotEnginesLocked()
	s.mtx.Unlock()
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
	ctx, task := trace.NewTask(ctx, "provider/spacewave/packfile/store/get-block")
	defer task.End()

	h := ref.GetHash()
	if h == nil {
		trace.Log(ctx, "result", "empty-hash")
		return nil, nil
	}
	trace.Log(ctx, "block-ref", ref.MarshalString())
	key := []byte(h.MarshalString())

	var stored *block.StoredBlock
	var lookup packLookup
	err := s.probePacks(key, &lookup, func(eng *PackReader) (bool, error) {
		trace.Log(ctx, "pack-id", eng.packID)
		var err error
		stored, err = eng.getBlock(ctx, key)
		return stored != nil, err
	})
	s.recordLookupStats(lookup)
	if err != nil {
		trace.Log(ctx, "result", "error")
		return nil, err
	}
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
	h := ref.GetHash()
	if h == nil {
		return false, nil
	}
	key := []byte(h.MarshalString())

	var lookup packLookup
	err := s.probePacks(key, &lookup, func(eng *PackReader) (bool, error) {
		return eng.getBlockExists(ctx, key)
	})
	s.recordLookupStats(lookup)
	return lookup.hit, err
}

// GetBlockExistsBatch checks whether each block exists.
func (s *PackfileStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	indexes := make(map[string][]int, len(refs))
	var keys []string
	for i, ref := range refs {
		h := ref.GetHash()
		if h == nil {
			continue
		}
		key := h.MarshalString()
		if _, ok := indexes[key]; !ok {
			keys = append(keys, key)
		}
		indexes[key] = append(indexes[key], i)
	}

	if len(keys) == 0 {
		return out, nil
	}

	var lookup packLookup
	defer func() {
		s.recordLookupStats(lookup)
	}()
	for _, key := range keys {
		var found bool
		err := s.probePacks([]byte(key), &lookup, func(eng *PackReader) (bool, error) {
			var err error
			found, err = eng.getBlockExists(ctx, []byte(key))
			return found, err
		})
		if err != nil {
			return nil, err
		}
		if found {
			for _, index := range indexes[key] {
				out[index] = true
			}
		}
	}
	return out, nil
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (s *PackfileStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	h := ref.GetHash()
	if h == nil {
		return nil, nil
	}
	key := []byte(h.MarshalString())

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
func (s *PackfileStore) probePacks(
	key []byte,
	lookup *packLookup,
	visit func(eng *PackReader) (bool, error),
) error {
	var entries []*packfile.PackfileEntry
	var filters []*bloom.Filter
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		entries, filters = s.manifest, s.filters
	})

	bloomKey := bloom.NewKey(key)
	var candidates []*packfile.PackfileEntry
	for i, entry := range entries {
		if filters[i] == nil || filters[i].TestKey(bloomKey) {
			candidates = append(candidates, entry)
		}
	}
	lookup.candidates += len(candidates)

	for _, entry := range candidates {
		size, err := manifestPackSize(entry)
		if err != nil {
			return err
		}
		if size <= 0 {
			continue
		}
		eng, err := s.getOrOpenEngine(entry.GetId(), size, entry.GetBlockCount())
		if err != nil {
			return errors.Wrap(err, "opening packfile")
		}
		lookup.opened++
		found, err := visit(eng)
		if err != nil {
			return err
		}
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
	var notify func()
	s.mtx.Lock()
	s.stats.LookupCount++
	s.stats.CandidatePacks += uint64(lookup.candidates) //nolint:gosec // counts are non-negative loop lengths.
	s.stats.OpenedPacks += uint64(lookup.opened)        //nolint:gosec // counts are non-negative loop lengths.
	s.stats.NegativePacks += uint64(lookup.negative)    //nolint:gosec // counts are non-negative loop lengths.
	if lookup.hit {
		s.stats.TargetHits++
	}
	s.stats.LastCandidatePacks = lookup.candidates
	s.stats.LastOpenedPacks = lookup.opened
	s.stats.LastNegativePacks = lookup.negative
	s.stats.LastTargetHit = lookup.hit
	notify = s.notify
	s.mtx.Unlock()
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
func (s *PackfileStore) PutBlockBatch(_ context.Context, entries []*block.PutBatchEntry) error {
	if len(entries) == 0 {
		return nil
	}
	return block_store.ErrReadOnly
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

// UpdateManifest filters superseded entries, orders cloud entries by descending
// sequence with an ascending pack-ID tie-break, then places zero-sequence local
// entries after them.
func (s *PackfileStore) UpdateManifest(entries []*packfile.PackfileEntry) {
	active := make([]*packfile.PackfileEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.GetId() == "" || entry.GetSupersededBy() != "" {
			continue
		}
		active = append(active, entry)
	}
	slices.SortStableFunc(active, func(a, b *packfile.PackfileEntry) int {
		aSequence := a.GetSequence()
		bSequence := b.GetSequence()
		if aSequence == 0 {
			if bSequence == 0 {
				return cmp.Compare(a.GetId(), b.GetId())
			}
			return 1
		}
		if bSequence == 0 {
			return -1
		}
		if result := cmp.Compare(bSequence, aSequence); result != 0 {
			return result
		}
		return cmp.Compare(a.GetId(), b.GetId())
	})

	// mtx fences manifest publication before Close clears the bloom state.
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		return
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.filters = parseManifestFilters(active, s.manifest, s.filters)
		s.manifest = active
		broadcast()
	})
	s.mtx.Unlock()

	s.evictInactiveEngines(active)
	s.notifyStatsChanged()
}

func (s *PackfileStore) evictInactiveEngines(entries []*packfile.PackfileEntry) {
	active := make(map[string]bool, len(entries))
	for _, entry := range entries {
		active[entry.GetId()] = true
	}
	var removed []*PackReader
	s.mtx.Lock()
	for id, engine := range s.engines {
		if !active[id] {
			delete(s.engines, id)
			removed = append(removed, engine)
		}
	}
	s.mtx.Unlock()
	for _, engine := range removed {
		if engine != nil {
			engine.Close()
		}
	}
}

func (s *PackfileStore) notifyStatsChanged() {
	s.mtx.Lock()
	notify := s.notify
	s.mtx.Unlock()
	if notify != nil {
		notify()
	}
}

// getOrOpenEngine returns the engine for a pack, opening and configuring
// it via the opener on the first request.
func (s *PackfileStore) getOrOpenEngine(packID string, size int64, blockCount uint64) (*PackReader, error) {
	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		return nil, ErrPackfileStoreClosed
	}
	if eng, ok := s.engines[packID]; ok {
		s.mtx.Unlock()
		return eng, nil
	}
	opener := s.opener
	cache := s.cache
	wbCtx := s.writebackCtx
	wbTarget := s.writebackTarget
	wbWindow := s.writebackWindow
	verify := s.verifyQueue
	overrides := s.tuningOverrides
	notify := s.notify
	s.mtx.Unlock()

	eng, err := opener(packID, size)
	if err != nil {
		return nil, err
	}
	// Rebind packID so the engine uses the manifest id rather than whatever
	// the opener chose (HTTP openers commonly use the URL).
	eng.packID = packID
	eng.SetExpectedBlockCount(blockCount)
	eng.SetIndexCache(cache)
	eng.SetWriteback(wbCtx, wbTarget, wbWindow)
	eng.setBudget(s.budget)
	eng.SetVerifyQueue(verify)
	eng.SetStatsChangedCallback(notify)
	overrides.apply(eng)

	s.mtx.Lock()
	if s.closed {
		s.mtx.Unlock()
		eng.Close()
		return nil, ErrPackfileStoreClosed
	}
	if existing, ok := s.engines[packID]; ok {
		// Raced with another opener; discard ours.
		s.mtx.Unlock()
		eng.Close()
		return existing, nil
	}
	s.engines[packID] = eng
	s.mtx.Unlock()
	return eng, nil
}

// snapshotEnginesLocked returns the open engines. Caller holds mtx.
func (s *PackfileStore) snapshotEnginesLocked() []*PackReader {
	return slices.Collect(maps.Values(s.engines))
}

// parseManifestFilters returns the bloom filter of each entry, reusing filters
// already parsed for the previous manifest. Entries without a usable filter get
// nil, which matches every key.
func parseManifestFilters(
	entries []*packfile.PackfileEntry,
	prevEntries []*packfile.PackfileEntry,
	prevFilters []*bloom.Filter,
) []*bloom.Filter {
	prev := make(map[string]*bloom.Filter, len(prevEntries))
	for i, entry := range prevEntries {
		prev[entry.GetId()] = prevFilters[i]
	}

	filters := make([]*bloom.Filter, len(entries))
	for i, entry := range entries {
		if bf, ok := prev[entry.GetId()]; ok {
			filters[i] = bf
			continue
		}
		var pbf bloom.BloomFilter
		if err := pbf.UnmarshalBlock(entry.GetBloomFilter()); err == nil {
			filters[i] = pbf.ToBloomFilter()
		}
	}
	return filters
}

// _ is a type assertion
var _ block.StoreOps = (*PackfileStore)(nil)
