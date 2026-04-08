//go:build js

package blockshard

import (
	"context"
	"hash/fnv"
	"runtime"
	"sync"
	"syscall/js"
	"time"

	"github.com/aperturerobotics/hydra/opfs"
	"github.com/aperturerobotics/hydra/volume/js/opfs/segment"
	"github.com/pkg/errors"
)

// DefaultShardCount is the default number of block shards.
const DefaultShardCount = 4

// DefaultFlushThreshold is the default entry count that triggers a flush.
const DefaultFlushThreshold = 4

// DefaultFlushMaxAge is the default maximum age before flushing.
const DefaultFlushMaxAge = 50 * time.Millisecond

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
	flushN      int
	flushAge    time.Duration
	broadcaster *Broadcaster
	listener    *Listener
}

// NewEngine creates a new block shard engine in the given OPFS directory.
// It creates shard subdirectories and starts per-shard write actors.
func NewEngine(ctx context.Context, dir js.Value, lockPrefix string, shardCount int) (*Engine, error) {
	if shardCount < 1 {
		shardCount = DefaultShardCount
	}

	ctx, cancel := context.WithCancel(ctx)
	e := &Engine{
		shards:      make([]*Shard, shardCount),
		actors:      make([]chan writeReq, shardCount),
		cancel:      cancel,
		flushN:      DefaultFlushThreshold,
		flushAge:    DefaultFlushMaxAge,
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
		shard, err := NewShard(i, shardDir, lockPrefix)
		if err != nil {
			cancel()
			return nil, errors.Errorf("open shard %d: %v", i, err)
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
	if len(entries) == 0 {
		return nil
	}

	// Partition by shard.
	buckets := make([][]segment.Entry, len(e.shards))
	for i := range entries {
		idx := e.ShardForKey(entries[i].Key)
		buckets[idx] = append(buckets[idx], entries[i])
	}

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
			select {
			case e.actors[idx] <- writeReq{entries: b, err: ch}:
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
	shard := e.shards[shardIdx]
	m := shard.Manifest()

	// Scan segments newest-first (last in manifest = newest).
	for i := len(m.Segments) - 1; i >= 0; i-- {
		seg := &m.Segments[i]
		// Range check.
		if string(key) < string(seg.MinKey) || string(key) > string(seg.MaxKey) {
			continue
		}
		sr, err := OpenSegment(shard.dir, seg.Filename)
		if err != nil {
			return nil, false, errors.Errorf("open segment %s: %v", seg.Filename, err)
		}
		val, found, err := sr.Get(key)
		if err != nil {
			return nil, false, err
		}
		if found {
			return val, true, nil
		}
	}
	return nil, false, nil
}

// Get looks up a key across all shards.
func (e *Engine) Get(key []byte) ([]byte, bool, error) {
	idx := e.ShardForKey(key)
	return e.GetFromShard(idx, key)
}

// runActor is the per-shard write actor goroutine.
// It coalesces queued writes using yield + non-blocking drain.
func (e *Engine) runActor(ctx context.Context, shardIdx int) {
	defer e.wg.Done()
	ch := e.actors[shardIdx]
	shard := e.shards[shardIdx]

	for {
		// Block for the first request.
		var reqs []writeReq
		select {
		case req := <-ch:
			reqs = append(reqs, req)
		case <-ctx.Done():
			return
		}

		// Yield so sibling goroutines can enqueue.
		runtime.Gosched()

		// Non-blocking drain up to flush threshold.
		maxDrain := e.flushN - len(reqs)
		if maxDrain < 0 {
			maxDrain = 0
		}
	drain:
		for range maxDrain {
			select {
			case req := <-ch:
				reqs = append(reqs, req)
			default:
				break drain
			}
		}

		// If we haven't hit the threshold, wait up to flushAge for more.
		entryCount := 0
		for i := range reqs {
			entryCount += len(reqs[i].entries)
		}
		if entryCount < e.flushN {
			timer := time.NewTimer(e.flushAge)
		wait:
			for entryCount < e.flushN {
				select {
				case req := <-ch:
					reqs = append(reqs, req)
					entryCount += len(req.entries)
				case <-timer.C:
					break wait
				case <-ctx.Done():
					timer.Stop()
					for _, r := range reqs {
						r.err <- ctx.Err()
					}
					return
				}
			}
			timer.Stop()
		}

		// Merge all entries.
		var merged []segment.Entry
		for i := range reqs {
			merged = append(merged, reqs[i].entries...)
		}

		// Acquire publish lock and flush.
		release, err := shard.AcquirePublishLock()
		if err != nil {
			for _, r := range reqs {
				r.err <- errors.Wrap(err, "acquire publish lock")
			}
			continue
		}

		err = shard.Publish(merged)
		gen := shard.Manifest().Generation
		release()

		// Broadcast invalidation to peer workers.
		if err == nil {
			e.broadcaster.Send(shardIdx, gen)
		}

		// Reply to all waiters.
		for _, r := range reqs {
			r.err <- err
		}

		// Check if compaction is needed after publish.
		if err == nil {
			plan := PlanCompaction(shard, DefaultL0Trigger)
			if plan != nil {
				release, lockErr := shard.AcquirePublishLock()
				if lockErr == nil {
					compErr := ExecuteCompaction(shard, plan)
					compGen := shard.Manifest().Generation
					release()
					if compErr == nil {
						e.broadcaster.Send(shardIdx, compGen)
						// Delete old segments after compaction.
						names := make([]string, len(plan.InputSegs))
						for i, seg := range plan.InputSegs {
							names[i] = seg.Filename
						}
						deleteRelease, delErr := shard.AcquirePublishLock()
						if delErr == nil {
							DeleteOldSegments(shard, names)
							deleteRelease()
						}
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
				// Re-read manifests from OPFS to pick up the new generation.
				a := readFileBytes(shard.dir, "manifest-a")
				b := readFileBytes(shard.dir, "manifest-b")
				m := PickManifest(a, b)
				if m != nil && m.Generation > current.Generation {
					shard.mu.Lock()
					shard.manifest = m
					shard.mu.Unlock()
				}
			}
		case <-ctx.Done():
			return
		}
	}
}
