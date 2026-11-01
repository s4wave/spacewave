//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"cmp"
	"context"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

// batchConcurrency bounds the concurrent requests of one batch call.
const batchConcurrency = 16

const (
	// packDir holds the packfiles under the object prefix.
	packDir = "packs/"
	// entryDir holds one PackfileEntry per complete packfile.
	entryDir = "entries/"
)

const (
	// packPayloadBytes bounds the block bytes of a pack an upload builds. It is
	// half the packfile ceiling, which leaves room for the refs, the index and
	// the encoding around the block data.
	packPayloadBytes = writer.DefaultMaxPackBytes / 2
	// packMaxBlocks bounds the blocks of a pack an upload builds. A pack of
	// packPayloadBytes holds it at blocks averaging 2 KiB, so the bytes bound
	// first. The packfile policy's smaller count is the wire limit of
	// Spacewave Cloud, which a bucket pack never crosses.
	packMaxBlocks = 16384
	// packUploads bounds the packs an upload writes at once. Each holds about
	// three copies of its blocks in memory while it writes.
	packUploads = 3
	// packLinger is how long a short pack of settled blocks waits for more.
	packLinger = 30 * time.Second
)

// ErrPackImmutable is returned when removing a block from a PackStore.
var ErrPackImmutable = errors.New("bucket packfiles do not remove blocks")

// PackStore is a block store on an S3-compatible bucket that writes each batch
// of blocks as one packfile, so small blocks cost one request per batch
// instead of one each.
//
// A packfile is the object {prefix}packs/{id}: a kvfile of block objects, the
// bytes and refs of each block, keyed by block hash. The object
// {prefix}entries/{id} holds its PackfileEntry with the bloom filter readers
// use to find the pack of a block. The entry is written after the packfile, so
// a listed entry names a complete packfile.
//
// Reads use the entries this store wrote or listed. The first read lists the
// entries, and a background routine lists them again every refresh interval,
// which finds the packfiles other writers added or merged since. A read that
// finds no block does not list: a block missing from every known packfile is
// missing, or in a packfile the next refresh finds. A read that finds a
// packfile deleted lists the entries at once.
//
// After each write a background routine merges small packfiles, as
// compact describes. Reclaim drops dead blocks when that saves money on the
// service's price list. A store that keeps its packs does neither, so no
// packfile it lists is ever rewritten or deleted, except by a tail merge.
//
// With a tail merge size set, a short upload batch merges into the newest
// packfile below that size, as mergeTail describes, so a store that keeps its
// packs does not gather one small packfile per trickle of writes.
type PackStore struct {
	// le is the logger.
	le *logrus.Entry
	// client stores the objects.
	client ObjectStore
	// bucket is the bucket holding the objects.
	bucket string
	// prefix precedes every object key.
	prefix string
	// pricing is the price list of the service holding the bucket.
	pricing *Pricing
	// keepPacks stops compaction and reclaim.
	keepPacks bool
	// packs reads blocks from the known packfiles.
	packs *packfile_store.PackfileStore
	// compactor runs compact after writes.
	compactor *routine.RoutineContainer
	// refresher lists the entries every refreshInterval once a read started it.
	refresher *routine.RoutineContainer
	// refreshInterval is the time between two listings of the refresher.
	refreshInterval time.Duration
	// readWindow is the pack readers' minimum fetch, or zero for their default.
	readWindow int
	// linger is how long a short upload batch waits for more blocks.
	linger time.Duration
	// mergeTailBytes is the packfile size below which a short batch merges
	// into the newest packfile, or zero to never merge.
	mergeTailBytes int64
	// index is the entry cache and block location index in the local KV, or
	// nil without one.
	index *packIndex
	// indexer indexes the listed packs in the background.
	indexer *routine.RoutineContainer
	// cacheLoaded is set once the cached entries were read. Guarded by listMtx.
	cacheLoaded bool
	// refreshOnce starts the refresher at the first read.
	refreshOnce sync.Once
	// loaded is set once a listing completed, which makes the entries the
	// bucket's own.
	loaded atomic.Bool

	// bcast guards entries and writes.
	bcast broadcast.Broadcast
	// entries are the known packfiles by id.
	entries map[string]*packfile.PackfileEntry
	// writes counts the packfiles this store wrote.
	writes uint64

	// listMtx serializes entry listings.
	listMtx csync.Mutex
	// listings counts the entry listings started.
	listings atomic.Uint64
	// dropped counts the packfile entries listings dropped.
	dropped atomic.Uint64
	// compactMtx serializes compaction and reclaim passes.
	compactMtx csync.Mutex
	// reclaimLoaded reports whether reclaimState was read from the bucket.
	// Guarded by compactMtx.
	reclaimLoaded bool
	// reclaimState is the last reclaim pass, or nil before the first.
	// Guarded by compactMtx.
	reclaimState *ReclaimState
	// passWrites is writes when this store last ran a reclaim pass.
	// Guarded by compactMtx.
	passWrites uint64
}

// DefaultRefreshInterval is the time between two listings of a PackStore's
// packfile entries. Each listing costs one request per thousand entries, so a
// minute bounds the cost of an idle reader while it finds another device's
// packfiles within the minute.
const DefaultRefreshInterval = time.Minute

// compactBackoff spaces the retries of a failed compaction pass.
var compactBackoff = &backoff.Backoff{
	BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
	Exponential: &backoff.Exponential{
		InitialInterval: 1000,
		MaxInterval:     60000,
		Multiplier:      2,
	},
}

// PackStoreOptions configures a PackStore.
type PackStoreOptions struct {
	// Pricing is the price list of the service holding the bucket; nil uses
	// PricingForEndpoint for a *Client and the Amazon S3 prices otherwise.
	Pricing *Pricing
	// KeepPacks stops compaction and reclaim, so the store only ever adds
	// packfiles.
	KeepPacks bool
	// RefreshInterval is the time between two listings of the packfile
	// entries, which find the packfiles other writers added. Zero uses
	// DefaultRefreshInterval.
	RefreshInterval time.Duration
	// Index is a local KV store for the entry cache and the block location
	// index; nil keeps neither. The store owns every key in it, so pass a
	// prefixed view.
	//
	// With it, a start reads the cached entries and lists the bucket in the
	// background, reading only the entries it does not know. A lookup reads the
	// pack the index places a block in. A background routine indexes the listed
	// packs: one read of the end of each pack and a few short write transactions,
	// with a pause between them. It takes no lock a read waits on. Until a
	// pack is indexed, lookups test its bloom filter as without the index.
	Index kvtx.Store
	// ReadWindow is the fewest bytes one block miss fetches from a pack, and
	// their alignment. Zero uses the pack reader's default, which suits
	// HTTP range requests.
	ReadWindow int
	// Linger is how long a short upload batch of settled blocks waits for more
	// blocks before it is written anyway. Zero uses packLinger.
	Linger time.Duration
	// MergeTailBytes merges a batch, when not zero, into the newest packfile
	// smaller than this many bytes while the merged packfile fits one upload
	// batch, and deletes the packfile it replaces, even with KeepPacks set.
	MergeTailBytes int64
}

// NewPackStore builds a packfile block store on the bucket. opts may be nil.
// Close stops its compaction routine.
func NewPackStore(le *logrus.Entry, client ObjectStore, bucket, prefix string, opts *PackStoreOptions) *PackStore {
	// Price the bucket's service from its endpoint unless overridden.
	if opts == nil {
		opts = &PackStoreOptions{}
	}
	pricing := opts.Pricing
	if c, ok := client.(*Client); ok && pricing == nil {
		pricing = PricingForEndpoint(c.endpoint)
	}
	refresh := opts.RefreshInterval
	if refresh <= 0 {
		refresh = DefaultRefreshInterval
	}
	pricing = cmp.Or(pricing, awsPricing)
	s := &PackStore{
		le:        le,
		client:    client,
		bucket:    bucket,
		prefix:    prefix,
		pricing:   pricing,
		keepPacks: opts.KeepPacks,
		entries:   make(map[string]*packfile.PackfileEntry),
		compactor: routine.NewRoutineContainerWithLogger(
			le.WithField("routine", "pack-compaction"),
			routine.WithRetry(compactBackoff),
		),
		refresher: routine.NewRoutineContainerWithLogger(
			le.WithField("routine", "pack-refresh"),
			routine.WithRetry(compactBackoff),
		),
		refreshInterval: refresh,
		readWindow:      opts.ReadWindow,
		linger:          cmp.Or(max(opts.Linger, 0), packLinger),
		mergeTailBytes:  max(opts.MergeTailBytes, 0),
	}

	// Read the known packfiles and start compaction.
	// The local index, when there is one, also keeps each pack's index tail,
	// so a restart does not read the tails again.
	var tails packfile_store.IndexCache
	if opts.Index != nil {
		s.index = newPackIndex(opts.Index)
		tails = s.index
	}
	s.packs = packfile_store.NewPackfileStore(s.openPack, tails)
	s.compactor.SetRoutine(s.runCompaction)
	s.compactor.SetContext(context.Background(), false)
	s.refresher.SetRoutine(s.runRefresh)
	if s.index != nil {
		s.packs.SetLocator(s.index)
		s.indexer = routine.NewRoutineContainerWithLogger(
			le.WithField("routine", "pack-index"),
			routine.WithRetry(compactBackoff),
		)
		s.indexer.SetRoutine(s.runIndexer)
	}
	return s
}

// Close stops compaction and the refresh and releases the open packfile
// readers.
func (s *PackStore) Close() {
	s.compactor.ClearContext()
	s.refresher.ClearContext()
	if s.indexer != nil {
		s.indexer.ClearContext()
	}
	s.packs.Close()
}

// GetHashType returns 0 to use the default hash type.
func (s *PackStore) GetHashType() hash.HashType {
	return 0
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (s *PackStore) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeature_STORE_FEATURE_UNKNOWN
}

// BeginReadOperation returns the store as the scoped read handle.
func (s *PackStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// PutBlock writes one block as its own packfile.
func (s *PackStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Build the block reference before writing its packfile.
	ref, err := block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}

	// Write the block and its references through the batch packfile path.
	entry := &block.PutBatchEntry{Ref: ref, Data: data, Refs: opts.GetRefs(), RefsKnown: opts.GetRefsKnown()}
	if _, err := s.PutBlockBatch(ctx, []*block.PutBatchEntry{entry}); err != nil {
		return nil, false, err
	}
	return ref, false, nil
}

// GetBatchLimits returns the batch that fills a pack, so an upload writes few
// large packs instead of many small ones.
func (s *PackStore) GetBatchLimits() block.BatchLimits {
	return block.BatchLimits{Bytes: packPayloadBytes, Blocks: packMaxBlocks, Workers: packUploads, Linger: s.linger}
}

// PutBlockBatch writes the batch as one packfile and its entry, or merges it
// into the newest small packfile, as mergeTail describes.
//
// The batch must fit one packfile: at most writer.DefaultMaxPackBytes bytes.
// Tombstones are rejected. Every block is written again, so none is reported
// as existing.
func (s *PackStore) PutBlockBatch(ctx context.Context, batch []*block.PutBatchEntry) ([]bool, error) {
	// Reject tombstones before writing anything.
	for _, entry := range batch {
		if entry.Tombstone {
			return nil, ErrPackImmutable
		}
	}

	// Merge a short batch into the newest small packfile when one fits.
	if s.mergeTailBytes > 0 {
		merged, err := s.mergeTail(ctx, batch)
		if err != nil {
			return nil, err
		}
		if merged {
			return make([]bool, len(batch)), nil
		}
	}

	// Write the batch as one immutable packfile.
	i := 0
	entry, err := s.writePack(ctx, func() (*hash.Hash, *block.StoredBlock, error) {
		// Yield each batch block and reject tombstones.
		if i == len(batch) {
			return nil, nil, nil
		}
		entry := batch[i]
		i++
		return entry.Ref.GetHash(), &block.StoredBlock{Data: entry.Data, Refs: entry.Refs, RefsKnown: true}, nil
	})
	if err != nil || entry == nil {
		return nil, err
	}

	s.published(entry)
	return make([]bool, len(batch)), nil
}

// published lists a packfile this store wrote and wakes compaction watchers.
func (s *PackStore) published(entry *packfile.PackfileEntry) {
	s.updateEntries([]*packfile.PackfileEntry{entry}, nil)
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.writes++
		broadcast()
	})
}

// writePack writes the blocks next yields as one packfile, then its entry.
// Returns nil when next yields no blocks.
func (s *PackStore) writePack(ctx context.Context, next writer.BlockIterator) (*packfile.PackfileEntry, error) {
	// Pack the blocks in memory within the size limit.
	var buf bytes.Buffer
	var keys [][]byte
	result, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		h, b, err := next()
		if h != nil && s.index != nil {
			keys = append(keys, packfile.BlockKey(h))
		}
		return h, b, err
	})
	if err != nil {
		return nil, err
	}
	if result.BlockCount == 0 {
		return nil, nil
	}
	if int64(buf.Len()) > writer.DefaultMaxPackBytes {
		return nil, errors.Errorf("packfile of %d bytes exceeds the %d byte limit", buf.Len(), writer.DefaultMaxPackBytes)
	}

	// Name the packfile randomly: compaction and reclaim delete packfiles by id,
	// so an id derived from the blocks would let a rewrite of the same blocks
	// land on a packfile being deleted and leave its entry without a packfile.
	id := identity.NewPackID()
	entry := &packfile.PackfileEntry{
		Id:                 id,
		BloomFilter:        result.BloomFilter,
		BloomFormatVersion: packfile.BloomFormatVersionV1,
		BlockCount:         result.BlockCount,
		SizeBytes:          result.BytesWritten,
		CreatedAt:          timestamppb.New(time.Now().UTC()),
	}
	entryData, err := entry.MarshalVT()
	if err != nil {
		return nil, err
	}

	// Upload the packfile before the entry that lists it.
	if err := s.client.PutObject(ctx, s.bucket, s.prefix+packDir+id, buf.Bytes(), "application/octet-stream"); err != nil {
		return nil, errors.Wrap(err, "write packfile")
	}
	if err := s.client.PutObject(ctx, s.bucket, s.prefix+entryDir+id, entryData, "application/octet-stream"); err != nil {
		return nil, errors.Wrap(err, "write packfile entry")
	}

	// Index the keys with the entry, so the pack is found at once.
	if s.index != nil {
		if err := s.index.addWritten(ctx, entry, keys); err != nil {
			return nil, errors.Wrap(err, "index packfile")
		}
	}
	return entry, nil
}

// GetBlock looks up a block in the store.
func (s *PackStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	stored, err := s.GetStoredBlock(ctx, ref)
	return stored.GetData(), stored != nil, err
}

// GetStoredBlock returns the block with its refs. Returns nil if the block is
// not found.
func (s *PackStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if ref.GetEmpty() {
		return nil, block.ErrEmptyBlockRef
	}
	var stored *block.StoredBlock
	err := s.lookup(ctx, func() (bool, error) {
		var err error
		stored, err = s.packs.GetStoredBlock(ctx, ref)
		return stored != nil, err
	})
	return stored, err
}

// GetBlockExists checks the packfiles for the block.
func (s *PackStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	var exists bool
	err := s.lookup(ctx, func() (bool, error) {
		var err error
		exists, err = s.packs.GetBlockExists(ctx, ref)
		return exists, err
	})
	return exists, err
}

// GetBlockExistsBatch checks the packfiles for each block.
func (s *PackStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	var exists []bool
	err := s.lookup(ctx, func() (bool, error) {
		var err error
		exists, err = s.packs.GetBlockExistsBatch(ctx, refs)
		return err == nil && !slices.Contains(exists, false), err
	})
	return exists, err
}

// StatBlock returns the block's metadata, or nil if the block is not found.
func (s *PackStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	var stat *block.BlockStat
	err := s.lookup(ctx, func() (bool, error) {
		var err error
		stat, err = s.packs.StatBlock(ctx, ref)
		return stat != nil, err
	})
	return stat, err
}

// RmBlock returns ErrPackImmutable: packfiles are written once.
func (s *PackStore) RmBlock(context.Context, *block.BlockRef) error {
	return ErrPackImmutable
}

// Sync reports always-durable: each batch is written before PutBlockBatch
// returns.
func (s *PackStore) Sync(context.Context) (bool, error) {
	return true, nil
}

// lookup runs find against the known packfiles, which the first lookup lists.
// A miss is final. When find reports a deleted packfile, lookup lists the
// entries, which drops the packfiles merged away and finds their merged
// packfile, and runs find again. It repeats while each listing drops a
// packfile, so a reader crosses any number of tail merges that replace its
// packfile while it lists, and stops when a listing drops none.
func (s *PackStore) lookup(ctx context.Context, find func() (bool, error)) error {
	// Load the packfile entries before the first lookup and keep them fresh.
	if err := s.loadEntries(ctx); err != nil {
		return err
	}
	s.refreshOnce.Do(func() {
		s.refresher.SetContext(context.Background(), false)
		if s.indexer != nil {
			s.indexer.SetContext(context.Background(), false)
		}
	})

	// Look for the block in the known packfiles, and list the entries again
	// after each deleted packfile while the listings drop packfiles.
	for {
		listed, dropped := s.listings.Load(), s.dropped.Load()
		found, err := find()
		if found || err == nil || !errors.Is(err, ErrNotFound) {
			return err
		}
		if err := s.listEntries(ctx, listed); err != nil {
			return err
		}
		if s.dropped.Load() == dropped {
			return err
		}
	}
}

// loadEntries lists the packfile entries unless a listing completed.
func (s *PackStore) loadEntries(ctx context.Context) error {
	for !s.loaded.Load() {
		if err := s.listEntries(ctx, s.listings.Load()); err != nil {
			return err
		}
	}
	return nil
}

// runRefresh lists the packfile entries every refreshInterval until ctx ends.
// The routine's backoff retries a failed listing.
func (s *PackStore) runRefresh(ctx context.Context) error {
	timer := time.NewTimer(s.refreshInterval)
	defer timer.Stop()
	for first := true; ; first = false {
		// A start from the cached entries lists at once.
		if !first || s.listings.Load() != 0 {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-timer.C:
			}
		}
		if err := s.listEntries(ctx, s.listings.Load()); err != nil {
			return err
		}
		timer.Reset(s.refreshInterval)
	}
}

// listEntries lists the packfile entries, reads the new ones, and drops the
// known ones the listing no longer holds, unless a listing started after the
// caller's count of listed already completed. Entries added while the listing
// runs are kept.
func (s *PackStore) listEntries(ctx context.Context, listed uint64) error {
	// Serialize packfile listings and skip a listing already completed.
	release, err := s.listMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	if s.listings.Load() != listed {
		return nil
	}

	// Start from the cached entries when there are any, and list in the
	// background: the cache holds the packs the last run knew.
	if s.index != nil && !s.cacheLoaded {
		s.cacheLoaded = true
		cached, err := s.index.load(ctx)
		if err != nil {
			return err
		}
		if len(cached) != 0 {
			s.updateEntries(cached, nil)
			s.loaded.Store(true)
			return nil
		}
	}
	s.listings.Add(1)

	// Snapshot the packfile entries known before the listing starts.
	var known map[string]bool
	s.bcast.HoldLock(func(func(), func() <-chan struct{}) {
		known = make(map[string]bool, len(s.entries))
		for id := range s.entries {
			known[id] = true
		}
	})

	// List until a listing reads every new entry it names. A tail merge
	// writes the merged entry before it deletes the replaced one, so an entry
	// that vanished after the listing means the listing predates the merged
	// entry and may miss it.
	read := make(map[string]*packfile.PackfileEntry)
	var ids map[string]bool
	for {
		var vanished bool
		ids, vanished, err = s.listOnce(ctx, known, read)
		if err != nil {
			return err
		}
		if !vanished {
			break
		}
	}

	// Index and publish the new entries the last listing holds, and drop the
	// known entries it no longer holds.
	var entries []*packfile.PackfileEntry
	for id, entry := range read {
		if !ids[id] {
			continue
		}
		entries = append(entries, entry)
		if s.index != nil {
			if err := s.index.addListed(ctx, entry); err != nil {
				return err
			}
		}
	}
	if s.index != nil {
		if err := s.index.flush(ctx); err != nil {
			return err
		}
	}
	var removed []string
	for id := range known {
		if !ids[id] {
			removed = append(removed, id)
		}
	}
	s.updateEntries(entries, removed)
	s.loaded.Store(true)
	return nil
}

// listOnce lists the packfile entries and reads each one neither known nor in
// read into read. It returns the listed ids, and whether a listed entry was
// deleted before it could be read.
func (s *PackStore) listOnce(ctx context.Context, known map[string]bool, read map[string]*packfile.PackfileEntry) (map[string]bool, bool, error) {
	// List packfile entries and identify the new ones.
	listed := make(map[string]bool, len(known))
	var ids []string
	dir := s.prefix + entryDir
	err := s.client.ListObjects(ctx, s.bucket, dir, func(key string, _ int64) error {
		id := strings.TrimPrefix(key, dir)
		listed[id] = true
		if _, ok := read[id]; !known[id] && !ok {
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return nil, false, errors.Wrap(err, "list packfile entries")
	}

	// Read new packfile entries with bounded concurrent requests.
	entries := make([]*packfile.PackfileEntry, len(ids))
	var vanished atomic.Bool
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(s.readConcurrency())
	for i, id := range ids {
		eg.Go(func() error {
			entry, err := s.readEntry(egCtx, id)
			if errors.Is(err, ErrNotFound) {
				vanished.Store(true)
				return nil
			}
			entries[i] = entry
			return err
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, false, err
	}
	for _, entry := range entries {
		if entry != nil {
			read[entry.GetId()] = entry
		}
	}
	return listed, vanished.Load(), nil
}

// readConcurrency returns the concurrent requests that read the packfile
// entries: batchConcurrency, or the width of the object store when it is wider,
// as each entry is one request on one of its connections.
func (s *PackStore) readConcurrency() int {
	if w, ok := s.client.(interface{ Conns() int }); ok {
		return max(batchConcurrency, w.Conns())
	}
	return batchConcurrency
}

// readEntry reads the entry object of packfile id.
func (s *PackStore) readEntry(ctx context.Context, id string) (*packfile.PackfileEntry, error) {
	// Read and close the packfile entry object body.
	body, err := s.client.GetObject(ctx, s.bucket, s.prefix+entryDir+id)
	if err != nil {
		return nil, errors.Wrap(err, "read packfile entry")
	}
	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		return nil, errors.Wrap(err, "read packfile entry")
	}

	// Decode the packfile entry and verify it names the requested packfile.
	entry := &packfile.PackfileEntry{}
	if err := entry.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "decode packfile entry "+id)
	}
	if entry.GetId() != id {
		return nil, errors.Errorf("packfile entry %s names packfile %s", id, entry.GetId())
	}
	return entry, nil
}

// updateEntries adds and removes known packfiles and publishes the result to
// the reader. Publishing under the lock keeps concurrent updates in order.
func (s *PackStore) updateEntries(add []*packfile.PackfileEntry, remove []string) {
	if s.index != nil {
		if err := s.index.remove(context.Background(), remove); err != nil {
			s.le.WithError(err).Warn("drop packs from the index")
		}
	}
	s.bcast.HoldLock(func(func(), func() <-chan struct{}) {
		for _, id := range remove {
			delete(s.entries, id)
		}
		s.dropped.Add(uint64(len(remove)))
		for _, entry := range add {
			s.entries[entry.GetId()] = entry
		}
		s.packs.UpdateManifest(slices.Collect(maps.Values(s.entries)))
	})
}

// openPack opens a reader over the ranges of packfile id.
func (s *PackStore) openPack(id string, size int64) (*packfile_store.PackReader, error) {
	transport := &rangeTransport{client: s.client, bucket: s.bucket, key: s.prefix + packDir + id}
	eng := packfile_store.NewPackReader(id, size, transport)
	eng.SetReadWindow(s.readWindow)
	return eng, nil
}

// rangeTransport reads byte ranges of one packfile object.
type rangeTransport struct {
	client ObjectStore
	bucket string
	key    string
}

// Fetch reads bytes [off, off+length) of the object.
func (t *rangeTransport) Fetch(ctx context.Context, off int64, length int) ([]byte, error) {
	if length <= 0 {
		return nil, nil
	}
	return t.client.GetObjectRange(ctx, t.bucket, t.key, off, length)
}

// _ is a type assertion
var (
	_ block.StoreOps           = (*PackStore)(nil)
	_ block.BatchSizer         = (*PackStore)(nil)
	_ packfile_store.Transport = (*rangeTransport)(nil)
)
