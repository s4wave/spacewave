//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/singleflight"
)

// coldReadLimit bounds the complete foreground cold lookup and each tail fill.
const coldReadLimit = 3 * time.Second

// errColdReadBusy is a cache miss when all demand range slots are in use.
var errColdReadBusy = errors.New("cold range read slots busy")

// errColdInvalidExtent reports bytes that disagree with the requested extent.
var errColdInvalidExtent = errors.New("invalid cold block extent")

// coldPackReadsOff is the process-wide rollback switch. Unconfigured stores
// construct no ColdPackStore, regardless of this setting.
var coldPackReadsOff atomic.Bool

// SetColdPackReads enables or disables lookup in every mounted cold pack set.
// A read that has already started can finish.
func SetColdPackReads(on bool) { coldPackReadsOff.Store(!on) }

// ColdPackStore serves an immutable, offline-verified pack set. It has only
// a range-read capability, no main PackStore or mutable bucket catalog. It
// belongs below the Flowgraph because a World must be readable to load one.
// A mount loads one immutable manifest. The LRU, negative cache and gone
// marks never outlive it; changing the manifest requires a new mount.
// A pack may leave the bucket only after a new manifest no longer names it
// and all old mounts are gone, or after its live keys are adopted into the
// main index and cold reads are off. An observed 404 immediately fences its
// cached evidence; caches cannot detect a deletion they have not observed.
type ColdPackStore struct {
	// le reports ignored cold errors.
	le *logrus.Entry
	// client reads ranges and cannot mutate or list objects.
	client ColdPackRangeReader
	// bucket owns the configured cold prefix.
	bucket string
	// opts names the immutable catalog and the dedicated promoted log.
	opts ColdPackOptions
	// cancel ends both background workers.
	cancel context.CancelFunc
	// done closes after preload and both workers release their resources.
	done chan struct{}
	// catalog stays nil during preload, after a failed preload, and on close.
	catalog atomic.Pointer[coldPackCatalog]
	// tails holds a bounded working set of verified index readers.
	tails *coldTailCache
	// negative remembers full digests after all matching tails miss.
	negative *coldCache[[sha256.Size]byte, struct{}]
	// ranges admits at most eight foreground range reads without queueing.
	ranges chan struct{}
	// reads deduplicates simultaneous reads of an exact block extent.
	reads singleflight.Group
	// bodyRanges remembers recent payload ranges, not their bytes.
	bodyRanges *coldCache[coldExtent, struct{}]
	// promotions owns the bounded log and detached promoter.
	promotions *coldPromotions
	// warnings carries best-effort error reporting off the read path.
	warnings chan coldWarning
	// stats accumulates counters for the admin readback.
	stats coldPackCounters
}

// NewColdPackStore starts Preload in the background. Until the catalog is
// ready, or if it fails to load, requests see a miss and never wait for it.
func NewColdPackStore(ctx context.Context, le *logrus.Entry, client ColdPackRangeReader, bucket string, opts ColdPackOptions) (*ColdPackStore, error) {
	// Require an explicit cold object prefix and independent local file paths.
	opts.Prefix = strings.TrimSuffix(opts.Prefix, "/") + "/"
	if client == nil || bucket == "" || !coldPackSubpath(opts.Prefix) || opts.ManifestFile == "" {
		return nil, errors.New("cold packs require a bucket, a dedicated prefix and a manifest")
	}
	if opts.PromoteCap < 0 || int64(opts.PromoteCap) > math.MaxInt64/coldPromotionBytes || opts.TailCacheBytes < 0 || opts.NegativeCacheKeys < 0 {
		return nil, errors.New("invalid cold pack cache or promotion limit")
	}
	if opts.PromotedLog != "" && filepath.Clean(opts.ManifestFile) == filepath.Clean(opts.PromotedLog) {
		return nil, errors.New("cold manifest and promoted log must be separate files")
	}

	// Allocate only in-memory scheduling and caches before serving.
	if opts.TailCacheBytes == 0 {
		opts.TailCacheBytes = 64 << 20
	}
	if opts.NegativeCacheKeys == 0 {
		opts.NegativeCacheKeys = 65536
	}
	if le == nil {
		le = logrus.NewEntry(logrus.StandardLogger())
	}

	// Start the workers under the mount’s lifetime, not a read scope.
	ctx, cancel := context.WithCancel(ctx)
	s := &ColdPackStore{
		le: le, client: client, bucket: bucket, opts: opts, cancel: cancel,
		done: make(chan struct{}), negative: newColdCache[[sha256.Size]byte, struct{}](opts.NegativeCacheKeys),
		ranges: make(chan struct{}, 8), warnings: make(chan coldWarning, 64),
		bodyRanges: newColdCache[coldExtent, struct{}](4096),
	}
	s.tails = newColdTailCache(s, opts.TailCacheBytes)
	s.promotions = newColdPromotions(s, opts.PromotedLog, opts.PromoteCap)
	go s.preload(ctx)
	return s, nil
}

// Close stops background work. This is a store-lifetime operation, never a
// read scope release or a write durability fence.
func (s *ColdPackStore) Close() {
	// Cancel the workers and wait for their own handles to be released.
	s.cancel()
	<-s.done
}

// GetHashType reports the only hash supported by a cold pack manifest.
func (s *ColdPackStore) GetHashType() hash.HashType { return hash.RecommendedHashType }

// GetSupportedFeatures reports no native write, root or flush features.
func (s *ColdPackStore) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeature_STORE_FEATURE_UNKNOWN
}

// BeginReadOperation returns the same tier without waiting for any background work.
func (s *ColdPackStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// PutBlock rejects writes to the immutable cold pack set.
func (s *ColdPackStore) PutBlock(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, ErrPackImmutable
}

// PutBlockBatch rejects writes to the immutable cold pack set.
func (s *ColdPackStore) PutBlockBatch(context.Context, []*block.PutBatchEntry) ([]bool, error) {
	return nil, ErrPackImmutable
}

// GetBlock returns only verified bytes from cold storage.
func (s *ColdPackStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	// Share the stored-block lookup, including its verification and promotion.
	stored, _, _, _ := s.lookup(ctx, ref, coldReadBlock)
	return stored.GetData(), stored != nil, nil
}

// GetStoredBlock returns a verified block and its outgoing refs.
func (s *ColdPackStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// The cold tier converts even unexpected errors to misses.
	stored, _, _, _ := s.lookup(ctx, ref, coldReadBlock)
	return stored, nil
}

// GetStoredBlockForMove treats an unresolved cold read as unavailable. A
// storage move must not switch placements if a cold-only block may be missing.
func (s *ColdPackStore) GetStoredBlockForMove(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Use the manifest indexes, not a promotion hint that may be unverified.
	stored, _, found, unresolved := s.lookup(ctx, ref, coldMoveBlock)
	if !found && unresolved {
		return nil, block.ErrUnavailable
	}
	return stored, nil
}

// GetBlockExists checks only exact, in-memory membership. It never waits for
// or issues a bucket request, including when a World writer holds its slot.
func (s *ColdPackStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	// An unconfirmed presence costs another put, not a skipped write.
	_, _, found, _ := s.lookup(ctx, ref, coldExists)
	return found, nil
}

// GetBlockExistsBatch checks exact, resident membership even for a mixed batch.
func (s *ColdPackStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// A miss or cold failure affects only that ref and never prevents a put.
	out := make([]bool, len(refs))
	for i, ref := range refs {
		_, _, out[i], _ = s.lookup(ctx, ref, coldExists)
	}
	return out, nil
}

// RmBlock rejects deletions from cold storage.
func (s *ColdPackStore) RmBlock(context.Context, *block.BlockRef) error { return ErrPackImmutable }

// StatBlock reports the decoded length after confirming the requested ref.
func (s *ColdPackStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	// Check the full identity before returning size or claiming presence.
	_, size, found, _ := s.lookup(ctx, ref, coldStat)
	if !found {
		return nil, nil
	}
	return &block.BlockStat{Ref: ref, Size: size}, nil
}

// Sync is not a durability fence: this tier accepts no writes. Promotion
// survives or is rebuilt independently, and no World commit waits for fsync.
func (s *ColdPackStore) Sync(context.Context) (bool, error) { return false, nil }

// GetRoot reports that the cold tier holds no roots.
func (s *ColdPackStore) GetRoot(context.Context, string) (*block.BlockRef, error) { return nil, nil }

// ListRoots reports no roots.
func (s *ColdPackStore) ListRoots(context.Context, string, func(block.NamedRoot) error) error {
	return nil
}

// SetRoots rejects writes and root changes on the immutable tier.
func (s *ColdPackStore) SetRoots(context.Context, []*block.PutBatchEntry, []block.NamedRoot) error {
	return ErrPackImmutable
}

// preload publishes the catalog after local recovery, then runs workers without
// any lock or resource that request paths need to acquire.
func (s *ColdPackStore) preload(ctx context.Context) {
	// Load and validate metadata before exposing any cold membership.
	defer close(s.done)
	catalog, err := s.loadCatalog()
	if err != nil {
		s.le.WithError(err).Warn("load cold pack manifest")
		return
	}

	// Treat the promoted log as optional; its failure never invalidates tails.
	log, err := s.promotions.open(s.opts.ManifestFile, catalog)
	if err != nil {
		s.le.WithError(err).Warn("recover cold promoted log")
	}

	// Take the initial warm list before publishing the immutable catalog.
	warm := s.promotions.warmPacks()
	if ctx.Err() == nil {
		s.catalog.Store(catalog)
	}
	defer s.catalog.Store(nil)

	// Admit fills independently of the promoter's local disk writes.
	var workers sync.WaitGroup
	workers.Go(func() { s.logWarnings(ctx) })
	workers.Go(func() { s.tails.fill(ctx, catalog, warm) })
	if log != nil {
		workers.Go(func() { s.promotions.run(ctx, log) })
	}
	workers.Wait()
}

// loadCatalog reads the single local manifest without listing the bucket.
func (s *ColdPackStore) loadCatalog() (*coldPackCatalog, error) {
	// Validate the complete immutable catalog before publication.
	data, err := os.ReadFile(s.opts.ManifestFile)
	if err != nil {
		return nil, err
	}
	manifest := &packfile.ColdPackManifest{}
	if err := manifest.UnmarshalVT(data); err != nil {
		return nil, err
	}
	return newColdPackCatalog(s.opts.Prefix, manifest)
}

// coldLookupMode says which evidence a read can obtain on the request path.
type coldLookupMode uint8

const (
	// coldExists uses memory only.
	coldExists coldLookupMode = iota
	// coldStat can read a tail or block when its decoded size is not yet known.
	coldStat
	// coldReadBlock reads and verifies the requested block body.
	coldReadBlock
	// coldMoveBlock distinguishes an unresolved source from a confirmed miss.
	coldMoveBlock
)

// lookup tries promoted extents, then intervals, bloom filters and exact tails.
// No cold error is returned to the caller or remembered as a negative result.
func (s *ColdPackStore) lookup(ctx context.Context, ref *block.BlockRef, mode coldLookupMode) (stored *block.StoredBlock, size int64, found, unresolved bool) {
	// Off, warming, closed, canceled, or using another hash: miss immediately.
	catalog := s.catalog.Load()
	h := ref.GetHash()
	if h.GetHashType() != hash.RecommendedHashType || len(h.GetHash()) != sha256.Size {
		return nil, 0, false, false
	}
	if coldPackReadsOff.Load() || catalog == nil || ctx.Err() != nil {
		return nil, 0, false, true
	}
	digest := [sha256.Size]byte(h.GetHash())

	// Bound lookup latency, including overlapping intervals and the exact read.
	if mode != coldExists {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, coldReadLimit)
		defer cancel()
	}

	// Normal reads can use promoted hints and exact negatives. Moves consult
	// the indexes again: a normal negative may have skipped a fenced pack.
	var promoted coldPromotion
	var ok bool
	if mode != coldMoveBlock {
		promoted, ok = s.promotions.get(digest)
		if ok {
			stored, size, found = s.readPromoted(ctx, catalog, ref, mode, digest, promoted)
			if found {
				s.stats.trueHits.Add(1)
				return stored, size, true, false
			}
		}
		if _, missing := s.negative.get(digest); missing {
			return nil, 0, false, false
		}
	}

	// Test only blooms that cover this full key.
	key := packfile.BlockKey(h)
	bkey := bloom.NewKey(key)
	var tested, uncached bool

	// Probe every covering interval, continuing after an exact miss or failure.
	catalog.candidates(key, func(pack uint16) bool {
		// A missing pack or known bad payload cannot justify skipping a put.
		candidate := &catalog.packs[pack]
		if candidate.unusable() {
			// A move cannot discard a block a fenced pack may have held.
			unresolved = unresolved || mode == coldMoveBlock && candidate.filter.TestKey(bkey)
			return true
		}
		if mode == coldExists && candidate.badPayload.Load() {
			unresolved = true // A body read can still verify a different extent.
			return true
		}

		// A bloom hit locates evidence to check; it never proves membership.
		s.stats.bloomProbes.Add(1)
		if !catalog.packs[pack].filter.TestKey(bkey) {
			return true
		}
		s.stats.bloomHits.Add(1)
		tested = true

		// On a put's existence path, load nothing and wait behind no fill.
		tail := s.tails.get(pack)
		if tail == nil && mode == coldExists {
			if s.tails.cacheable(catalog.packs[pack].meta.IndexTailBytes) {
				s.tails.queue(pack)
			}
			uncached, unresolved = true, true
			return true
		}
		if tail == nil {
			var err error
			tail, err = s.tails.read(ctx, catalog, pack)
			if err != nil {
				s.warn(ctx, err, "read cold pack index tail")
				unresolved = true
				return true
			}
		}

		// Locate the full index key; only a bound pack can prove presence.
		off, length, hit, err := tail.locate(key)
		if err != nil {
			s.warn(ctx, err, "search cold pack index tail")
			unresolved = true
			return true
		}
		if !hit {
			s.stats.falsePositives.Add(1)
			return true
		}
		if mode == coldExists {
			found = candidate.verifiedBytes.Load() && !candidate.unusable() && !candidate.badPayload.Load()
			unresolved = unresolved || !found
			return !found
		}

		// Return only the result of a verified block read, then queue its extent.
		extent := coldExtent{pack: pack, off: off, size: length}
		if ok && promoted.exact && promoted.digest == digest && promoted.extent == extent {
			unresolved = true // The read of this exact extent already failed.
			return true
		}
		stored, err = s.readExtent(ctx, catalog, ref, extent)
		if err != nil {
			if errors.Is(err, errColdInvalidExtent) {
				candidate.badPayload.Store(true)
			}
			s.warn(ctx, err, "read cold block extent")
			unresolved = true
			return true
		}
		size, found = int64(len(stored.Data)), true
		s.promotions.offer(digest, extent, size)
		return false
	})

	// Do not poison negative membership with a transient failure or queued fill.
	if found {
		s.stats.trueHits.Add(1)
	} else if tested && !unresolved && ctx.Err() == nil && mode != coldMoveBlock {
		s.negative.put(digest, struct{}{}, 1, false)
	}
	if uncached && !found {
		s.stats.uncachedExistenceMisses.Add(1)
	}
	return stored, size, found, unresolved || ctx.Err() != nil
}

// readPromoted distinguishes exact in-memory evidence from a restored hint.
func (s *ColdPackStore) readPromoted(
	ctx context.Context, catalog *coldPackCatalog, ref *block.BlockRef,
	mode coldLookupMode, digest [sha256.Size]byte, rec coldPromotion,
) (*block.StoredBlock, int64, bool) {
	// A random absent key has at most PromoteCap / 2^96 chance of matching
	// a suffix (about 6.3e-24 at 500,000). That match never answers yes: only
	// a full digest verified against the payload in this mount, or an exact
	// index key from a bound pack, proves membership. A restored hint requires
	// one verified extent read before it can answer existence from RAM.
	pack := &catalog.packs[rec.extent.pack]
	if pack.unusable() {
		return nil, 0, false
	}
	exact := rec.exact && rec.digest == digest
	if mode == coldExists || mode == coldStat && exact && !pack.badPayload.Load() {
		return nil, int64(rec.dataSize), exact && !pack.badPayload.Load()
	}

	// Treat a known different digest as a suffix collision, not a read target.
	if rec.exact && !exact {
		return nil, 0, false
	}
	stored, err := s.readExtent(ctx, catalog, ref, rec.extent)
	if err != nil {
		if exact && errors.Is(err, errColdInvalidExtent) {
			pack.badPayload.Store(true)
		}
		s.warn(ctx, err, "read promoted cold block extent")
		return nil, 0, false
	}

	// Recover the full digest and actual decoded size, not the log's hint.
	s.promotions.confirm(digest, rec.extent, uint32(len(stored.Data))) //nolint:gosec // Decoded data fits the record's uint32 encoded extent.
	return stored, int64(len(stored.Data)), true
}

// readExtent shares a single exact ranged GET, then verifies the requested
// SHA-256 before returning any block. It never retains or aliases caller refs.
func (s *ColdPackStore) readExtent(ctx context.Context, catalog *coldPackCatalog, ref *block.BlockRef, extent coldExtent) (*block.StoredBlock, error) {
	// Deduplicate only on the immutable pack range, not on a shortened hash.
	pack := &catalog.packs[extent.pack]
	ch := s.reads.DoChan(extent.flightKey(), func() (any, error) {
		// Bound shared work independently of either caller's wait.
		flight, cancel := context.WithTimeout(context.WithoutCancel(ctx), coldReadLimit)
		defer cancel()
		data, err := s.readRange(flight, pack, int64(extent.off), int(extent.size), false, false) //nolint:gosec // Catalog and tail bounds constrain off and size.
		if err != nil {
			return nil, err
		}

		// Count once per successful bucket GET, not once per joined waiter.
		if _, repeated := s.bodyRanges.get(extent); repeated {
			s.stats.repeatExtentReads.Add(1)
		} else {
			s.bodyRanges.put(extent, struct{}{}, 1, false)
		}
		return data, nil
	})
	var result singleflight.Result
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result = <-ch:
		if result.Err != nil {
			return nil, result.Err
		}
	}

	// Reject short or oversized responses before decoding and hashing.
	raw := result.Val.([]byte)
	if uint64(len(raw)) != extent.size {
		return nil, errors.Wrap(errColdInvalidExtent, "cold pack returned a wrong-sized extent")
	}

	// Recheck the gone fence after the shared request and SHA-256 verification.
	stored, err := verifiedBlock(ref, raw)
	if pack.unusable() {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, errors.Wrap(errColdInvalidExtent, err.Error())
	}
	return stored, nil
}

// readRange bounds demand I/O and fences membership after an authoritative
// size mismatch, checksum mismatch, or not-found response. Only the tail worker can make a background range read;
// its single slot is separate so it cannot displace a foreground reader.
func (s *ColdPackStore) readRange(ctx context.Context, pack *coldPack, off int64, size int, background, tail bool) ([]byte, error) {
	// Refuse fenced packs and do not wait to enter a saturated demand lane.
	if pack.unusable() {
		return nil, ErrNotFound
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !background {
		select {
		case s.ranges <- struct{}{}:
			defer func() { <-s.ranges }()
		default:
			return nil, errColdReadBusy
		}
	}

	// Compare the mirror's verified size and SHA-256 with the range response.
	// Size alone cannot catch equal-length damage. Without a whole-object
	// checksum, only individually verified extents authorize presence. A
	// cached proof still cannot detect an unobserved replacement or deletion.
	if tail {
		s.stats.tailFetches.Add(1)
	}
	data, sum, err := s.client.GetColdPackRange(ctx, s.bucket, pack.key, off, size, pack.meta.Entry.SizeBytes)
	if err == nil && len(sum) != 0 {
		if bytes.Equal(sum, pack.meta.PackSha256) {
			pack.verifiedBytes.Store(true)
		} else {
			err = errColdObjectMismatch
		}
	}

	// The first authoritative mismatch or miss fences even cached evidence.
	if errors.Is(err, ErrNotFound) && pack.gone.CompareAndSwap(false, true) {
		s.offerWarning(coldWarning{key: pack.key, message: "cold pack is gone; ignoring its cached entries"})
	}
	if errors.Is(err, errColdObjectMismatch) && pack.wrongObject.CompareAndSwap(false, true) {
		s.offerWarning(coldWarning{key: pack.key, message: "cold pack size or digest changed; ignoring its cached entries"})
	}
	if pack.unusable() {
		return nil, ErrNotFound
	}
	return data, err
}

// coldWarning carries diagnostics without letting a log sink stall a read.
type coldWarning struct {
	// err is an unexpected storage or integrity failure, if any.
	err error
	// key is the gone pack's full object key, when applicable.
	key string
	// message is the storage operation that failed.
	message string
}

// warn records an unexpected cold failure without passing it to the next tier.
func (s *ColdPackStore) warn(ctx context.Context, err error, message string) {
	// Cancellation, saturation, and already reported missing packs are misses.
	if !errors.Is(err, ErrNotFound) && !errors.Is(err, errColdReadBusy) && !errors.Is(err, context.Canceled) && ctx.Err() != context.Canceled {
		s.offerWarning(coldWarning{err: err, message: message})
	}
}

// offerWarning admits a bounded warning without waiting on the logging sink.
func (s *ColdPackStore) offerWarning(warning coldWarning) {
	select {
	case s.warnings <- warning:
	default:
	}
}

// logWarnings drains diagnostic events independently of storage work.
func (s *ColdPackStore) logWarnings(ctx context.Context) {
	for ctx.Err() == nil {
		// The sink is never called under a cache lock or on a read path.
		select {
		case <-ctx.Done():
			return
		case event := <-s.warnings:
			le := s.le
			if event.err != nil {
				le = le.WithError(event.err)
			}
			if event.key != "" {
				le = le.WithField("pack", event.key)
			}
			le.Warn(event.message)
		}
	}
}

// _ is a type assertion.
var _ block.StoreOps = (*ColdPackStore)(nil)
