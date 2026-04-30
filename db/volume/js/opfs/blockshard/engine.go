//go:build js

package blockshard

import (
	"context"

	trace "github.com/s4wave/spacewave/db/traceutil"
	"hash/fnv"
	"runtime"
	"strconv"
	"sync"
	"syscall/js"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
)

// DefaultShardCount is the default number of block shards.
const DefaultShardCount = 4

// writeReq is an internal request to the shard write actor.
type writeReq struct {
	entries []segment.Entry
	err     chan error
}

// Engine is the multi-shard block store engine.
type Engine struct {
	shards      []*Shard
	actors      []chan writeReq
	bgActors    []chan writeReq
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	compactionN int
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
		actors:      make([]chan writeReq, settings.ShardCount),
		bgActors:    make([]chan writeReq, settings.ShardCount),
		cancel:      cancel,
		compactionN: settings.CompactionTrigger,
		broadcaster: NewBroadcaster(),
		listener:    NewListener(),
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
		release, err := shard.AcquirePublishLock()
		if err != nil {
			cancel()
			return nil, errors.Errorf("lock shard %d recovery: %v", i, err)
		}
		if _, err := shard.ReclaimPendingDelete(); err != nil {
			release()
			cancel()
			return nil, errors.Errorf("reclaim shard %d pending delete: %v", i, err)
		}
		release()
		if err := shard.CleanOrphans(); err != nil {
			cancel()
			return nil, errors.Errorf("clean shard %d orphans: %v", i, err)
		}
		e.shards[i] = shard
		e.actors[i] = make(chan writeReq, 64)
		e.bgActors[i] = make(chan writeReq, 64)

		e.wg.Add(1)
		go e.runActor(ctx, i)
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
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/put")
	defer task.End()

	if len(entries) == 0 {
		return nil
	}

	// Partition by shard.
	taskCtx, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/put/partition-by-shard")
	buckets := make([][]segment.Entry, len(e.shards))
	for i := range entries {
		idx := e.ShardForKey(entries[i].Key)
		buckets[idx] = append(buckets[idx], entries[i])
	}
	subtask.End()

	// Dispatch to shard actors and collect results.
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
			reqCtx, reqTask := trace.NewTask(taskCtx, "hydra/opfs-blockshard/put/queue-request")
			select {
			case e.actors[idx] <- writeReq{entries: b, err: ch}:
				reqTask.End()
			case <-ctx.Done():
				reqTask.End()
				errs[idx] = ctx.Err()
				return
			}
			_, waitTask := trace.NewTask(reqCtx, "hydra/opfs-blockshard/put/wait-request")
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

// PutBackground enqueues entries to the low-priority background channel.
// Background requests are processed only when no foreground work is pending.
// Used for GC block writes and other non-latency-sensitive operations.
func (e *Engine) PutBackground(ctx context.Context, entries []segment.Entry) error {
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/put-background")
	defer task.End()

	if len(entries) == 0 {
		return nil
	}

	_, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/put-background/partition-by-shard")
	buckets := make([][]segment.Entry, len(e.shards))
	for i := range entries {
		idx := e.ShardForKey(entries[i].Key)
		buckets[idx] = append(buckets[idx], entries[i])
	}
	subtask.End()

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
			select {
			case e.bgActors[idx] <- writeReq{entries: b, err: ch}:
			case <-ctx.Done():
				errs[idx] = ctx.Err()
				return
			}
			select {
			case errs[idx] = <-ch:
			case <-ctx.Done():
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
			if !retried && opfs.IsNotFound(err) {
				taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/get-from-shard/refresh-manifest/not-found-retry")
				refreshed, refreshErr := e.refreshShardManifest(shardIdx)
				subtask.End()
				if refreshErr == nil && refreshed != nil && refreshed.Generation > m.Generation {
					return e.getFromShard(ctx, shardIdx, key, true)
				}
			}
			return nil, false, errors.Errorf("load segment %s lookup: %v", seg.Filename, err)
		}
		taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/get-from-shard/open-segment")
		f, err := shard.getSegmentFile(taskCtx, seg)
		subtask.End()
		if err != nil {
			if !retried && opfs.IsNotFound(err) {
				taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/get-from-shard/refresh-manifest/not-found-retry")
				refreshed, refreshErr := e.refreshShardManifest(shardIdx)
				subtask.End()
				if refreshErr == nil && refreshed != nil && refreshed.Generation > m.Generation {
					return e.getFromShard(ctx, shardIdx, key, true)
				}
			}
			return nil, false, errors.Errorf("open segment %s: %v", seg.Filename, err)
		}
		taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/get-from-shard/locate")
		val, found, tombstone, err := lookup.Locate(f, key, true)
		subtask.End()
		if err != nil {
			if !retried && opfs.IsNotFound(err) {
				shard.dropSegmentFile(seg.Filename)
				taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/get-from-shard/refresh-manifest/not-found-retry")
				refreshed, refreshErr := e.refreshShardManifest(shardIdx)
				subtask.End()
				if refreshErr == nil && refreshed != nil && refreshed.Generation > m.Generation {
					return e.getFromShard(ctx, shardIdx, key, true)
				}
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

func (e *Engine) getExistsFromShard(shardIdx int, key []byte, retried bool) (bool, error) {
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
		lookup, err := shard.getLookup(context.Background(), seg)
		if err != nil {
			if !retried && opfs.IsNotFound(err) {
				shard.dropSegmentFile(seg.Filename)
				refreshed, refreshErr := e.refreshShardManifest(shardIdx)
				if refreshErr == nil && refreshed != nil && refreshed.Generation > m.Generation {
					return e.getExistsFromShard(shardIdx, key, true)
				}
			}
			return false, errors.Errorf("load segment %s lookup: %v", seg.Filename, err)
		}
		f, err := shard.getSegmentFile(context.Background(), seg)
		if err != nil {
			if !retried && opfs.IsNotFound(err) {
				refreshed, refreshErr := e.refreshShardManifest(shardIdx)
				if refreshErr == nil && refreshed != nil && refreshed.Generation > m.Generation {
					return e.getExistsFromShard(shardIdx, key, true)
				}
			}
			return false, errors.Errorf("open segment %s: %v", seg.Filename, err)
		}
		_, found, tombstone, err := lookup.Locate(f, key, false)
		if err != nil {
			if !retried && opfs.IsNotFound(err) {
				shard.dropSegmentFile(seg.Filename)
				refreshed, refreshErr := e.refreshShardManifest(shardIdx)
				if refreshErr == nil && refreshed != nil && refreshed.Generation > m.Generation {
					return e.getExistsFromShard(shardIdx, key, true)
				}
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
	taskCtx, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/refresh-shard-manifest/read-generation")
	genData := readFileBytesContext(taskCtx, shard.dir, manifestGen)
	subtask.End()
	if gen, ok := decodeManifestGeneration(genData); ok {
		if gen <= current.Generation {
			return current, nil
		}

		slot := manifestSlotA
		if gen%2 == 0 {
			slot = manifestSlotB
		}
		taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/refresh-shard-manifest/read-target-slot")
		slotData := readFileBytesContext(taskCtx, shard.dir, slot)
		subtask.End()
		taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/refresh-shard-manifest/decode-target-slot")
		m, err := DecodeManifest(slotData)
		subtask.End()
		if err == nil && m != nil && m.Generation == gen {
			_, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/refresh-shard-manifest/update-cache")
			shard.mu.Lock()
			shard.setManifestLocked(m)
			shard.mu.Unlock()
			subtask.End()
			return m.Clone(), nil
		}
	}

	taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/refresh-shard-manifest/read-slot-a")
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
			if !retried && opfs.IsNotFound(err) {
				shard.dropSegmentFile(seg.Filename)
				refreshed, refreshErr := e.refreshShardManifest(shardIdx)
				if refreshErr == nil && refreshed != nil && refreshed.Generation > m.Generation {
					return e.getExistsBatchFromShard(ctx, shardIdx, keys, true)
				}
			}
			return nil, errors.Errorf("load segment %s lookup: %v", seg.Filename, err)
		}
		f, err := shard.getSegmentFile(ctx, seg)
		if err != nil {
			if !retried && opfs.IsNotFound(err) {
				refreshed, refreshErr := e.refreshShardManifest(shardIdx)
				if refreshErr == nil && refreshed != nil && refreshed.Generation > m.Generation {
					return e.getExistsBatchFromShard(ctx, shardIdx, keys, true)
				}
			}
			return nil, errors.Errorf("open segment %s: %v", seg.Filename, err)
		}
		for _, j := range candidates {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			_, found, tombstone, err := lookup.Locate(f, keys[j], false)
			if err != nil {
				if !retried && opfs.IsNotFound(err) {
					shard.dropSegmentFile(seg.Filename)
					refreshed, refreshErr := e.refreshShardManifest(shardIdx)
					if refreshErr == nil && refreshed != nil && refreshed.Generation > m.Generation {
						return e.getExistsBatchFromShard(ctx, shardIdx, keys, true)
					}
				}
				return nil, err
			}
			if tombstone {
				resolved[j] = true
				out[j] = false
				continue
			}
			if found {
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
// Priority channels: foreground requests (actors[i]) are always drained
// before background requests (bgActors[i]). Background requests are only
// processed when the foreground channel is empty, or when bgStarvationLimit
// consecutive foreground-only cycles have occurred.
//
// Coalescing: after the first request, the actor yields and drains repeatedly
// until no new requests arrive or maxCoalesceRounds is reached. This collapses
// commit-burst traffic into fewer, larger publishes without adding latency to
// singleton puts.
func (e *Engine) runActor(ctx context.Context, shardIdx int) {
	defer e.wg.Done()
	fgCh := e.actors[shardIdx]
	bgCh := e.bgActors[shardIdx]
	shard := e.shards[shardIdx]

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
		for i := range reqs {
			merged = append(merged, reqs[i].entries...)
		}

		// Acquire publish lock and flush.
		publishCtx, publishTask := trace.NewTask(ctx, "hydra/opfs-blockshard/run-actor/publish")
		trace.Log(publishCtx, "coalesce", "reqs="+strconv.Itoa(len(reqs))+" entries="+strconv.Itoa(len(merged)))
		_, lockTask := trace.NewTask(publishCtx, "hydra/opfs-blockshard/run-actor/publish/acquire-lock")
		release, err := shard.AcquirePublishLock()
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
			_, err = shard.ReclaimPendingDelete()
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

		// Reply to all waiters.
		for _, r := range reqs {
			r.err <- err
		}
		reqs = reqs[:0]

		// Pipeline overlap: drain foreground entries that arrived during
		// publish. Background entries are picked up at the top of the
		// next iteration after foreground is serviced.
		e.drainCh(fgCh, &reqs)

		// Run compaction only when no foreground entries are waiting.
		if err == nil && len(reqs) == 0 {
			plan := PlanCompaction(shard, e.compactionN)
			if plan != nil {
				release, lockErr := shard.AcquirePublishLock()
				if lockErr == nil {
					compErr := ExecuteCompaction(shard, plan)
					if compErr == nil {
						_, compErr = shard.ReclaimPendingDelete()
					}
					compGen := shard.Manifest().Generation
					shard.observeGeneration(compGen)
					release()
					if compErr == nil {
						e.broadcaster.Send(shardIdx, compGen)
					}
				}
			}
		}
	}
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
