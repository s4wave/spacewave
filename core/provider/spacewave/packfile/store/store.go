package store

import (
	"context"
	"sync"
	"weak"

	"github.com/aperturerobotics/util/broadcast"
	bbloom "github.com/bits-and-blooms/bloom/v3"
	"github.com/pkg/errors"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/bloom"
	block_store "github.com/s4wave/spacewave/db/block/store"
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

// bloomNode is a node in the manifest's bloom pruning tree.
type bloomNode struct {
	// merged is the OR-merged bloom filter covering all children.
	merged *bbloom.BloomFilter
	// left is the left child (nil for leaf nodes).
	left *bloomNode
	// right is the right child (nil for leaf nodes).
	right *bloomNode
	// entryIdx is the manifest index for leaf nodes (-1 for internal nodes).
	entryIdx int
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

	mu      sync.Mutex
	engines map[string]*PackReader
	stats   packLookupStats
	notify  func()

	// bcast guards manifest/bloom state.
	bcast broadcast.Broadcast

	// writebackCtx is the long-lived ctx used for async writebacks.
	// nil disables writeback.
	writebackCtx context.Context
	// writebackTarget receives verified cache copies when writeback is enabled.
	writebackTarget block.StoreOps
	// writebackWindow is the byte window for selecting neighbor blocks.
	writebackWindow int64
	// maxBytes is the resident-byte budget applied to each engine.
	maxBytes int64
	// tuningOverrides are explicit per-engine tuning overrides.
	tuningOverrides engineTuningOverrides

	// manifest state, guarded by bcast.
	manifest []*packfile.PackfileEntry
	blooms   map[string]weak.Pointer[bbloom.BloomFilter]
	tree     *bloomNode
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
		maxBytes:        defaultResidentBudget,
		blooms:          make(map[string]weak.Pointer[bbloom.BloomFilter]),
	}
	return s
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
	s.mu.Lock()
	s.writebackCtx = ctx
	s.writebackTarget = target
	s.writebackWindow = windowBytes
	engines := make([]*PackReader, 0, len(s.engines))
	for _, e := range s.engines {
		engines = append(engines, e)
	}
	s.mu.Unlock()
	for _, e := range engines {
		e.SetWriteback(ctx, target, windowBytes)
	}
}

// SetRangeCacheMaxBytes sets the resident-byte budget applied to each engine.
func (s *PackfileStore) SetRangeCacheMaxBytes(maxBytes int64) {
	s.mu.Lock()
	s.maxBytes = maxBytes
	engines := make([]*PackReader, 0, len(s.engines))
	for _, e := range s.engines {
		engines = append(engines, e)
	}
	s.mu.Unlock()
	for _, e := range engines {
		e.SetMaxBytes(maxBytes)
	}
}

// SetVerifyConcurrency replaces the shared verify/persist queue.
//
// Must be called before any reads begin; changing the queue while
// engines are servicing verify jobs is not supported.
func (s *PackfileStore) SetVerifyConcurrency(maxConcurrency int) error {
	s.mu.Lock()
	if len(s.engines) != 0 {
		s.mu.Unlock()
		return errors.New("SetVerifyConcurrency must be called before reads begin")
	}
	s.verifyQueue = newDefaultVerifyExecutor(maxConcurrency)
	s.mu.Unlock()
	return nil
}

// SetStatsChangedCallback sets a callback invoked after observable stats change.
func (s *PackfileStore) SetStatsChangedCallback(fn func()) {
	s.mu.Lock()
	s.notify = fn
	engines := make([]*PackReader, 0, len(s.engines))
	for _, e := range s.engines {
		engines = append(engines, e)
	}
	s.mu.Unlock()
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

// GetBlock gets a block by reference from the packfile store.
//
// The manifest's bloom pruning selects candidate packs, and each candidate
// engine is consulted in turn. The first engine that finds the block
// returns its bytes.
func (s *PackfileStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	h := ref.GetHash()
	if h == nil {
		return nil, false, nil
	}
	key := []byte(h.MarshalString())

	var entries []*packfile.PackfileEntry
	var tree *bloomNode
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		entries = s.manifest
		tree = s.tree
	})
	if len(entries) == 0 {
		return nil, false, nil
	}

	candidates := s.findCandidates(key, entries, tree)
	opened := 0
	negative := 0
	hit := false
	defer func() {
		s.recordLookupStats(len(candidates), opened, negative, hit)
	}()

	for _, idx := range candidates {
		entry := entries[idx]
		// Bloom filter prune (per-pack fallback when tree not available).
		if tree == nil {
			bf := s.getOrDeserializeBloom(entry)
			if bf != nil && !bf.Test(key) {
				continue
			}
		}
		size := int64(entry.GetSizeBytes())
		if size <= 0 {
			continue
		}
		eng, err := s.getOrOpenEngine(entry.GetId(), size, entry.GetBlockCount())
		if err != nil {
			return nil, false, errors.Wrap(err, "opening packfile")
		}
		opened++
		data, found, err := eng.getBlock(ctx, key)
		if err != nil {
			return data, found, err
		}
		if found {
			hit = true
			return data, found, err
		}
		negative++
	}

	return nil, false, nil
}

func (s *PackfileStore) recordLookupStats(candidateCount, openedCount, negativeCount int, targetHit bool) {
	var notify func()
	s.mu.Lock()
	s.stats.LookupCount++
	s.stats.CandidatePacks += uint64(candidateCount)
	s.stats.OpenedPacks += uint64(openedCount)
	s.stats.NegativePacks += uint64(negativeCount)
	if targetHit {
		s.stats.TargetHits++
	}
	s.stats.LastCandidatePacks = candidateCount
	s.stats.LastOpenedPacks = openedCount
	s.stats.LastNegativePacks = negativeCount
	s.stats.LastTargetHit = targetHit
	notify = s.notify
	s.mu.Unlock()
	if notify != nil {
		notify()
	}
}

// GetBlockExists reports whether a block exists in the store.
func (s *PackfileStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	_, found, err := s.GetBlock(ctx, ref)
	return found, err
}

// GetBlockExistsBatch checks whether each block exists.
func (s *PackfileStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := s.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
}

// StatBlock returns metadata about a block without reading its data.
// Returns nil, nil if the block does not exist.
func (s *PackfileStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	_, found, err := s.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: -1}, nil
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

// PutBlockBackground is not supported on a read-only store.
func (s *PackfileStore) PutBlockBackground(_ context.Context, _ []byte, _ *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, block_store.ErrReadOnly
}

// RmBlock is not supported on a read-only store.
func (s *PackfileStore) RmBlock(_ context.Context, _ *block.BlockRef) error {
	return block_store.ErrReadOnly
}

// Flush has no buffered work for the read-only store.
func (s *PackfileStore) Flush(_ context.Context) error {
	return nil
}

// BeginDeferFlush is a no-op for the read-only store.
func (s *PackfileStore) BeginDeferFlush() {}

// EndDeferFlush is a no-op for the read-only store.
func (s *PackfileStore) EndDeferFlush(_ context.Context) error {
	return nil
}

// UpdateManifest replaces the manifest and rebuilds the bloom tree.
func (s *PackfileStore) UpdateManifest(entries []*packfile.PackfileEntry) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.manifest = entries
		s.tree = buildBloomTree(entries, s.blooms)
		broadcast()
	})
	s.notifyStatsChanged()
}

func (s *PackfileStore) notifyStatsChanged() {
	s.mu.Lock()
	notify := s.notify
	s.mu.Unlock()
	if notify != nil {
		notify()
	}
}

// getOrOpenEngine returns the engine for a pack, opening and configuring
// it via the opener on the first request.
func (s *PackfileStore) getOrOpenEngine(packID string, size int64, blockCount uint64) (*PackReader, error) {
	s.mu.Lock()
	if eng, ok := s.engines[packID]; ok {
		s.mu.Unlock()
		return eng, nil
	}
	opener := s.opener
	cache := s.cache
	wbCtx := s.writebackCtx
	wbTarget := s.writebackTarget
	wbWindow := s.writebackWindow
	maxBytes := s.maxBytes
	verify := s.verifyQueue
	overrides := s.tuningOverrides
	notify := s.notify
	s.mu.Unlock()

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
	eng.SetMaxBytes(maxBytes)
	eng.SetVerifyQueue(verify)
	eng.SetStatsChangedCallback(notify)
	overrides.apply(eng)

	s.mu.Lock()
	if existing, ok := s.engines[packID]; ok {
		// Raced with another opener; discard ours.
		s.mu.Unlock()
		return existing, nil
	}
	s.engines[packID] = eng
	s.mu.Unlock()
	return eng, nil
}

// findCandidates returns manifest indices that might contain the key.
func (s *PackfileStore) findCandidates(key []byte, entries []*packfile.PackfileEntry, tree *bloomNode) []int {
	if tree == nil {
		result := make([]int, len(entries))
		for i := range entries {
			result[i] = i
		}
		return result
	}
	var result []int
	collectCandidates(tree, key, &result)
	return result
}

// collectCandidates traverses the bloom tree, pruning subtrees.
func collectCandidates(node *bloomNode, key []byte, result *[]int) {
	if node == nil {
		return
	}
	if node.merged != nil && !node.merged.Test(key) {
		return
	}
	if node.entryIdx >= 0 {
		*result = append(*result, node.entryIdx)
		return
	}
	collectCandidates(node.left, key, result)
	collectCandidates(node.right, key, result)
}

// getOrDeserializeBloom returns the bloom filter for an entry, using the
// store's weak pointer cache so filters share memory across calls while
// remaining eligible for GC when no caller retains them.
func (s *PackfileStore) getOrDeserializeBloom(entry *packfile.PackfileEntry) *bbloom.BloomFilter {
	id := entry.GetId()
	bloomData := entry.GetBloomFilter()
	if len(bloomData) == 0 {
		return nil
	}
	if wp, ok := s.blooms[id]; ok {
		if bf := wp.Value(); bf != nil {
			return bf
		}
	}
	var pbf bloom.BloomFilter
	if err := pbf.UnmarshalBlock(bloomData); err != nil {
		return nil
	}
	bf := pbf.ToBloomFilter()
	if bf == nil {
		return nil
	}
	s.blooms[id] = weak.Make(bf)
	return bf
}

// buildBloomTree builds a binary bloom tree from manifest entries.
func buildBloomTree(entries []*packfile.PackfileEntry, blooms map[string]weak.Pointer[bbloom.BloomFilter]) *bloomNode {
	if len(entries) == 0 {
		return nil
	}
	leaves := make([]*bloomNode, len(entries))
	for i, entry := range entries {
		var bf *bbloom.BloomFilter
		bloomData := entry.GetBloomFilter()
		if len(bloomData) > 0 {
			if wp, ok := blooms[entry.GetId()]; ok {
				bf = wp.Value()
			}
			if bf == nil {
				var pbf bloom.BloomFilter
				if err := pbf.UnmarshalBlock(bloomData); err == nil {
					bf = pbf.ToBloomFilter()
					if bf != nil {
						blooms[entry.GetId()] = weak.Make(bf)
					}
				}
			}
		}
		leaves[i] = &bloomNode{
			merged:   bf,
			entryIdx: i,
		}
	}
	nodes := leaves
	for len(nodes) > 1 {
		var next []*bloomNode
		for i := 0; i < len(nodes); i += 2 {
			if i+1 >= len(nodes) {
				next = append(next, nodes[i])
				continue
			}
			merged := mergeBloomFilters(nodes[i].merged, nodes[i+1].merged)
			next = append(next, &bloomNode{
				merged:   merged,
				left:     nodes[i],
				right:    nodes[i+1],
				entryIdx: -1,
			})
		}
		nodes = next
	}
	return nodes[0]
}

// mergeBloomFilters OR-merges two bloom filters.
func mergeBloomFilters(a, b *bbloom.BloomFilter) *bbloom.BloomFilter {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	merged := a.Copy()
	if err := merged.Merge(b); err != nil {
		return nil
	}
	return merged
}

// _ is a type assertion
var _ block.StoreOps = ((*PackfileStore)(nil))
