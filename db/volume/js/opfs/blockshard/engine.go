//go:build js

package blockshard

import (
	"context"
	"hash/fnv"
	"runtime"
	"strconv"
	"sync"
	"syscall/js"

	trace "github.com/s4wave/spacewave/db/traceutil"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

// DefaultShardCount is the default number of block shards.
const DefaultShardCount = 4

// writeReq is an internal request to the shard write actor.
type writeReq struct {
	entries    []segment.Entry
	background bool
	err        chan error
}

type compactReq struct {
	ctx context.Context
	err chan error
}

type shardActor struct {
	shardIdx   int
	shard      *Shard
	foreground chan writeReq
	background chan writeReq
	compaction chan compactReq
}

func newShardActor(shardIdx int, shard *Shard) *shardActor {
	return &shardActor{
		shardIdx:   shardIdx,
		shard:      shard,
		foreground: make(chan writeReq, 64),
		background: make(chan writeReq, 64),
		compaction: make(chan compactReq, 1),
	}
}

func (a *shardActor) writeCh(background bool) chan writeReq {
	if background {
		return a.background
	}
	return a.foreground
}

// Engine is the multi-shard block store engine.
type Engine struct {
	shards      []*Shard
	actors      []*shardActor
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	compactionN int
	maxEntryN   int
	broadcaster *Broadcaster
	listener    *Listener
}

// NewEngine creates a new block shard engine in the given OPFS directory.
// It creates shard subdirectories and starts per-shard write actors.
func NewEngine(ctx context.Context, dir js.Value, lockPrefix string, shardCount int) (*Engine, error) {
	settings := DefaultSettings()
	settings.ShardCount = shardCount
	return NewEngineWithSettings(ctx, dir, lockPrefix, settings)
}

// NewEngineWithSettings creates a block shard engine with explicit settings.
func NewEngineWithSettings(
	ctx context.Context,
	dir js.Value,
	lockPrefix string,
	settings *Settings,
) (*Engine, error) {
	settings = normalizeSettings(settings)
	ctx, cancel := context.WithCancel(ctx)
	e := &Engine{
		shards:      make([]*Shard, settings.ShardCount),
		actors:      make([]*shardActor, settings.ShardCount),
		ctx:         ctx,
		cancel:      cancel,
		compactionN: settings.CompactionTrigger,
		maxEntryN:   settings.MaxEntryValueBytes,
		broadcaster: NewBroadcaster(lockPrefix),
		listener:    NewListener(lockPrefix),
	}

	for i := range e.shards {
		name := "shard-" + zeroPad(uint64(i), 2)
		shardDir, err := opfs.GetDirectory(dir, name, true)
		if err != nil {
			cancel()
			return nil, errors.Errorf("create shard %d directory: %v", i, err)
		}
		shard, err := NewShard(i, shardDir, lockPrefix, settings)
		if err != nil {
			cancel()
			return nil, errors.Errorf("open shard %d: %v", i, err)
		}
		release, err := shard.AcquirePublishLockContext(ctx)
		if err != nil {
			cancel()
			return nil, errors.Errorf("lock shard %d recovery: %v", i, err)
		}
		if _, err := shard.ReclaimPendingDelete(ctx); err != nil {
			release()
			cancel()
			return nil, errors.Errorf("reclaim shard %d pending delete: %v", i, err)
		}
		if err := shard.CleanOrphans(); err != nil {
			release()
			cancel()
			return nil, errors.Errorf("clean shard %d orphans: %v", i, err)
		}
		release()
		e.shards[i] = shard
		e.actors[i] = newShardActor(i, shard)

		e.wg.Add(1)
		go e.runActor(ctx, e.actors[i])
	}

	// Start invalidation listener.
	e.wg.Add(1)
	go e.runInvalidationListener(ctx)

	return e, nil
}

// Close stops all write actors and waits for them to drain.
func (e *Engine) Close() {
	e.cancel()
	e.wg.Wait()
	e.broadcaster.Close()
	e.listener.Close()
}

// ShardForKey returns the shard index for a given key.
func (e *Engine) ShardForKey(key []byte) int {
	h := fnv.New32a()
	h.Write(key)
	return int(h.Sum32() % uint32(len(e.shards)))
}

// Put enqueues entries to the appropriate shard write actor.
// Blocks until the entries are flushed to OPFS.
func (e *Engine) Put(ctx context.Context, entries []segment.Entry) error {
	return e.putToActors(ctx, "hydra/opfs-blockshard/put", entries, false)
}

// PutBackground enqueues entries to the low-priority background channel.
// Background requests are processed only when no foreground work is pending.
// Used for GC block writes and other non-latency-sensitive operations.
func (e *Engine) PutBackground(ctx context.Context, entries []segment.Entry) error {
	return e.putToActors(ctx, "hydra/opfs-blockshard/put-background", entries, true)
}

// CompactOnce runs at most one compaction plan per shard.
//
// Compaction is storage maintenance, so foreground writes do not run it
// inline. Maintenance owners can call CompactOnce when they have a lifecycle
// slot for background OPFS work.
func (e *Engine) CompactOnce(ctx context.Context) error {
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/compact-once")
	defer task.End()

	for shardIdx := range e.shards {
		ch := make(chan error, 1)
		req := compactReq{ctx: ctx, err: ch}
		select {
		case e.actors[shardIdx].compaction <- req:
		case <-ctx.Done():
			return ctx.Err()
		case <-e.ctx.Done():
			return context.Canceled
		}
		select {
		case err := <-ch:
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		case <-e.ctx.Done():
			return context.Canceled
		}
	}
	return nil
}

func (e *Engine) compactShardOnce(ctx context.Context, shardIdx int, shard *Shard) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	plan := PlanCompaction(shard, e.compactionN)
	if plan == nil {
		return false, nil
	}

	release, err := shard.AcquirePublishLockContext(ctx)
	if err != nil {
		return false, errors.Wrapf(err, "acquire shard %d publish lock", shardIdx)
	}
	compErr := ExecuteCompaction(ctx, shard, plan)
	if compErr == nil {
		_, compErr = shard.ReclaimPendingDelete(ctx)
	}
	gen := shard.Manifest().Generation
	shard.observeGeneration(gen)
	release()
	if compErr != nil {
		return false, errors.Wrapf(compErr, "compact shard %d", shardIdx)
	}
	e.broadcaster.Send(shardIdx, gen)
	return true, nil
}

// putToActors partitions entries by shard, dispatches them to each shard actor,
// and waits for all replies. tracePrefix is the trace task name prefix used for
// all sub-spans emitted by this call.
func (e *Engine) putToActors(
	ctx context.Context,
	tracePrefix string,
	entries []segment.Entry,
	background bool,
) error {
	ctx, task := trace.NewTask(ctx, tracePrefix)
	defer task.End()

	if len(entries) == 0 {
		return nil
	}
	if err := e.validateEntryValues(entries); err != nil {
		return err
	}

	taskCtx, partitionTask := trace.NewTask(ctx, tracePrefix+"/partition-by-shard")
	buckets := make([][]segment.Entry, len(e.shards))
	for i := range entries {
		idx := e.ShardForKey(entries[i].Key)
		buckets[idx] = append(buckets[idx], entries[i])
	}
	partitionTask.End()

	var wg sync.WaitGroup
	errs := make([]error, len(e.shards))
	for i, batch := range buckets {
		if len(batch) == 0 {
			continue
		}
		wg.Add(1)
		go func(idx int, b []segment.Entry) {
			defer wg.Done()
			ch := make(chan error, 1)
			reqCtx, reqTask := trace.NewTask(taskCtx, tracePrefix+"/queue-request")
			actor := e.actors[idx]
			select {
			case actor.writeCh(background) <- writeReq{entries: b, background: background, err: ch}:
				reqTask.End()
			case <-ctx.Done():
				reqTask.End()
				errs[idx] = ctx.Err()
				return
			}
			_, waitTask := trace.NewTask(reqCtx, tracePrefix+"/wait-request")
			select {
			case errs[idx] = <-ch:
				waitTask.End()
			case <-ctx.Done():
				waitTask.End()
				errs[idx] = ctx.Err()
			}
		}(i, batch)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) validateEntryValues(entries []segment.Entry) error {
	if e.maxEntryN < 1 {
		return nil
	}
	for i := range entries {
		if entries[i].Tombstone {
			continue
		}
		if len(entries[i].Value) > e.maxEntryN {
			return errors.Errorf("blockshard entry value exceeds max size %d", e.maxEntryN)
		}
	}
	return nil
}

// GetFromShard looks up a key in a specific shard by scanning segments newest-first.
func (e *Engine) GetFromShard(shardIdx int, key []byte) ([]byte, bool, error) {
	return e.getFromShard(context.Background(), shardIdx, key, false)
}

// GetExistsFromShard checks whether a key exists in a specific shard without
// loading its value.
func (e *Engine) GetExistsFromShard(shardIdx int, key []byte) (bool, error) {
	return e.getExistsFromShard(shardIdx, key, false)
}

func (e *Engine) getFromShard(
	ctx context.Context,
	shardIdx int,
	key []byte,
	retried bool,
) ([]byte, bool, error) {
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/get-from-shard")
	defer task.End()

	shard := e.shards[shardIdx]
	m := shard.Manifest()
	if latestGen := shard.getLatestGeneration(); latestGen > m.Generation {
		_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/get-from-shard/refresh-manifest/latest-gen-ahead")
		refreshed, err := e.refreshShardManifest(shardIdx)
		subtask.End()
		if err == nil && refreshed != nil && refreshed.Generation > m.Generation {
			m = refreshed
		}
	}

	// Scan segments newest-first (last in manifest = newest).
	for i := len(m.Segments) - 1; i >= 0; i-- {
		seg := &m.Segments[i]
		// Range check.
		if string(key) < string(seg.MinKey) || string(key) > string(seg.MaxKey) {
			continue
		}
		taskCtx, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/get-from-shard/load-lookup")
		lookup, err := shard.getLookup(taskCtx, seg)
		subtask.End()
		if err != nil {
			if e.shouldRetryAfterRefresh(ctx, shardIdx, m.Generation, retried, err) {
				return e.getFromShard(ctx, shardIdx, key, true)
			}
			return nil, false, errors.Errorf("load segment %s lookup: %v", seg.Filename, err)
		}
		taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/get-from-shard/open-segment")
		f, err := shard.getSegmentFile(taskCtx, seg)
		subtask.End()
		if err != nil {
			if e.shouldRetryAfterRefresh(ctx, shardIdx, m.Generation, retried, err) {
				return e.getFromShard(ctx, shardIdx, key, true)
			}
			return nil, false, errors.Errorf("open segment %s: %v", seg.Filename, err)
		}
		taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/get-from-shard/locate")
		val, found, tombstone, err := lookup.Locate(f, key, true)
		subtask.End()
		if err != nil {
			if opfs.IsNotFound(err) {
				shard.dropSegmentFile(seg.Filename)
			}
			if e.shouldRetryAfterRefresh(ctx, shardIdx, m.Generation, retried, err) {
				return e.getFromShard(ctx, shardIdx, key, true)
			}
			return nil, false, err
		}
		if tombstone {
			return nil, false, nil
		}
		if found {
			return val, true, nil
		}
	}
	return nil, false, nil
}

// shouldRetryAfterRefresh decides whether a retried call is warranted after
// hitting err on a segment access.
//
// Returns true only when:
//   - we have not already retried this call,
//   - err is a NotFound (a segment file vanished, almost always because a
//     concurrent compaction retired it), and
//   - refreshing the shard manifest reveals a newer generation than the one
//     used for the current attempt.
//
// Tracing for the refresh attempt is emitted under the caller-supplied ctx so
// the retry shows up as a sibling span next to the original lookup/open/locate
// task it replaced.
func (e *Engine) shouldRetryAfterRefresh(
	ctx context.Context,
	shardIdx int,
	currentGen uint64,
	retried bool,
	err error,
) bool {
	if retried || !opfs.IsNotFound(err) {
		return false
	}
	_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/refresh-manifest/not-found-retry")
	refreshed, refreshErr := e.refreshShardManifest(shardIdx)
	subtask.End()
	return refreshErr == nil && refreshed != nil && refreshed.Generation > currentGen
}

func (e *Engine) getExistsFromShard(shardIdx int, key []byte, retried bool) (bool, error) {
	ctx := context.Background()
	shard := e.shards[shardIdx]
	m := shard.Manifest()
	if latestGen := shard.getLatestGeneration(); latestGen > m.Generation {
		refreshed, err := e.refreshShardManifest(shardIdx)
		if err == nil && refreshed != nil && refreshed.Generation > m.Generation {
			m = refreshed
		}
	}

	for i := len(m.Segments) - 1; i >= 0; i-- {
		seg := &m.Segments[i]
		if string(key) < string(seg.MinKey) || string(key) > string(seg.MaxKey) {
			continue
		}
		lookup, err := shard.getLookup(ctx, seg)
		if err != nil {
			if opfs.IsNotFound(err) {
				shard.dropSegmentFile(seg.Filename)
			}
			if e.shouldRetryAfterRefresh(ctx, shardIdx, m.Generation, retried, err) {
				return e.getExistsFromShard(shardIdx, key, true)
			}
			return false, errors.Errorf("load segment %s lookup: %v", seg.Filename, err)
		}
		f, err := shard.getSegmentFile(ctx, seg)
		if err != nil {
			if e.shouldRetryAfterRefresh(ctx, shardIdx, m.Generation, retried, err) {
				return e.getExistsFromShard(shardIdx, key, true)
			}
			return false, errors.Errorf("open segment %s: %v", seg.Filename, err)
		}
		_, found, tombstone, err := lookup.Locate(f, key, false)
		if err != nil {
			if opfs.IsNotFound(err) {
				shard.dropSegmentFile(seg.Filename)
			}
			if e.shouldRetryAfterRefresh(ctx, shardIdx, m.Generation, retried, err) {
				return e.getExistsFromShard(shardIdx, key, true)
			}
			return false, err
		}
		if tombstone {
			return false, nil
		}
		if found {
			return true, nil
		}
	}
	return false, nil
}

func (e *Engine) refreshShardManifest(shardIdx int) (*Manifest, error) {
	ctx := context.Background()
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/refresh-shard-manifest")
	defer task.End()

	shard := e.shards[shardIdx]
	current := shard.Manifest()
	taskCtx, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/refresh-shard-manifest/read-slot-a")
	a := readFileBytesContext(taskCtx, shard.dir, manifestSlotA)
	subtask.End()
	taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/refresh-shard-manifest/read-slot-b")
	b := readFileBytesContext(taskCtx, shard.dir, manifestSlotB)
	subtask.End()
	taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/refresh-shard-manifest/pick-manifest")
	m := PickManifest(a, b)
	subtask.End()
	if m == nil {
		return nil, nil
	}
	if m.Generation <= current.Generation {
		return current, nil
	}
	_, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/refresh-shard-manifest/update-cache")
	shard.mu.Lock()
	shard.setManifestLocked(m)
	shard.mu.Unlock()
	subtask.End()
	return m.Clone(), nil
}

// Get looks up a key across all shards.
func (e *Engine) Get(key []byte) ([]byte, bool, error) {
	return e.GetContext(context.Background(), key)
}

// GetContext looks up a key across all shards with tracing context.
func (e *Engine) GetContext(ctx context.Context, key []byte) ([]byte, bool, error) {
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/get")
	defer task.End()

	idx := e.ShardForKey(key)
	taskCtx, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/get/get-from-shard")
	val, found, err := e.getFromShard(taskCtx, idx, key, false)
	subtask.End()
	return val, found, err
}

// Stat resolves a key and returns its value size without loading the value.
func (e *Engine) Stat(ctx context.Context, key []byte) (int64, bool, error) {
	idx := e.ShardForKey(key)
	return e.statFromShard(ctx, idx, key, false)
}

// GetExists checks whether a key exists across all shards without loading its
// value.
func (e *Engine) GetExists(key []byte) (bool, error) {
	idx := e.ShardForKey(key)
	return e.GetExistsFromShard(idx, key)
}

// GetExistsBatch checks whether a batch of keys exists across shards without
// loading their values.
func (e *Engine) GetExistsBatch(ctx context.Context, keys [][]byte) ([]bool, error) {
	out := make([]bool, len(keys))
	shardKeys := make(map[int][][]byte)
	shardIdx := make(map[int][]int)
	for i, key := range keys {
		if len(key) == 0 {
			continue
		}
		idx := e.ShardForKey(key)
		shardKeys[idx] = append(shardKeys[idx], key)
		shardIdx[idx] = append(shardIdx[idx], i)
	}

	for idx, batch := range shardKeys {
		found, err := e.getExistsBatchFromShard(ctx, idx, batch, false)
		if err != nil {
			return nil, err
		}
		for i, ok := range found {
			out[shardIdx[idx][i]] = ok
		}
	}
	return out, nil
}

func (e *Engine) statFromShard(
	ctx context.Context,
	shardIdx int,
	key []byte,
	retried bool,
) (int64, bool, error) {
	shard := e.shards[shardIdx]
	m := shard.Manifest()
	if latestGen := shard.getLatestGeneration(); latestGen > m.Generation {
		refreshed, err := e.refreshShardManifest(shardIdx)
		if err == nil && refreshed != nil && refreshed.Generation > m.Generation {
			m = refreshed
		}
	}

	for i := len(m.Segments) - 1; i >= 0; i-- {
		seg := &m.Segments[i]
		if string(key) < string(seg.MinKey) || string(key) > string(seg.MaxKey) {
			continue
		}
		lookup, err := shard.getLookup(ctx, seg)
		if err != nil {
			if opfs.IsNotFound(err) {
				shard.dropSegmentFile(seg.Filename)
			}
			if e.shouldRetryAfterRefresh(ctx, shardIdx, m.Generation, retried, err) {
				return e.statFromShard(ctx, shardIdx, key, true)
			}
			return 0, false, errors.Errorf("load segment %s lookup: %v", seg.Filename, err)
		}
		f, err := shard.getSegmentFile(ctx, seg)
		if err != nil {
			if e.shouldRetryAfterRefresh(ctx, shardIdx, m.Generation, retried, err) {
				return e.statFromShard(ctx, shardIdx, key, true)
			}
			return 0, false, errors.Errorf("open segment %s: %v", seg.Filename, err)
		}
		stat, err := lookup.Stat(f, key)
		if err != nil {
			if opfs.IsNotFound(err) {
				shard.dropSegmentFile(seg.Filename)
			}
			if e.shouldRetryAfterRefresh(ctx, shardIdx, m.Generation, retried, err) {
				return e.statFromShard(ctx, shardIdx, key, true)
			}
			return 0, false, err
		}
		if stat.Tombstone {
			return 0, false, nil
		}
		if stat.Found {
			return stat.ValueSize, true, nil
		}
	}
	return 0, false, nil
}

func (e *Engine) getExistsBatchFromShard(
	ctx context.Context,
	shardIdx int,
	keys [][]byte,
	retried bool,
) ([]bool, error) {
	shard := e.shards[shardIdx]
	m := shard.Manifest()
	if latestGen := shard.getLatestGeneration(); latestGen > m.Generation {
		refreshed, err := e.refreshShardManifest(shardIdx)
		if err == nil && refreshed != nil && refreshed.Generation > m.Generation {
			m = refreshed
		}
	}

	out := make([]bool, len(keys))
	resolved := make([]bool, len(keys))
	for i := len(m.Segments) - 1; i >= 0; i-- {
		seg := &m.Segments[i]
		var candidates []int
		for j, key := range keys {
			if resolved[j] || len(key) == 0 {
				continue
			}
			keyStr := string(key)
			if keyStr < string(seg.MinKey) || keyStr > string(seg.MaxKey) {
				continue
			}
			candidates = append(candidates, j)
		}
		if len(candidates) == 0 {
			continue
		}

		lookup, err := shard.getLookup(ctx, seg)
		if err != nil {
			if opfs.IsNotFound(err) {
				shard.dropSegmentFile(seg.Filename)
			}
			if e.shouldRetryAfterRefresh(ctx, shardIdx, m.Generation, retried, err) {
				return e.getExistsBatchFromShard(ctx, shardIdx, keys, true)
			}
			return nil, errors.Errorf("load segment %s lookup: %v", seg.Filename, err)
		}
		f, err := shard.getSegmentFile(ctx, seg)
		if err != nil {
			if e.shouldRetryAfterRefresh(ctx, shardIdx, m.Generation, retried, err) {
				return e.getExistsBatchFromShard(ctx, shardIdx, keys, true)
			}
			return nil, errors.Errorf("open segment %s: %v", seg.Filename, err)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		candidateKeys := make([][]byte, len(candidates))
		for i, j := range candidates {
			candidateKeys[i] = keys[j]
		}
		results, err := lookup.LocateBatch(f, candidateKeys, false)
		if err != nil {
			if opfs.IsNotFound(err) {
				shard.dropSegmentFile(seg.Filename)
			}
			if e.shouldRetryAfterRefresh(ctx, shardIdx, m.Generation, retried, err) {
				return e.getExistsBatchFromShard(ctx, shardIdx, keys, true)
			}
			return nil, err
		}
		for i, result := range results {
			j := candidates[i]
			if result.Tombstone {
				resolved[j] = true
				out[j] = false
				continue
			}
			if result.Found {
				resolved[j] = true
				out[j] = true
			}
		}
	}

	if !retried && shard.getLatestGeneration() > m.Generation {
		refreshed, err := e.refreshShardManifest(shardIdx)
		if err == nil && refreshed != nil && refreshed.Generation > m.Generation {
			return e.getExistsBatchFromShard(ctx, shardIdx, keys, true)
		}
	}
	return out, nil
}

// maxCoalesceRounds is the maximum number of yield+drain cycles the actor
// performs before publishing a coalesced batch. Prevents unbounded looping
// when requests arrive faster than the drain rate.
const maxCoalesceRounds = 16

// bgStarvationLimit is the maximum number of consecutive foreground-only
// publish cycles before the actor forces one background drain. Prevents
// background requests from starving under sustained foreground load.
const bgStarvationLimit = 4

// runActor is the per-shard write actor goroutine.
// Pipeline model: publish immediately on first entry, accumulate the queue
// behind running I/O, and batch whatever arrived during publish as the next
// round. Singleton writes pay only publish cost (no idle wait). Bursty writes
// batch naturally because entries collect while I/O is in flight.
//
// Priority channels: foreground requests are always drained before background
// requests. Background requests are only processed when the foreground channel
// is empty, or when bgStarvationLimit consecutive foreground-only cycles have
// occurred.
//
// Coalescing: after the first request, the actor yields and drains repeatedly
// until no new requests arrive or maxCoalesceRounds is reached. This collapses
// commit-burst traffic into fewer, larger publishes without adding latency to
// singleton puts.
func (e *Engine) runActor(ctx context.Context, actor *shardActor) {
	defer e.wg.Done()
	shardIdx := actor.shardIdx
	fgCh := actor.foreground
	bgCh := actor.background
	compactCh := actor.compaction
	shard := actor.shard

	var reqs []writeReq
	var fgOnly int // consecutive foreground-only cycles
	for {
		// If no pending entries, block for the next request.
		// Prefer foreground: try fgCh first, only fall through to
		// bgCh when fgCh is not ready.
		if len(reqs) == 0 {
			select {
			case req := <-fgCh:
				reqs = append(reqs, req)
			case <-ctx.Done():
				return
			default:
				select {
				case req := <-fgCh:
					reqs = append(reqs, req)
				case req := <-bgCh:
					reqs = append(reqs, req)
				case req := <-compactCh:
					req.err <- e.runCompactReq(req.ctx, shardIdx, shard)
					continue
				case <-ctx.Done():
					return
				}
			}
		}

		// Drain foreground channel (always first priority).
		e.drainCh(fgCh, &reqs)

		// Drain background channel when foreground is empty or
		// starvation limit is reached.
		hasBg := len(bgCh) > 0
		hasFg := len(reqs) > 0
		drainBg := hasBg && (!hasFg || fgOnly >= bgStarvationLimit)
		if drainBg {
			e.drainCh(bgCh, &reqs)
			fgOnly = 0
		} else if hasFg && !hasBg {
			fgOnly++
		}

		// Coalescing yield-drain loop: repeat yield+drain until no new
		// requests arrive or maxCoalesceRounds is reached. Singleton puts
		// (nothing queued after first round) publish immediately.
		// Only drain the background channel during coalescing when the
		// starvation/empty condition was met for this cycle, otherwise
		// background entries would inflate foreground publish latency.
		for range maxCoalesceRounds {
			runtime.Gosched()
			prevLen := len(reqs)
			e.drainCh(fgCh, &reqs)
			if drainBg {
				e.drainCh(bgCh, &reqs)
			}
			if len(reqs) == prevLen {
				break
			}
		}

		// Merge all entries.
		var merged []segment.Entry
		hasForegroundReq := false
		for i := range reqs {
			if !reqs[i].background {
				hasForegroundReq = true
			}
			merged = append(merged, reqs[i].entries...)
		}

		// Acquire publish lock and flush.
		publishCtx, publishTask := trace.NewTask(ctx, "hydra/opfs-blockshard/run-actor/publish")
		trace.Log(publishCtx, "coalesce", "reqs="+strconv.Itoa(len(reqs))+" entries="+strconv.Itoa(len(merged)))
		_, lockTask := trace.NewTask(publishCtx, "hydra/opfs-blockshard/run-actor/publish/acquire-lock")
		release, err := shard.AcquirePublishLockContext(publishCtx)
		lockTask.End()
		if err != nil {
			publishTask.End()
			for _, r := range reqs {
				r.err <- errors.Wrap(err, "acquire publish lock")
			}
			reqs = reqs[:0]
			continue
		}

		writeCtx, writeTask := trace.NewTask(publishCtx, "hydra/opfs-blockshard/run-actor/publish/shard-publish")
		err = shard.Publish(writeCtx, merged)
		writeTask.End()
		if err == nil {
			_, reclaimTask := trace.NewTask(publishCtx, "hydra/opfs-blockshard/run-actor/publish/reclaim-pending-delete")
			_, err = shard.ReclaimPendingDelete(publishCtx)
			reclaimTask.End()
		}
		gen := shard.Manifest().Generation
		shard.observeGeneration(gen)
		release()
		publishTask.End()

		// Broadcast invalidation to peer workers.
		if err == nil {
			e.broadcaster.Send(shardIdx, gen)
		}

		currentReqs := reqs
		reqs = nil

		// Pipeline overlap: drain foreground entries that arrived during
		// publish. Background entries are picked up at the top of the
		// next iteration after foreground is serviced. The drain happens
		// before background-only compaction so a queued foreground write can
		// preempt maintenance before any current-cycle waiters are released.
		e.drainCh(fgCh, &reqs)

		// Compaction is maintenance, not part of the foreground write
		// completion path. Large browser uploads can create enough segments for
		// compaction immediately after the upload promise resolves; running it
		// here keeps the shard publish Web Lock held while user readback and
		// debug requests are waiting on the same TinyGo worker. Only
		// background-only cycles may opportunistically compact here.
		if err == nil && !hasForegroundReq && len(reqs) == 0 {
			if _, compErr := e.compactShardOnce(ctx, shardIdx, shard); compErr != nil && ctx.Err() == nil {
				trace.Logf(ctx, "blockshard", "background compaction failed: shard=%d err=%v", shardIdx, compErr)
			}
		}

		// Reply to the published waiters. Background-only waiters observe
		// opportunistic maintenance completion; foreground waiters never wait
		// on inline compaction because hasForegroundReq disables it.
		for _, r := range currentReqs {
			r.err <- err
		}
	}
}

func (e *Engine) runCompactReq(ctx context.Context, shardIdx int, shard *Shard) error {
	_, err := e.compactShardOnce(ctx, shardIdx, shard)
	return err
}

// drainCh non-blocking drains all available requests from ch into reqs.
func (e *Engine) drainCh(ch <-chan writeReq, reqs *[]writeReq) {
	for {
		select {
		case req := <-ch:
			*reqs = append(*reqs, req)
		default:
			return
		}
	}
}

// runInvalidationListener handles BroadcastChannel messages from peer workers.
// When a peer publishes a new shard generation, we refresh our manifest cache.
func (e *Engine) runInvalidationListener(ctx context.Context) {
	defer e.wg.Done()
	for {
		select {
		case <-e.listener.Notify():
			for _, msg := range e.listener.DrainPending() {
				idx := int(msg.ShardID)
				if idx < 0 || idx >= len(e.shards) {
					continue
				}
				shard := e.shards[idx]
				shard.observeGeneration(msg.Generation)
				current := shard.Manifest()
				if msg.Generation > current.Generation {
					if _, err := e.refreshShardManifest(idx); err != nil {
						continue
					}
				}
			}
		case <-ctx.Done():
			return
		}
	}
}
