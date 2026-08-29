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

// writeReq is a wake signal to the shard write actor.
//
// The published content always comes from the shard pending buffer, never from
// the request: putToActors inserts entries into the buffer before sending the
// wake, and the actor publishes a snapshot of the buffer. A barrier carries no
// new write; it is a durability fence the actor satisfies after the publish
// covering all earlier-enqueued writes for the shard completes. Foreground
// requests wait on err; background requests are fire-and-forget (err is drained
// by the actor but unread), so a background write returns before it is durable
// and is fenced only by Sync.
type writeReq struct {
	background bool
	barrier    bool
	err        chan error
}

// pendingEntry is a buffered, not-yet-published write held for read-through.
type pendingEntry struct {
	value     []byte
	tombstone bool
	seq       uint64
}

// pendingBuffer is a shard's in-memory buffer of accepted-but-unpublished
// writes. It is the read-through source so a just-enqueued block reads back
// before its publish, and the publish source so a failed publish keeps the
// entries durable-pending until a later cycle republishes them. Entries are
// keyed by segment key; a later write to the same key supersedes the earlier
// one by carrying a higher seq, and a read always sees the latest.
type pendingBuffer struct {
	mu      sync.Mutex
	seq     uint64
	entries map[string]pendingEntry
}

func newPendingBuffer() *pendingBuffer {
	return &pendingBuffer{entries: make(map[string]pendingEntry)}
}

// insert buffers entries, assigning each a monotonic seq so a later write to the
// same key supersedes an earlier one.
func (p *pendingBuffer) insert(entries []segment.Entry) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i := range entries {
		p.seq++
		p.entries[string(entries[i].Key)] = pendingEntry{
			value:     entries[i].Value,
			tombstone: entries[i].Tombstone,
			seq:       p.seq,
		}
	}
}

// get returns the buffered entry for key, if any.
func (p *pendingBuffer) get(key []byte) (pendingEntry, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e, ok := p.entries[string(key)]
	return e, ok
}

// snapshot copies the buffered entries for a publish, returning the entries to
// write and the per-key seq that publish covers (for matched eviction).
func (p *pendingBuffer) snapshot() ([]segment.Entry, map[string]uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.entries) == 0 {
		return nil, nil
	}
	entries := make([]segment.Entry, 0, len(p.entries))
	seqs := make(map[string]uint64, len(p.entries))
	for key, e := range p.entries {
		entries = append(entries, segment.Entry{
			Key:       []byte(key),
			Value:     e.value,
			Tombstone: e.tombstone,
		})
		seqs[key] = e.seq
	}
	return entries, seqs
}

// evict drops keys whose seq still matches what the completed publish covered.
// A newer write (higher seq) that arrived during the publish is kept so the next
// cycle republishes it.
func (p *pendingBuffer) evict(seqs map[string]uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, seq := range seqs {
		if e, ok := p.entries[key]; ok && e.seq == seq {
			delete(p.entries, key)
		}
	}
}

// length reports the number of buffered keys (bounded by in-flight writes).
func (p *pendingBuffer) length() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries)
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

// Engine is the multi-shard block store engine.
type Engine struct {
	shards      []*Shard
	actors      []*shardActor
	pending     []*pendingBuffer
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	compactionN int
	maxEntryN   int
	broadcaster *Broadcaster
	listener    *Listener
	cache       *cacheCoordinator
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
	// Normalize settings and derive the engine cancellation context.
	settings = normalizeSettings(settings)
	ctx, cancel := context.WithCancel(ctx)

	// Allocate shard state and one shared cache budget.
	cache := newDefaultCacheCoordinator()
	e := &Engine{
		shards:      make([]*Shard, settings.ShardCount),
		actors:      make([]*shardActor, settings.ShardCount),
		pending:     make([]*pendingBuffer, settings.ShardCount),
		ctx:         ctx,
		cancel:      cancel,
		compactionN: settings.CompactionTrigger,
		maxEntryN:   settings.MaxEntryValueBytes,
		broadcaster: NewBroadcaster(lockPrefix),
		listener:    NewListener(lockPrefix),
		cache:       cache,
	}

	// Open and recover every shard before starting its write actor.
	for i := range e.shards {
		// Bind one immutable shard directory to the engine cache.
		name := "shard-" + zeroPad(uint64(i), 2)
		shardDir, err := opfs.GetDirectory(dir, name, true)
		if err != nil {
			cancel()
			return nil, errors.Errorf("create shard %d directory: %v", i, err)
		}
		shard, err := newShard(i, shardDir, lockPrefix, settings, cache)
		if err != nil {
			cancel()
			return nil, errors.Errorf("open shard %d: %v", i, err)
		}

		// Recover pending deletion and orphan state under the publish lock.
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

		// Publish recovered shard state before starting its actor.
		e.shards[i] = shard
		e.actors[i] = newShardActor(i, shard)
		e.pending[i] = newPendingBuffer()
		e.wg.Add(1)
		go e.runActor(ctx, e.actors[i])
	}

	// Start the invalidation listener after all shard actors are running.
	e.wg.Add(1)
	go e.runInvalidationListener(ctx)

	return e, nil
}

// Close stops engine activity and releases cache and broadcast resources.
func (e *Engine) Close() {
	// Stop engine goroutines and their cross-runtime broadcasts.
	e.cancel()
	e.wg.Wait()
	e.broadcaster.Close()

	// Wait for active lookups and release every cached file handle.
	e.cache.close()

	// Release the invalidation listener after cache shutdown.
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

// Sync blocks until every write enqueued before the call is durable on every
// shard. It dispatches a barrier request to each shard write actor and waits
// for all replies. The actor satisfies a barrier only after the publish
// covering earlier-enqueued writes for that shard completes; an idle shard
// replies immediately without emitting an empty generation.
func (e *Engine) Sync(ctx context.Context) error {
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/sync")
	defer task.End()

	var wg sync.WaitGroup
	errs := make([]error, len(e.actors))
	for i := range e.actors {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ch := make(chan error, 1)
			actor := e.actors[idx]
			select {
			case actor.foreground <- writeReq{barrier: true, err: ch}:
			case <-ctx.Done():
				errs[idx] = ctx.Err()
				return
			case <-e.ctx.Done():
				errs[idx] = context.Canceled
				return
			}
			select {
			case errs[idx] = <-ch:
			case <-ctx.Done():
				errs[idx] = ctx.Err()
			case <-e.ctx.Done():
				errs[idx] = context.Canceled
			}
		}(i)
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// CompactOnce runs at most one compaction plan per shard.
//
// Compaction is storage maintenance, so foreground writes do not run it
// inline. The caller's background lifecycle invokes CompactOnce, handles its
// errors, and cancels ctx during shutdown.
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

	// Buffer every write for read-through before waking the actor, so a reader
	// in the same worker sees it immediately even though the publish is async.
	for idx := range buckets {
		if len(buckets[idx]) != 0 {
			e.pending[idx].insert(buckets[idx])
		}
	}

	if background {
		// Fire-and-forget: wake each actor and return before the publish. The
		// pending buffer holds the entries for read-through, and Sync fences
		// their durability.
		for idx := range buckets {
			if len(buckets[idx]) == 0 {
				continue
			}
			_, reqTask := trace.NewTask(taskCtx, tracePrefix+"/queue-request")
			actor := e.actors[idx]
			select {
			case actor.background <- writeReq{background: true, err: make(chan error, 1)}:
				reqTask.End()
			case <-ctx.Done():
				reqTask.End()
				return ctx.Err()
			case <-e.ctx.Done():
				reqTask.End()
				return context.Canceled
			}
		}
		return nil
	}

	// Foreground: wake each actor and wait for the publish covering this write.
	var wg sync.WaitGroup
	errs := make([]error, len(e.shards))
	for i := range buckets {
		if len(buckets[i]) == 0 {
			continue
		}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ch := make(chan error, 1)
			reqCtx, reqTask := trace.NewTask(taskCtx, tracePrefix+"/queue-request")
			actor := e.actors[idx]
			select {
			case actor.foreground <- writeReq{err: ch}:
				reqTask.End()
			case <-ctx.Done():
				reqTask.End()
				errs[idx] = ctx.Err()
				return
			case <-e.ctx.Done():
				reqTask.End()
				errs[idx] = context.Canceled
				return
			}
			_, waitTask := trace.NewTask(reqCtx, tracePrefix+"/wait-request")
			select {
			case errs[idx] = <-ch:
				waitTask.End()
			case <-ctx.Done():
				waitTask.End()
				errs[idx] = ctx.Err()
			case <-e.ctx.Done():
				waitTask.End()
				errs[idx] = context.Canceled
			}
		}(i)
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
	candidateSegments := 0
	acquiredSegments := 0
	pendingHit := 0
	foundPublished := 0
	// manifestSegments stays zero when pending read-through avoids a manifest scan.
	manifestSegments := 0
	retriedInt := 0
	if retried {
		retriedInt = 1
	}
	defer func() {
		trace.Logf(ctx, "hydra/opfs-blockshard/get-from-shard/shape", "manifest_segments=%d candidates=%d acquisitions=%d pending_hit=%d found_published=%d retried=%d", manifestSegments, candidateSegments, acquiredSegments, pendingHit, foundPublished, retriedInt)
		task.End()
	}()

	// Pending-then-published: a buffered (not-yet-published) write or tombstone
	// is newer than anything in the manifest and wins.
	if pe, ok := e.pending[shardIdx].get(key); ok {
		pendingHit = 1
		if pe.tombstone {
			return nil, false, nil
		}
		return pe.value, true, nil
	}

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
	manifestSegments = len(m.Segments)
	for i := len(m.Segments) - 1; i >= 0; i-- {
		seg := &m.Segments[i]
		// Skip segments whose immutable key range excludes the request.
		if string(key) < string(seg.MinKey) || string(key) > string(seg.MaxKey) {
			continue
		}

		candidateSegments++
		// Pin cache resources for this segment lookup step.
		taskCtx, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/get-from-shard/acquire-segment")
		lease, err := shard.acquireSegment(taskCtx, seg)
		subtask.End()
		if err != nil {
			if e.shouldRetryAfterRefresh(ctx, shardIdx, m.Generation, retried, err) {
				return e.getFromShard(ctx, shardIdx, key, true)
			}
			return nil, false, errors.Errorf("acquire segment %s: %v", seg.Filename, err)
		}
		acquiredSegments++

		// Locate the key and release every segment resource pin.
		taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/get-from-shard/locate")
		val, found, tombstone, err := lease.lookup.Locate(lease, key, true)
		lease.Release()
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
			foundPublished = 1
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
	if pe, ok := e.pending[shardIdx].get(key); ok {
		return !pe.tombstone, nil
	}
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

		// Pin cache resources while checking this segment.
		lease, err := shard.acquireSegment(ctx, seg)
		if err != nil {
			if opfs.IsNotFound(err) {
				shard.dropSegmentFile(seg.Filename)
			}
			if e.shouldRetryAfterRefresh(ctx, shardIdx, m.Generation, retried, err) {
				return e.getExistsFromShard(shardIdx, key, true)
			}
			return false, errors.Errorf("acquire segment %s: %v", seg.Filename, err)
		}
		_, found, tombstone, err := lease.lookup.Locate(lease, key, false)
		lease.Release()
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
	if pe, ok := e.pending[shardIdx].get(key); ok {
		if pe.tombstone {
			return 0, false, nil
		}
		return int64(len(pe.value)), true, nil
	}
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

		// Pin cache resources while reading value metadata.
		lease, err := shard.acquireSegment(ctx, seg)
		if err != nil {
			if opfs.IsNotFound(err) {
				shard.dropSegmentFile(seg.Filename)
			}
			if e.shouldRetryAfterRefresh(ctx, shardIdx, m.Generation, retried, err) {
				return e.statFromShard(ctx, shardIdx, key, true)
			}
			return 0, false, errors.Errorf("acquire segment %s: %v", seg.Filename, err)
		}
		stat, err := lease.lookup.Stat(lease, key)
		lease.Release()
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

	// Pending-then-published: resolve buffered keys before scanning segments so
	// an in-flight write or tombstone wins over an older published value.
	for i, key := range keys {
		if len(key) == 0 {
			continue
		}
		if pe, ok := e.pending[shardIdx].get(key); ok {
			out[i] = !pe.tombstone
			resolved[i] = true
		}
	}
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

		// Pin cache resources for the batched segment step.
		lease, err := shard.acquireSegment(ctx, seg)
		if err != nil {
			if opfs.IsNotFound(err) {
				shard.dropSegmentFile(seg.Filename)
			}
			if e.shouldRetryAfterRefresh(ctx, shardIdx, m.Generation, retried, err) {
				return e.getExistsBatchFromShard(ctx, shardIdx, keys, true)
			}
			return nil, errors.Errorf("acquire segment %s: %v", seg.Filename, err)
		}
		if err := ctx.Err(); err != nil {
			lease.Release()
			return nil, err
		}
		candidateKeys := make([][]byte, len(candidates))
		for i, j := range candidates {
			candidateKeys[i] = keys[j]
		}
		results, err := lease.lookup.LocateBatch(lease, candidateKeys, false)
		lease.Release()
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

// bgCoalesceTargetEntries is the per-shard pending depth a background-only
// publish cycle accumulates toward before flushing. A serial PutBackground feed
// wakes the actor once per block, so without coalescing the actor publishes one
// or two entries per cycle; draining toward this depth batches the feed into far
// fewer publishes. The target stays well below the full in-flight buffer because
// an oversized segment write costs more than the fixed per-publish lock and
// manifest tax it saves.
const bgCoalesceTargetEntries = 32

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
// singleton puts. A background-only cycle additionally accumulates toward
// bgCoalesceTargetEntries before flushing, so a serial PutBackground feed
// batches into deep publishes instead of one per block; foreground and barrier
// cycles never wait on depth.
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

		// Classify the cycle. A foreground write's publish latency must be
		// protected; a Sync barrier must fence earlier background writes now; a
		// background-only cycle has neither constraint and may coalesce deeply.
		barrierPresent := hasBarrier(reqs)
		cycleHasForeground := hasForeground(reqs)
		backgroundOnly := !barrierPresent && !cycleHasForeground

		// Drain the background channel into this cycle when the cycle is
		// background-only (a serial PutBackground feed coalesces instead of
		// waking the actor per block), when sustained foreground has starved
		// background, or when a barrier must fence pending background writes.
		hasBg := len(bgCh) > 0
		drainBg := backgroundOnly || (hasBg && (fgOnly >= bgStarvationLimit || barrierPresent))
		if drainBg {
			e.drainCh(bgCh, &reqs)
			fgOnly = 0
		} else if cycleHasForeground && !hasBg {
			fgOnly++
		}

		// Coalescing yield-drain loop: repeat yield+drain until no new requests
		// arrive or maxCoalesceRounds is reached. Singleton puts (nothing queued
		// after the first round) publish immediately. A background-only cycle
		// keeps draining until the shard buffer reaches bgCoalesceTargetEntries,
		// letting a serial producer fill the buffer before the publish; the
		// background channel is drained only when the background-only,
		// starvation, or barrier condition was met for this cycle, so background
		// entries cannot inflate foreground publish latency.
		for range maxCoalesceRounds {
			runtime.Gosched()
			prevLen := len(reqs)
			e.drainCh(fgCh, &reqs)
			if drainBg {
				e.drainCh(bgCh, &reqs)
			}
			if backgroundOnly && e.pending[shardIdx].length() >= bgCoalesceTargetEntries {
				break
			}
			if len(reqs) == prevLen {
				break
			}
		}

		// The published content is the shard pending-buffer snapshot, not the
		// requests: writers buffer entries before waking the actor. A barrier
		// carries no entry, and a write already published by an earlier
		// coalesced cycle is no longer buffered, so an empty snapshot means the
		// fence is already satisfied: reply without an empty publish.
		hasForegroundReq := hasForeground(reqs)

		snapshot, snapshotSeqs := e.pending[shardIdx].snapshot()
		if len(snapshot) == 0 {
			currentReqs := reqs
			reqs = nil
			for _, r := range currentReqs {
				r.err <- nil
			}
			continue
		}

		// Acquire publish lock and flush.
		publishCtx, publishTask := trace.NewTask(ctx, "hydra/opfs-blockshard/run-actor/publish")
		trace.Log(publishCtx, "coalesce", "reqs="+strconv.Itoa(len(reqs))+" entries="+strconv.Itoa(len(snapshot)))
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
		err = shard.Publish(writeCtx, snapshot)
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

		// Evict only on a durable publish, matching seq so a write that arrived
		// during the publish stays buffered for the next cycle. A failed publish
		// keeps every entry buffered (readable and retried), so durability is
		// never silently lost. Broadcast peer invalidation only on success.
		if err == nil {
			e.pending[shardIdx].evict(snapshotSeqs)
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

// hasBarrier reports whether any request in reqs is a Sync durability fence.
func hasBarrier(reqs []writeReq) bool {
	for i := range reqs {
		if reqs[i].barrier {
			return true
		}
	}
	return false
}

// hasForeground reports whether any request in reqs is a foreground write, whose
// publish latency the actor must protect. A background write or a Sync barrier is
// not a foreground write.
func hasForeground(reqs []writeReq) bool {
	for i := range reqs {
		if !reqs[i].barrier && !reqs[i].background {
			return true
		}
	}
	return false
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
