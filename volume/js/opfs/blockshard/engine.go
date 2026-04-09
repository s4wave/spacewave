//go:build js

package blockshard

import (
	"context"
	"hash/fnv"
	"runtime/trace"
	"runtime"
	"sync"
	"syscall/js"

	"github.com/aperturerobotics/hydra/opfs"
	"github.com/aperturerobotics/hydra/volume/js/opfs/segment"
	"github.com/pkg/errors"
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

// GetFromShard looks up a key in a specific shard by scanning segments newest-first.
func (e *Engine) GetFromShard(shardIdx int, key []byte) ([]byte, bool, error) {
	return e.getFromShard(shardIdx, key, false)
}

func (e *Engine) getFromShard(shardIdx int, key []byte, retried bool) ([]byte, bool, error) {
	shard := e.shards[shardIdx]
	m := shard.Manifest()

	// Scan segments newest-first (last in manifest = newest).
	for i := len(m.Segments) - 1; i >= 0; i-- {
		seg := &m.Segments[i]
		// Range check.
		if string(key) < string(seg.MinKey) || string(key) > string(seg.MaxKey) {
			continue
		}
		lookup, err := shard.getLookup(seg)
		if err != nil {
			if !retried && opfs.IsNotFound(err) {
				refreshed, refreshErr := e.refreshShardManifest(shardIdx)
				if refreshErr == nil && refreshed != nil && refreshed.Generation > m.Generation {
					return e.getFromShard(shardIdx, key, true)
				}
			}
			return nil, false, errors.Errorf("load segment %s lookup: %v", seg.Filename, err)
		}
		f, err := opfs.OpenAsyncFile(shard.dir, seg.Filename)
		if err != nil {
			if !retried && opfs.IsNotFound(err) {
				refreshed, refreshErr := e.refreshShardManifest(shardIdx)
				if refreshErr == nil && refreshed != nil && refreshed.Generation > m.Generation {
					return e.getFromShard(shardIdx, key, true)
				}
			}
			return nil, false, errors.Errorf("open segment %s: %v", seg.Filename, err)
		}
		val, found, err := lookup.Get(f, key)
		if err != nil {
			if !retried && opfs.IsNotFound(err) {
				refreshed, refreshErr := e.refreshShardManifest(shardIdx)
				if refreshErr == nil && refreshed != nil && refreshed.Generation > m.Generation {
					return e.getFromShard(shardIdx, key, true)
				}
			}
			return nil, false, err
		}
		if found {
			return val, true, nil
		}
	}
	return nil, false, nil
}

func (e *Engine) refreshShardManifest(shardIdx int) (*Manifest, error) {
	shard := e.shards[shardIdx]
	a := readFileBytes(shard.dir, manifestSlotA)
	b := readFileBytes(shard.dir, manifestSlotB)
	m := PickManifest(a, b)
	if m == nil {
		return nil, nil
	}
	shard.mu.Lock()
	shard.setManifestLocked(m)
	shard.mu.Unlock()
	return m.Clone(), nil
}

// Get looks up a key across all shards.
func (e *Engine) Get(key []byte) ([]byte, bool, error) {
	idx := e.ShardForKey(key)
	return e.GetFromShard(idx, key)
}

// runActor is the per-shard write actor goroutine.
// Pipeline model: publish immediately on first entry, accumulate the queue
// behind running I/O, and batch whatever arrived during publish as the next
// round. Singleton writes pay only publish cost (no idle wait). Bursty writes
// batch naturally because entries collect while I/O is in flight.
func (e *Engine) runActor(ctx context.Context, shardIdx int) {
	defer e.wg.Done()
	ch := e.actors[shardIdx]
	shard := e.shards[shardIdx]

	var reqs []writeReq
	for {
		// If no pending entries, block for the next request.
		if len(reqs) == 0 {
			select {
			case req := <-ch:
				reqs = append(reqs, req)
			case <-ctx.Done():
				return
			}
		}

		// Yield so sibling goroutines can enqueue.
		runtime.Gosched()

		// Non-blocking drain: collect whatever accumulated during the yield.
	drain:
		for {
			select {
			case req := <-ch:
				reqs = append(reqs, req)
			default:
				break drain
			}
		}

		// Merge all entries.
		var merged []segment.Entry
		for i := range reqs {
			merged = append(merged, reqs[i].entries...)
		}

		// Acquire publish lock and flush.
		publishCtx, publishTask := trace.NewTask(ctx, "hydra/opfs-blockshard/run-actor/publish")
		release, err := shard.AcquirePublishLock()
		if err != nil {
			publishTask.End()
			for _, r := range reqs {
				r.err <- errors.Wrap(err, "acquire publish lock")
			}
			reqs = reqs[:0]
			continue
		}

		err = shard.Publish(publishCtx, merged)
		if err == nil {
			_, err = shard.ReclaimPendingDelete()
		}
		gen := shard.Manifest().Generation
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

		// Pipeline overlap: drain entries that arrived during publish.
		// If any accumulated, the next loop iteration skips the blocking
		// wait and publishes them immediately. Compaction runs only when
		// there is no pending foreground work.
	overlap:
		for {
			select {
			case req := <-ch:
				reqs = append(reqs, req)
			default:
				break overlap
			}
		}

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
					release()
					if compErr == nil {
						e.broadcaster.Send(shardIdx, compGen)
					}
				}
			}
		}
	}
}

// runInvalidationListener handles BroadcastChannel messages from peer workers.
// When a peer publishes a new shard generation, we refresh our manifest cache.
func (e *Engine) runInvalidationListener(ctx context.Context) {
	defer e.wg.Done()
	for {
		select {
		case msg := <-e.listener.Messages():
			idx := int(msg.ShardID)
			if idx < 0 || idx >= len(e.shards) {
				continue
			}
			shard := e.shards[idx]
			current := shard.Manifest()
			if msg.Generation > current.Generation {
				if _, err := e.refreshShardManifest(idx); err != nil {
					continue
				}
			}
		case <-ctx.Done():
			return
		}
	}
}
