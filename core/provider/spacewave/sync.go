package provider_spacewave

import (
	"bytes"
	"context"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/identity"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	packfile_order "github.com/s4wave/spacewave/core/provider/spacewave/packfile/order"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

const syncPushRetryTimeout = 30 * time.Second

const syncNoProgressBackoff = time.Second

const defaultSyncSizeThresholdBytes = 48 * 1024 * 1024

const syncOrderDirtyBlocksLimit = 1024

const syncPushConcurrency = 1

const syncPushProgressInterval = 250 * time.Millisecond

// syncController manages packfile push/pull synchronization.
type syncController struct {
	le         *logrus.Entry
	store      kvtx.Store
	client     *SessionClient
	resourceID string
	mfst       *manifest.Manifest
	lower      *packfile_store.PackfileStore
	remote     func() []*packfile.PackfileEntry
	upper      block.StoreOps
	refGraph   packfile_order.RefGraph
	conf       *SyncConfig
	tmpDir     string
	telemetry  *ProviderAccount
	gateBcast  *broadcast.Broadcast
	skipPull   bool

	// dirtySize is guarded by bcast.
	dirtySize int64
	bcast     broadcast.Broadcast

	// flushMtx serializes foreground and background flush operations.
	flushMtx sync.Mutex
}

// Init performs the initial setup: clean stale temp files, recalculate dirty
// size, and run the initial pull. Access-gated pull failures are returned so
// callers can wait for account/resource invalidation instead of retrying.
// Must be called before Execute.
func (s *syncController) Init(ctx context.Context) error {
	s.cleanStaleTempFiles()
	s.recalcDirtySize(ctx)

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

// Execute runs the sync controller loop.
func (s *syncController) Execute(ctx context.Context) error {
	bo := providerBackoff.Construct()
	threshold := int64(s.conf.GetSizeThresholdBytes())
	if threshold == 0 {
		threshold = defaultSyncSizeThresholdBytes
	}

	timeout := time.Duration(s.conf.GetInactivityTimeoutSecs()) * time.Second
	if timeout == 0 {
		timeout = 10 * time.Second
	}

	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		var ch <-chan struct{}
		var dirty int64
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			dirty = s.dirtySize
		})

		if dirty >= threshold {
			if err := s.FlushNow(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				if isDirtySyncGatedCloudError(err) {
					bo.Reset()
					s.le.WithError(err).Warn("flush gated, waiting for account state change")
					if err := s.waitDirtySyncGate(ctx); err != nil {
						return nil
					}
					continue
				}
				s.le.WithError(err).Warn("flush failed")
				delay := nextProviderRetryDelay(bo, err)
				s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
					ch = getWaitCh()
				})
				if err := waitDirtySyncRetry(ctx, ch, delay); err != nil {
					return nil
				}
			} else {
				bo.Reset()
				var nextDirty int64
				s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
					nextDirty = s.dirtySize
				})
				if nextDirty >= dirty && nextDirty > 0 {
					s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
						ch = getWaitCh()
					})
					select {
					case <-ctx.Done():
						return nil
					case <-ch:
					case <-time.After(syncNoProgressBackoff):
					}
				}
			}
			continue
		}

		if dirty > 0 {
			select {
			case <-ctx.Done():
				return nil
			case <-ch:
				continue
			case <-time.After(timeout):
				if err := s.FlushNow(ctx); err != nil {
					if ctx.Err() != nil {
						return nil
					}
					if isDirtySyncGatedCloudError(err) {
						bo.Reset()
						s.le.WithError(err).Warn("flush on timeout gated, waiting for account state change")
						if err := s.waitDirtySyncGate(ctx); err != nil {
							return nil
						}
						continue
					}
					s.le.WithError(err).Warn("flush on timeout failed")
					delay := nextProviderRetryDelay(bo, err)
					s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
						ch = getWaitCh()
					})
					if err := waitDirtySyncRetry(ctx, ch, delay); err != nil {
						return nil
					}
				} else {
					bo.Reset()
				}
			}
		} else {
			bo.Reset()
			select {
			case <-ctx.Done():
				return nil
			case <-ch:
			}
		}
	}
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
		var dirty int64
		var dirtyCh <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			dirty = s.dirtySize
			dirtyCh = getWaitCh()
		})
		if dirty == 0 {
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
	return s.flush(ctx, true)
}

// FlushNowUnordered flushes dirty blocks without refgraph locality ordering.
func (s *syncController) FlushNowUnordered(ctx context.Context) error {
	s.flushMtx.Lock()
	defer s.flushMtx.Unlock()
	return s.flush(ctx, false)
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

	if s.le != nil {
		s.le.WithField("pack-id", packID).
			Debug("retrying canceled sync push with detached context")
	}
	return pushFn(retryCtx, packID, blockCount)
}

// MarkDirty marks a block as dirty for sync.
func (s *syncController) MarkDirty(ctx context.Context, h *hash.Hash, size int64) {
	key := []byte("dirty/" + h.MarshalString())
	tx, err := s.store.NewTransaction(ctx, true)
	if err != nil {
		s.le.WithError(err).Warn("failed to open tx for dirty mark")
		return
	}
	defer tx.Discard()
	sizeBytes := []byte(strconv.FormatInt(size, 10))
	if err := tx.Set(ctx, key, sizeBytes); err != nil {
		s.le.WithError(err).Warn("failed to mark dirty")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		s.le.WithError(err).Warn("failed to commit dirty mark")
		return
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.dirtySize += size
		broadcast()
	})
	s.telemetrySafeCall(func(t *ProviderAccount, id string) {
		t.addSyncTelemetryDirty(id, size)
	})
}

// recalcDirtySize recalculates the dirty size from the object store on startup.
func (s *syncController) recalcDirtySize(ctx context.Context) {
	var total int64
	var count int
	tx, err := s.store.NewTransaction(ctx, false)
	if err != nil {
		s.le.WithError(err).Warn("failed to open tx for dirty recalc")
		return
	}
	defer tx.Discard()
	prefix := []byte("dirty/")
	_ = tx.ScanPrefix(ctx, prefix, func(_, v []byte) error {
		if len(v) > 0 {
			if n, err := strconv.ParseInt(string(v), 10, 64); err == nil {
				total += n
			}
		}
		count++
		return nil
	})

	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.dirtySize = total
		broadcast()
	})
	s.telemetrySafeCall(func(t *ProviderAccount, id string) {
		t.setSyncTelemetryPending(id, total, count)
	})
}

// dirtyBlock holds a dirty block's hash key and data for flushing.
type dirtyBlock struct {
	key  []byte
	hash *hash.Hash
	data []byte
}

type preparedSyncChunk struct {
	blocks   []dirtyBlock
	entry    *packfile.PackfileEntry
	packData []byte
	bodyHash []byte
}

// syncPushProgressReader wraps an io.Reader and fires a progress callback at
// most once per syncPushProgressInterval, plus a final callback at EOF.
// io.Reader is not safe for concurrent goroutine use, and this type inherits
// that contract: Read must be invoked from a single goroutine, which is why
// sent and next are plain int64 rather than atomic.Int64.
type syncPushProgressReader struct {
	reader io.Reader
	sent   int64
	next   int64
	cb     func(int64)
}

func newSyncPushProgressReader(reader io.Reader, cb func(int64)) *syncPushProgressReader {
	return &syncPushProgressReader{
		reader: reader,
		cb:     cb,
		next:   time.Now().Add(syncPushProgressInterval).UnixNano(),
	}
}

func (p *syncPushProgressReader) Read(buf []byte) (int, error) {
	n, err := p.reader.Read(buf)
	if n > 0 {
		p.sent += int64(n)
		now := time.Now().UnixNano()
		if now >= p.next {
			p.next = now + int64(syncPushProgressInterval)
			p.cb(p.sent)
		}
	}
	if err == io.EOF {
		p.cb(p.sent)
	}
	return n, err
}

// packBlocks writes the dirty blocks to w and returns the pack result and body hash.
func (s *syncController) packBlocks(w io.Writer, blocks []dirtyBlock) (*writer.PackResult, []byte, error) {
	hashWriter := sha256.New()
	multiWriter := io.MultiWriter(w, hashWriter)

	idx := 0
	iter := func() (*hash.Hash, []byte, error) {
		if idx >= len(blocks) {
			return nil, nil, nil
		}
		b := blocks[idx]
		idx++
		return b.hash, b.data, nil
	}

	result, err := writer.PackBlocks(multiWriter, iter)
	if err != nil {
		return nil, nil, errors.Wrap(err, "packing blocks")
	}
	return result, hashWriter.Sum(nil), nil
}

// cleanupDirtyBlocks removes flushed dirty keys from the object store.
func (s *syncController) cleanupDirtyBlocks(ctx context.Context, blocks []dirtyBlock) error {
	wtx, err := s.store.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "write tx for dirty cleanup")
	}
	defer wtx.Discard()
	for _, b := range blocks {
		if err := wtx.Delete(ctx, b.key); err != nil {
			return errors.Wrap(err, "deleting dirty key")
		}
	}
	if err := wtx.Commit(ctx); err != nil {
		return errors.Wrap(err, "committing dirty cleanup")
	}
	return nil
}

// orderDirtyBlocks orders dirty blocks for pack locality before chunking.
func (s *syncController) orderDirtyBlocks(ctx context.Context, blocks []dirtyBlock) ([]dirtyBlock, error) {
	refs := make([]*block.BlockRef, 0, len(blocks))
	byKey := make(map[string]dirtyBlock, len(blocks))
	for _, b := range blocks {
		key := b.hash.MarshalString()
		byKey[key] = b
		refs = append(refs, block.NewBlockRef(b.hash))
	}

	orderedRefs, err := packfile_order.BlockRefs(ctx, s.refGraph, refs)
	if err != nil {
		return nil, err
	}
	ordered := make([]dirtyBlock, 0, len(orderedRefs))
	for _, ref := range orderedRefs {
		b, ok := byKey[ref.GetHash().MarshalString()]
		if ok {
			ordered = append(ordered, b)
		}
	}
	return ordered, nil
}

func (s *syncController) filterDuplicateDirtyBlocks(ctx context.Context, blocks []dirtyBlock) ([]dirtyBlock, []dirtyBlock, error) {
	refs := make([]*block.BlockRef, 0, len(blocks))
	for _, b := range blocks {
		refs = append(refs, block.NewBlockRef(b.hash))
	}
	exists, err := s.lower.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return nil, nil, err
	}

	pack := make([]dirtyBlock, 0, len(blocks))
	deduped := make([]dirtyBlock, 0)
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
			bytes += int64(len(b.data))
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

// prepareFlushChunk packs one bounded dirty-block chunk.
func (s *syncController) prepareFlushChunk(blocks []dirtyBlock) (*preparedSyncChunk, error) {
	var buf bytes.Buffer
	started := time.Now()
	result, bodyHash, err := s.packBlocks(&buf, blocks)
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

	packData := bytes.Clone(buf.Bytes())
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
		packData: packData,
		bodyHash: bodyHash,
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
		int(entry.GetBlockCount()),
		func(ctx context.Context, packID string, blockCount int) error {
			return s.client.syncPushDataWithProgress(
				ctx,
				s.resourceID,
				packID,
				blockCount,
				chunk.packData,
				chunk.bodyHash,
				entry.GetBloomFilter(),
				entry.GetBloomFormatVersion(),
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

func (s *syncController) pushPreparedChunks(ctx context.Context, chunks []*preparedSyncChunk) error {
	sem := make(chan struct{}, syncPushConcurrency)
	errCh := make(chan error, len(chunks))
	var wg sync.WaitGroup
	for _, chunk := range chunks {
		if chunk == nil {
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(chunk *preparedSyncChunk) {
			defer wg.Done()
			defer func() { <-sem }()
			errCh <- s.pushPreparedChunk(ctx, chunk)
		}(chunk)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

// flush collects dirty blocks, packs them, pushes to the server, and updates the manifest.
func (s *syncController) flush(ctx context.Context, orderBlocks bool) error {
	// Collect dirty keys from ObjectStore.
	var dirtyKeys [][]byte
	var dirtyHashes []*hash.Hash
	tx, err := s.store.NewTransaction(ctx, false)
	if err != nil {
		return errors.Wrap(err, "read tx for dirty")
	}
	prefix := []byte("dirty/")
	err = tx.ScanPrefix(ctx, prefix, func(k, v []byte) error {
		keyCopy := make([]byte, len(k))
		copy(keyCopy, k)
		hashStr := string(k[len(prefix):])
		h := &hash.Hash{}
		if err := h.ParseFromB58(hashStr); err != nil {
			return errors.Wrap(err, "parsing dirty hash key")
		}
		dirtyKeys = append(dirtyKeys, keyCopy)
		dirtyHashes = append(dirtyHashes, h)
		return nil
	})
	tx.Discard()
	if err != nil {
		return errors.Wrap(err, "scanning dirty keys")
	}

	if len(dirtyHashes) == 0 {
		s.recalcDirtySize(ctx)
		return nil
	}

	s.le.WithField("dirty-blocks", len(dirtyHashes)).
		WithField("order-blocks", orderBlocks).
		Debug("starting dirty block flush")

	// Collect block data from upper store.
	var blocks []dirtyBlock
	for i, h := range dirtyHashes {
		ref := block.NewBlockRef(h)
		data, found, err := s.upper.GetBlock(ctx, ref)
		if err != nil {
			return errors.Wrap(err, "getting dirty block")
		}
		if !found {
			continue
		}
		blocks = append(blocks, dirtyBlock{key: dirtyKeys[i], hash: h, data: data})
	}

	if len(blocks) == 0 {
		s.recalcDirtySize(ctx)
		return nil
	}

	blocks, dedupedBlocks, err := s.filterDuplicateDirtyBlocks(ctx, blocks)
	if err != nil {
		s.recalcDirtySize(ctx)
		return errors.Wrap(err, "filtering duplicate dirty blocks")
	}
	if len(blocks) == 0 {
		if err := s.cleanupDirtyBlocks(ctx, dedupedBlocks); err != nil {
			return err
		}
		s.recalcDirtySize(ctx)
		return nil
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
			s.recalcDirtySize(ctx)
			return errors.Wrap(err, "ordering dirty blocks")
		}
	}

	policy := writer.DefaultPolicy()
	maxChunkBytes := policy.MaxPackBytes
	maxChunkBlocks := int(policy.MaxBlocksPerPack)
	start := 0
	var chunks []*preparedSyncChunk
	for start < len(blocks) {
		end := start
		var chunkBytes int64
		for end < len(blocks) {
			blockBytes := int64(len(blocks[end].data))
			if chunkBytes == 0 && blockBytes > maxChunkBytes {
				return errors.Errorf(
					"dirty block %s exceeds max pack chunk size",
					blocks[end].hash.MarshalString(),
				)
			}
			if chunkBytes > 0 && chunkBytes+blockBytes > maxChunkBytes {
				break
			}
			if maxChunkBlocks > 0 && end-start >= maxChunkBlocks {
				break
			}
			chunkBytes += blockBytes
			end++
		}

		chunk, err := s.prepareFlushChunk(blocks[start:end])
		if err != nil {
			s.recalcDirtySize(ctx)
			return err
		}
		if chunk != nil {
			chunks = append(chunks, chunk)
		}
		start = end
	}

	if err := s.pushPreparedChunks(ctx, chunks); err != nil {
		s.recalcDirtySize(ctx)
		return err
	}

	entries := make([]*packfile.PackfileEntry, 0, len(chunks))
	flushedBlocks := make([]dirtyBlock, 0, len(dedupedBlocks)+len(blocks))
	flushedBlocks = append(flushedBlocks, dedupedBlocks...)
	for _, chunk := range chunks {
		entries = append(entries, chunk.entry)
		flushedBlocks = append(flushedBlocks, chunk.blocks...)
	}
	if len(entries) != 0 {
		if err := s.mfst.ApplyDelta(ctx, entries, nil); err != nil {
			return errors.Wrap(err, "applying push delta")
		}
		s.lower.UpdateManifest(s.mergedManifestEntries())
	}

	if err := s.cleanupDirtyBlocks(ctx, flushedBlocks); err != nil {
		return err
	}

	// Recalculate dirty size from the store so concurrent markDirty calls that
	// raced with this flush are preserved for the next cycle.
	s.recalcDirtySize(ctx)

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

	if len(respData) == 0 {
		return nil
	}

	resp := &packfile.PullResponse{}
	if err := resp.UnmarshalVT(respData); err != nil {
		return errors.Wrap(err, "unmarshaling pull response")
	}

	entries := resp.GetEntries()
	events := resp.GetReplacementEvents()
	if len(entries) == 0 && len(events) == 0 {
		return nil
	}

	if err := s.mfst.ApplyDelta(ctx, entries, events); err != nil {
		return errors.Wrap(err, "applying pull delta")
	}
	s.lower.UpdateManifest(s.mergedManifestEntries())

	s.le.WithField("entries", len(entries)).
		WithField("replacement-events", len(events)).
		Debug("pulled packfile delta")
	return nil
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

func isRetryableSyncPushCancel(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "context canceled") ||
		strings.Contains(msg, "deadline exceeded")
}

// syncTmpDir returns the temp directory for packfile writes.
// Uses BLDR_PLUGIN_STATE_PATH/tmp if set, otherwise system temp.
func syncTmpDir() string {
	dir := os.Getenv("BLDR_PLUGIN_STATE_PATH")
	if dir == "" {
		return ""
	}
	tmpDir := filepath.Join(dir, "tmp")
	_ = os.MkdirAll(tmpDir, 0o755)
	return tmpDir
}

// cleanStaleTempFiles removes stale pack temp files older than 1 hour.
func (s *syncController) cleanStaleTempFiles() {
	if s.tmpDir == "" {
		return
	}
	entries, err := os.ReadDir(s.tmpDir)
	if err != nil {
		return
	}
	threshold := time.Now().Add(-1 * time.Hour)
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "pack-") || !strings.HasSuffix(e.Name(), ".tmp") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(threshold) {
			_ = os.Remove(filepath.Join(s.tmpDir, e.Name()))
		}
	}
}

// _ is a type assertion
var _ io.Reader = ((*syncPushProgressReader)(nil))
