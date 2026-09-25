package provider_spacewave

import (
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	packfile_order "github.com/s4wave/spacewave/db/packfile/order"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

const syncPushRetryTimeout = 30 * time.Second

const syncNoProgressBackoff = time.Second

// syncPendingSinceKey retains the first dirty-block deadline across restart.
const syncPendingSinceKey = "sync/pending-since"

const defaultSyncSizeThresholdBytes = 48 * 1024 * 1024

const syncOrderDirtyBlocksLimit = 1024

// syncFlushMaxPackBytes is the browser sync target for one upload body. The
// wire cap remains writer.DefaultMaxPackBytes, but TinyGo browser builds need
// enough headroom for the pack writer, request body, foreground uploads, OPFS,
// and the full app runtime.
const syncFlushMaxPackBytes int64 = 4 * 1024 * 1024

// syncController manages packfile push/pull synchronization.
type syncController struct {
	le                *logrus.Entry
	store             kvtx.Store
	client            *SessionClient
	resourceID        string
	mfst              *manifest.Manifest
	lower             *packfile_store.PackfileStore
	remote            func() []*packfile.PackfileEntry
	upper             block.StoreOps
	refGraph          packfile_order.RefGraph
	conf              *SyncConfig
	telemetry         *ProviderAccount
	gateBcast         *broadcast.Broadcast
	skipPull          bool
	remotePullRoutine *coalescedTriggerRoutine

	// dirtySize and dirtyPendingAt project durable dirty records under bcast.
	dirtySize      int64
	dirtyPendingAt time.Time
	// publications tracks mounted hosts with durable pending cloud work.
	publications map[*cloudSOHost]time.Time
	// bcast wakes the scheduler when the durable dirty queue changes.
	bcast broadcast.Broadcast
	// dirtyMtx orders durable dirty mutations and their in-memory projection.
	dirtyMtx csync.Mutex

	// flushMtx serializes foreground and background flush operations.
	flushMtx sync.Mutex
	// compactWaitPull holds merges after a failed merge until a pull refreshes
	// the manifest, so a stale plan is never retried. Guarded by flushMtx.
	compactWaitPull bool
}

// Init recalculates the dirty size and runs the initial pull. Access-gated pull
// failures are returned so callers can wait for account/resource invalidation
// instead of retrying. Must be called before Execute.
func (s *syncController) Init(ctx context.Context) error {
	if err := s.recalcDirtySize(ctx); err != nil {
		return err
	}

	if s.skipPull {
		s.lower.UpdateManifest(s.mergedManifestEntries())
		return nil
	}

	if err := s.pull(ctx); err != nil {
		if isCloudAccessGatedError(err) {
			s.le.WithError(err).Warn("initial pull gated, stopping sync")
			return err
		}
		s.le.WithError(err).Warn("initial pull failed")
	}
	return nil
}

func (s *syncController) mergedManifestEntries() []*packfile.PackfileEntry {
	local := s.mfst.GetEntries()
	if s.remote == nil {
		return local
	}
	remote := s.remote()
	if len(remote) == 0 {
		return local
	}
	seen := make(map[string]bool, len(remote)+len(local))
	out := make([]*packfile.PackfileEntry, 0, len(remote)+len(local))
	for _, entry := range remote {
		id := entry.GetId()
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, entry)
	}
	for _, entry := range local {
		id := entry.GetId()
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, entry)
	}
	return out
}

// pendingSnapshot reads the durable queue projection and its notification together.
func (s *syncController) pendingSnapshot() (time.Time, int64, <-chan struct{}) {
	var first time.Time
	var dirty int64
	var changed <-chan struct{}
	s.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
		first, dirty, changed = s.dirtyPendingAt, s.dirtySize, getWait()
		for _, pendingAt := range s.publications {
			if first.IsZero() || pendingAt.Before(first) {
				first = pendingAt
			}
		}
	})
	return first, dirty, changed
}

// Execute dispatches from the first pending change, without extending its deadline.
func (s *syncController) Execute(ctx context.Context) error {
	if s.remotePullRoutine != nil && !s.skipPull {
		s.remotePullRoutine.SetContext(ctx)
		defer s.remotePullRoutine.ClearContext()
	}

	// Keep the existing pressure and pack limits independent of the time boundary.
	bo := providerBackoff.Construct()
	threshold := int64(s.conf.GetSizeThresholdBytes())
	if threshold == 0 {
		threshold = defaultSyncSizeThresholdBytes
	}
	interval := time.Duration(s.conf.GetCheckpointIntervalSecs()) * time.Second
	if interval == 0 {
		interval = 30 * time.Second
	}

	for ctx.Err() == nil {
		first, dirty, changed := s.pendingSnapshot()
		if first.IsZero() && dirty < threshold {
			bo.Reset()
			select {
			case <-ctx.Done():
				return nil
			case <-changed:
				continue
			}
		}

		// A later edit wakes this wait but keeps the original persisted deadline.
		if delay := time.Until(first.Add(interval)); dirty < threshold && delay > 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-changed:
				continue
			case <-time.After(delay):
			}
		}

		if err := s.FlushNow(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			if isDirtySyncGatedCloudError(err) {
				bo.Reset()
				s.le.WithError(err).Warn("block upload gated, waiting for account state change")
				if err := s.waitDirtySyncGate(ctx); err != nil {
					return nil
				}
				continue
			}
			s.le.WithError(err).Warn("block upload failed")
			_, _, changed = s.pendingSnapshot()
			if err := waitDirtySyncRetry(ctx, changed, nextProviderRetryDelay(bo, err)); err != nil {
				return nil
			}
			continue
		}

		// A flush with no progress must not spin on an expired deadline.
		bo.Reset()
		next, _, changed := s.pendingSnapshot()
		if next.IsZero() && s.conf.GetCompactSmallPacks() {
			if err := s.CompactNow(ctx); err != nil && ctx.Err() == nil {
				s.le.WithError(err).Warn("small pack merge failed")
			}
		}
		if !next.IsZero() && !next.After(first) {
			if err := waitDirtySyncRetry(ctx, changed, syncNoProgressBackoff); err != nil {
				return nil
			}
		}
	}
	return nil
}

func waitDirtySyncRetry(ctx context.Context, ch <-chan struct{}, delay time.Duration) error {
	if delay == cbackoff.Stop {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
			return nil
		}
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
		return nil
	case <-time.After(delay):
		return nil
	}
}

func (s *syncController) waitDirtySyncGate(ctx context.Context) error {
	for {
		first, dirty, dirtyCh := s.pendingSnapshot()
		if first.IsZero() && dirty == 0 {
			return nil
		}
		if s.gateBcast == nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-dirtyCh:
				continue
			}
		}

		var gateCh <-chan struct{}
		s.gateBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			gateCh = getWaitCh()
		})
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-dirtyCh:
		case <-gateCh:
			return nil
		}
	}
}

// FlushNow serializes an immediate flush request.
func (s *syncController) FlushNow(ctx context.Context) error {
	s.flushMtx.Lock()
	defer s.flushMtx.Unlock()
	return s.flushCheckpoint(ctx, true)
}

// FlushNowUnordered flushes dirty blocks without refgraph locality ordering.
func (s *syncController) FlushNowUnordered(ctx context.Context) error {
	s.flushMtx.Lock()
	defer s.flushMtx.Unlock()
	return s.flushCheckpoint(ctx, false)
}

// PullNow serializes an immediate remote packfile manifest pull.
func (s *syncController) PullNow(ctx context.Context) error {
	s.flushMtx.Lock()
	defer s.flushMtx.Unlock()
	if s.skipPull {
		s.lower.UpdateManifest(s.mergedManifestEntries())
		return nil
	}
	return s.pull(ctx)
}

// LastPullSequence returns the local manifest's last-seen remote sequence.
func (s *syncController) LastPullSequence(ctx context.Context) (uint64, error) {
	return s.mfst.GetLastPullSequence(ctx)
}

// TriggerRemotePull coalesces a remote block-store nonce notification into one pull.
func (s *syncController) TriggerRemotePull() {
	if s.remotePullRoutine != nil && !s.skipPull {
		s.remotePullRoutine.Trigger()
	}
}

func (s *syncController) pullRemoteOnTrigger(ctx context.Context) {
	if err := s.PullNow(ctx); err != nil && ctx.Err() == nil {
		s.le.WithError(err).Warn("remote nonce pull failed")
		s.recordSyncOwnerError(err)
	}
}

// pushPackfile pushes a packfile and retries the same pack ID once when the
// request was canceled after the worker accepted it.
func (s *syncController) pushPackfile(
	ctx context.Context,
	packID string,
	blockCount int,
	pushFn func(context.Context, string, int) error,
) error {
	err := pushFn(ctx, packID, blockCount)
	if err == nil {
		return nil
	}
	if !isRetryableSyncPushCancel(err) {
		return err
	}

	retryCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		syncPushRetryTimeout,
	)
	defer cancel()

	// note: this fork of logrus dereferences the entry, so le must be set.
	if s.le != nil {
		s.le.WithField("pack-id", packID).
			Debug("retrying canceled sync push with detached context")
	}
	return pushFn(retryCtx, packID, blockCount)
}

// MarkDirty durably schedules written blocks before their caller can
// acknowledge the writes. Repeating a write repairs a failed marker without
// double-counting pending bytes.
func (s *syncController) MarkDirty(ctx context.Context, marks []block_store_writeback.Mark) error {
	release, err := s.dirtyMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// The markers and first-pending timestamp share the metadata transaction.
	var addedBytes int64
	var first time.Time
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) { return s.store.NewTransaction(ctx, true) },
		func(ctx context.Context, tx kvtx.Tx) error {
			addedBytes = 0
			for _, mark := range marks {
				key := []byte("dirty/" + mark.Hash.MarshalString())
				_, found, err := tx.Get(ctx, key)
				if err != nil {
					return err
				}
				if found {
					continue
				}
				if err := tx.Set(ctx, key, []byte(strconv.FormatInt(mark.Size, 10))); err != nil {
					return err
				}
				addedBytes += mark.Size
			}
			first, err = readDirtyPendingTime(ctx, tx)
			return err
		},
	)
	if err != nil {
		return errors.Wrap(err, "retain pending block")
	}

	// Publish only the committed projection, ordered with startup and cleanup scans.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.dirtySize += addedBytes
		s.dirtyPendingAt = first
		broadcast()
	})
	if addedBytes != 0 {
		s.telemetrySafeCall(func(t *ProviderAccount, id string) { t.addSyncTelemetryDirty(id, addedBytes) })
	}
	return nil
}

// readDirtyPendingTime retains the first dirty time for current and preexisting work.
// The caller holds a writable metadata transaction containing at least one marker.
func readDirtyPendingTime(ctx context.Context, tx kvtx.Tx) (time.Time, error) {
	value, found, err := tx.Get(ctx, []byte(syncPendingSinceKey))
	if err != nil {
		return time.Time{}, err
	}
	if found {
		return time.Parse(time.RFC3339Nano, string(value))
	}
	first := time.Now().UTC()
	return first, tx.Set(ctx, []byte(syncPendingSinceKey), []byte(first.Format(time.RFC3339Nano)))
}

// recalcDirtySize reconciles the durable queue and clears its deadline only when empty.
func (s *syncController) recalcDirtySize(ctx context.Context, flushed ...dirtyCandidate) error {
	release, err := s.dirtyMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Startup and successful cleanup use one transaction for count and deadline.
	var total int64
	var count int
	var first time.Time
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) { return s.store.NewTransaction(ctx, true) },
		func(ctx context.Context, tx kvtx.Tx) error {
			total, count, first = 0, 0, time.Time{}
			for _, candidate := range flushed {
				if err := tx.Delete(ctx, candidate.key); err != nil {
					return err
				}
			}
			if err := tx.ScanPrefix(ctx, []byte("dirty/"), func(_, value []byte) error {
				total += parseDirtySize(value)
				count++
				return nil
			}); err != nil {
				return err
			}
			if count == 0 {
				return tx.Delete(ctx, []byte(syncPendingSinceKey))
			}
			var err error
			first, err = readDirtyPendingTime(ctx, tx)
			return err
		},
	)
	if err != nil {
		return errors.Wrap(err, "read pending blocks")
	}

	// Readers observe counts and their matching timer together.
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.dirtySize, s.dirtyPendingAt = total, first
		broadcast()
	})
	s.telemetrySafeCall(func(t *ProviderAccount, id string) { t.setSyncTelemetryPending(id, total, count) })
	return nil
}

// dirtyCandidate is the metadata needed to decide which dirty blocks belong in
// a flush chunk without loading block data into memory.
type dirtyCandidate struct {
	key  []byte
	hash *hash.Hash
	size int64
}

// dirtyBlock holds one loaded block with its refs for the currently packed
// flush chunk.
type dirtyBlock struct {
	dirtyCandidate
	stored *block.StoredBlock
}

// preparedSyncChunk is one packed chunk ready to push.
type preparedSyncChunk struct {
	// blocks are the dirty blocks packed into packData. Empty for a merge.
	blocks []dirtyBlock
	// replaces are the committed packs a merge supersedes.
	replaces []string
	// entry is the manifest entry describing the pack.
	entry *packfile.PackfileEntry
	// packData is the encoded pack body.
	packData []byte
	// bodyHash is the SHA-256 digest of packData.
	bodyHash []byte
}

// packBlocks writes the dirty blocks to w and returns the pack result.
func (s *syncController) packBlocks(w io.Writer, blocks []dirtyBlock) (*writer.PackResult, error) {
	idx := 0
	iter := func() (*hash.Hash, *block.StoredBlock, error) {
		if idx >= len(blocks) {
			return nil, nil, nil
		}
		b := blocks[idx]
		idx++
		return b.hash, b.stored, nil
	}

	result, err := writer.PackBlocks(w, iter)
	if err != nil {
		return nil, errors.Wrap(err, "packing blocks")
	}
	return result, nil
}

// cleanupDirtyCandidates removes acknowledged markers and resets an empty queue's deadline atomically.
func (s *syncController) cleanupDirtyCandidates(ctx context.Context, blocks []dirtyCandidate) error {
	return s.recalcDirtySize(ctx, blocks...)
}

// orderDirtyBlocks orders dirty block metadata for pack locality before
// loading data.
func (s *syncController) orderDirtyBlocks(ctx context.Context, blocks []dirtyCandidate) ([]dirtyCandidate, error) {
	refs := make([]*block.BlockRef, 0, len(blocks))
	byKey := make(map[string]dirtyCandidate, len(blocks))
	for _, b := range blocks {
		key := b.hash.MarshalString()
		byKey[key] = b
		refs = append(refs, block.NewBlockRef(b.hash))
	}

	orderedRefs, err := packfile_order.BlockRefs(ctx, s.refGraph, refs)
	if err != nil {
		return nil, err
	}
	ordered := make([]dirtyCandidate, 0, len(orderedRefs))
	for _, ref := range orderedRefs {
		b, ok := byKey[ref.GetHash().MarshalString()]
		if ok {
			ordered = append(ordered, b)
		}
	}
	return ordered, nil
}

func (s *syncController) filterDuplicateDirtyBlocks(ctx context.Context, blocks []dirtyCandidate) ([]dirtyCandidate, []dirtyCandidate, error) {
	refs := make([]*block.BlockRef, 0, len(blocks))
	for _, b := range blocks {
		refs = append(refs, block.NewBlockRef(b.hash))
	}
	exists, err := s.lower.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return nil, nil, err
	}

	pack := make([]dirtyCandidate, 0, len(blocks))
	deduped := make([]dirtyCandidate, 0)
	for i, b := range blocks {
		if exists[i] {
			deduped = append(deduped, b)
			continue
		}
		pack = append(pack, b)
	}
	if len(deduped) != 0 {
		var bytes int64
		for _, b := range deduped {
			bytes += b.size
		}
		s.le.WithField("dirty-blocks", len(blocks)).
			WithField("deduped-blocks", len(deduped)).
			WithField("deduped-bytes", bytes).
			Debug("filtered duplicate dirty blocks")
		s.telemetrySafeCall(func(t *ProviderAccount, id string) {
			t.addSyncTelemetryDeduped(id, bytes, len(deduped))
		})
	}
	return pack, deduped, nil
}

func (s *syncController) scanDirtyCandidates(ctx context.Context) ([]dirtyCandidate, error) {
	var candidates []dirtyCandidate
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return s.store.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			attemptCandidates := make([]dirtyCandidate, 0)
			err := tx.ScanPrefix(ctx, []byte("dirty/"), func(k, v []byte) error {
				keyCopy := make([]byte, len(k))
				copy(keyCopy, k)
				hashStr := string(k[len("dirty/"):])
				h := &hash.Hash{}
				if err := h.ParseFromB58(hashStr); err != nil {
					return errors.Wrap(err, "parsing dirty hash key")
				}
				attemptCandidates = append(attemptCandidates, dirtyCandidate{
					key:  keyCopy,
					hash: h,
					size: parseDirtySize(v),
				})
				return nil
			})
			if err == nil {
				candidates = attemptCandidates
			}
			return err
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "scanning dirty keys")
	}
	return candidates, nil
}

func parseDirtySize(v []byte) int64 {
	size, err := strconv.ParseInt(string(v), 10, 64)
	if err != nil || size < 0 {
		return 0
	}
	return size
}

func dirtyCandidateChunkSize(size int64) int64 {
	if size <= 0 {
		return syncFlushMaxPackBytes
	}
	return size
}

func nextDirtyCandidateChunk(blocks []dirtyCandidate, start int, maxChunkBytes int64, maxChunkBlocks int) (int, error) {
	if maxChunkBytes <= 0 {
		maxChunkBytes = syncFlushMaxPackBytes
	}
	var chunkBytes int64
	end := start
	for end < len(blocks) {
		size := dirtyCandidateChunkSize(blocks[end].size)
		if size > writer.DefaultMaxPackBytes {
			return 0, errors.Errorf(
				"dirty block %s exceeds max pack chunk size",
				blocks[end].hash.MarshalString(),
			)
		}
		if maxChunkBlocks > 0 && end-start >= maxChunkBlocks {
			break
		}
		if chunkBytes > 0 && chunkBytes+size > maxChunkBytes {
			break
		}
		chunkBytes += size
		end++
	}
	if end == start {
		end++
	}
	return end, nil
}

func (s *syncController) loadDirtyBlocks(ctx context.Context, candidates []dirtyCandidate) ([]dirtyBlock, error) {
	blocks := make([]dirtyBlock, 0, len(candidates))
	for _, candidate := range candidates {
		ref := block.NewBlockRef(candidate.hash)
		stored, err := s.upper.GetStoredBlock(ctx, ref)
		if err != nil {
			return nil, errors.Wrap(err, "getting dirty block")
		}
		if stored == nil {
			return nil, errors.Wrap(block.ErrNotFound, candidate.hash.MarshalString())
		}
		if int64(len(stored.Data)) > writer.DefaultMaxPackBytes {
			return nil, errors.Errorf(
				"dirty block %s exceeds max pack chunk size",
				candidate.hash.MarshalString(),
			)
		}
		blocks = append(blocks, dirtyBlock{
			dirtyCandidate: candidate,
			stored:         stored,
		})
	}
	return blocks, nil
}

// flushLoadedBlocks packs and pushes one chunk of loaded dirty blocks, halving
// chunks that exceed the sync pack target, and commits each pushed pack.
func (s *syncController) flushLoadedBlocks(ctx context.Context, blocks []dirtyBlock) error {
	chunk, err := s.prepareFlushChunk(blocks)
	if err != nil {
		return err
	}
	if chunk == nil {
		return nil
	}
	if int64(len(chunk.packData)) > syncFlushMaxPackBytes && len(blocks) > 1 {
		chunk = nil
		mid := len(blocks) / 2
		if err := s.flushLoadedBlocks(ctx, blocks[:mid]); err != nil {
			return err
		}
		return s.flushLoadedBlocks(ctx, blocks[mid:])
	}
	if int64(len(chunk.packData)) > writer.DefaultMaxPackBytes {
		return errors.Errorf(
			"dirty pack %s exceeds max pack size",
			chunk.entry.GetId(),
		)
	}
	if int64(len(chunk.packData)) > syncFlushMaxPackBytes {
		s.le.WithField("pack-id", chunk.entry.GetId()).
			WithField("bytes", len(chunk.packData)).
			WithField("target", syncFlushMaxPackBytes).
			Debug("single-block sync pack exceeds browser target")
	}
	if err := s.pushPreparedChunk(ctx, chunk); err != nil {
		return err
	}
	return s.commitPushedChunk(ctx, chunk)
}

// commitPushedChunk records a pushed pack in the manifest and clears the dirty
// markers of its blocks, so a later failure in the same flush does not push the
// same blocks again under a different pack.
func (s *syncController) commitPushedChunk(ctx context.Context, chunk *preparedSyncChunk) error {
	if err := s.mfst.ApplyDelta(ctx, []*packfile.PackfileEntry{chunk.entry}, nil); err != nil {
		return errors.Wrap(err, "applying push delta")
	}
	s.lower.UpdateManifest(s.mergedManifestEntries())

	flushed := make([]dirtyCandidate, len(chunk.blocks))
	for i, block := range chunk.blocks {
		flushed[i] = block.dirtyCandidate
	}
	return s.cleanupDirtyCandidates(ctx, flushed)
}

// prepareFlushChunk packs one bounded dirty-block chunk.
func (s *syncController) prepareFlushChunk(blocks []dirtyBlock) (*preparedSyncChunk, error) {
	var buf bytes.Buffer
	started := time.Now()
	result, err := s.packBlocks(&buf, blocks)
	s.le.WithField("blocks", len(blocks)).
		WithField("duration", time.Since(started)).
		Debug("packed dirty blocks")
	if err != nil {
		return nil, err
	}
	if result.BlockCount == 0 {
		return nil, nil
	}
	packID, err := identity.BuildPackID(s.resourceID, result)
	if err != nil {
		return nil, errors.Wrap(err, "build pack id")
	}

	entry := &packfile.PackfileEntry{
		Id:                 packID,
		BloomFilter:        result.BloomFilter,
		BloomFormatVersion: packfile.BloomFormatVersionV1,
		BlockCount:         result.BlockCount,
		SizeBytes:          result.BytesWritten,
		CreatedAt:          timestamppb.New(time.Now().UTC()),
	}
	return &preparedSyncChunk{
		blocks:   blocks,
		entry:    entry,
		packData: buf.Bytes(),
		bodyHash: result.PackBytesDigest,
	}, nil
}

// pushPreparedChunk uploads one prepared packfile chunk.
func (s *syncController) pushPreparedChunk(ctx context.Context, chunk *preparedSyncChunk) error {
	entry := chunk.entry
	pushBytes := int64(len(chunk.packData))
	s.telemetrySafeCall(func(t *ProviderAccount, id string) {
		t.startSyncTelemetryPush(id, pushBytes)
	})
	started := time.Now()
	err := s.pushPackfile(
		ctx,
		entry.GetId(),
		int(entry.GetBlockCount()), //nolint:gosec // prepared entries are produced under writer.DefaultMaxBlocksPerPack (4096).
		func(ctx context.Context, packID string, blockCount int) error {
			return s.client.syncPushDataWithProgress(
				ctx,
				s.resourceID,
				&syncPushPack{
					packID:             packID,
					blockCount:         blockCount,
					bodyHash:           chunk.bodyHash,
					bloomFilter:        entry.GetBloomFilter(),
					bloomFormatVersion: entry.GetBloomFormatVersion(),
					replacedPackIDs:    chunk.replaces,
				},
				chunk.packData,
				func(sent int64) {
					s.telemetrySafeCall(func(t *ProviderAccount, id string) {
						t.setSyncTelemetryPushProgress(id, sent)
					})
				},
			)
		},
	)
	s.le.WithField("pack-id", entry.GetId()).
		WithField("blocks", entry.GetBlockCount()).
		WithField("bytes", len(chunk.packData)).
		WithField("duration", time.Since(started)).
		Debug("pushed packfile")
	s.telemetrySafeCall(func(t *ProviderAccount, id string) {
		t.finishSyncTelemetryPush(id, pushBytes, err)
	})
	if err != nil {
		return errors.Wrap(err, "pushing packfile")
	}

	s.le.WithField("pack-id", entry.GetId()).
		WithField("blocks", entry.GetBlockCount()).
		Debug("flushed packfile")
	return nil
}

// flush collects dirty blocks, packs them, pushes to the server, and updates the manifest.
func (s *syncController) flush(ctx context.Context, orderBlocks bool) error {
	blocks, err := s.scanDirtyCandidates(ctx)
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		return s.recalcDirtySize(ctx)
	}

	s.le.WithField("dirty-blocks", len(blocks)).
		WithField("order-blocks", orderBlocks).
		WithField("max-chunk-bytes", syncFlushMaxPackBytes).
		Debug("starting dirty block flush")

	blocks, dedupedBlocks, err := s.filterDuplicateDirtyBlocks(ctx, blocks)
	if err != nil {
		return errors.Wrap(err, "filtering duplicate dirty blocks")
	}
	if len(blocks) == 0 {
		return s.cleanupDirtyCandidates(ctx, dedupedBlocks)
	}

	if orderBlocks && len(blocks) > syncOrderDirtyBlocksLimit {
		s.le.WithField("dirty-blocks", len(blocks)).
			WithField("limit", syncOrderDirtyBlocksLimit).
			Debug("skipping dirty block ordering")
		orderBlocks = false
	}

	if orderBlocks {
		started := time.Now()
		blocks, err = s.orderDirtyBlocks(ctx, blocks)
		s.le.WithField("dirty-blocks", len(blocks)).
			WithField("duration", time.Since(started)).
			Debug("ordered dirty blocks")
		if err != nil {
			return errors.Wrap(err, "ordering dirty blocks")
		}
	}

	// Clear blocks the cloud already holds before pushing the rest.
	if len(dedupedBlocks) != 0 {
		if err := s.cleanupDirtyCandidates(ctx, dedupedBlocks); err != nil {
			return err
		}
	}

	maxChunkBlocks := int(writer.DefaultPolicy().MaxBlocksPerPack) //nolint:gosec // the built-in policy caps this at 4096 blocks.
	for start := 0; start < len(blocks); {
		end, err := nextDirtyCandidateChunk(blocks, start, syncFlushMaxPackBytes, maxChunkBlocks)
		if err != nil {
			return err
		}
		loadedBlocks, err := s.loadDirtyBlocks(ctx, blocks[start:end])
		if err != nil {
			return err
		}
		if err := s.flushLoadedBlocks(ctx, loadedBlocks); err != nil {
			return err
		}
		start = end
	}
	return nil
}

// pull fetches new packfile entries from the server since the last pull.
func (s *syncController) pull(ctx context.Context) error {
	lastSeq, err := s.mfst.GetLastPullSequence(ctx)
	if err != nil {
		return errors.Wrap(err, "getting last pull sequence")
	}
	since := ""
	if lastSeq != 0 {
		since = strconv.FormatUint(lastSeq, 10)
	}

	s.telemetrySafeCall(func(t *ProviderAccount, id string) {
		t.startSyncTelemetryPull(id)
	})
	respData, err := s.client.SyncPull(ctx, s.resourceID, since)
	s.telemetrySafeCall(func(t *ProviderAccount, id string) {
		t.finishSyncTelemetryPull(id, err)
	})
	if err != nil {
		return errors.Wrap(err, "pulling from server")
	}

	s.compactWaitPull = false
	if len(respData) == 0 {
		return nil
	}

	resp := &packfile.PullResponse{}
	if err := resp.UnmarshalVT(respData); err != nil {
		return errors.Wrap(err, "unmarshaling pull response")
	}

	entries := resp.GetEntries()
	events := resp.GetReplacementEvents()
	latestSequence := resp.GetLatestSequence()
	if len(entries) == 0 && len(events) == 0 {
		if latestSequence > lastSeq {
			if err := s.mfst.SetLastPullSequence(ctx, latestSequence); err != nil {
				return errors.Wrap(err, "recording empty pull sequence")
			}
		}
		s.recordSyncTelemetryRemoteSequence(latestSequence)
		return nil
	}

	if err := s.mfst.ApplyDelta(ctx, entries, events); err != nil {
		return errors.Wrap(err, "applying pull delta")
	}
	if latestSequence > lastSeq {
		if err := s.mfst.SetLastPullSequence(ctx, latestSequence); err != nil {
			return errors.Wrap(err, "recording pull sequence")
		}
	}
	s.lower.UpdateManifest(s.mergedManifestEntries())
	s.recordSyncTelemetryRemoteSequence(latestSequence)

	s.le.WithField("entries", len(entries)).
		WithField("replacement-events", len(events)).
		Debug("pulled packfile delta")
	return nil
}

func (s *syncController) recordSyncTelemetryRemoteSequence(sequence uint64) {
	s.telemetrySafeCall(func(t *ProviderAccount, id string) {
		t.setSyncTelemetryCloudRemoteSequence(id, sequence)
	})
}

// telemetrySafeCall invokes fn against the attached telemetry account when
// telemetry is enabled. The shared resource id and account handle are bound
// so call sites read as a single line per telemetry event.
func (s *syncController) telemetrySafeCall(fn func(t *ProviderAccount, resourceID string)) {
	if s.telemetry == nil {
		return
	}
	fn(s.telemetry, s.resourceID)
}

func (s *syncController) recordSyncOwnerError(err error) {
	s.telemetrySafeCall(func(t *ProviderAccount, id string) {
		t.recordSyncTelemetryError(id, err)
	})
}

func isRetryableSyncPushCancel(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "deadline exceeded")
}

// _ is a type assertion
