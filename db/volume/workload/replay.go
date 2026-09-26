package workload

import (
	"bytes"
	"context"
	"crypto/sha256"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Target is the storage engine surface a replay drives: the key-value store
// and block store a Volume serves, and its garbage collection journal.
type Target interface {
	kvtx.Store
	block.StoreOps
	// AppendJournal durably journals reference graph changes.
	AppendJournal(ctx context.Context, adds, removes []block_gc.RefEdge) error
	// ReplayJournal passes every journaled change to apply, then removes it.
	ReplayJournal(ctx context.Context, apply func(adds, removes []block_gc.RefEdge) error) error
}

// OrderedJournal is a Target that can journal with write ordering in place of
// a full durability flush, like kvtx.OrderedCommitTx.
type OrderedJournal interface {
	// AppendJournalOrdered journals reference graph changes with write
	// ordering only.
	AppendJournalOrdered(ctx context.Context, adds, removes []block_gc.RefEdge) error
}

// DiscardJournal accepts replayed journal changes without applying them. A
// replayed trace already carries the reference graph's own key-value writes.
func DiscardJournal(adds, removes []block_gc.RefEdge) error {
	return nil
}

// headSuffix ends the key of a shared object's head, whose commit publishes
// the blocks and state before it.
const headSuffix = "/host"

// Result reports one replay.
type Result struct {
	// Ops is the number of records replayed.
	Ops int
	// Wall is the replay's elapsed time.
	Wall time.Duration
	// Latency holds each timed operation's latencies in replay order: commit,
	// sync, put, put-batch, get-block, journal-append, and journal-replay.
	Latency map[Op][]time.Duration
	// CommitErrors counts commits the target rejected. A replay that commits
	// transactions the recording interleaved may conflict where it did not.
	CommitErrors int
	// OrderedCommits counts commits and journal appends made with write
	// ordering only.
	OrderedCommits int
	// BlockBytes is the payload written by block puts.
	BlockBytes int64
	// ValueBytes is the payload written by key-value sets.
	ValueBytes int64
}

// Percentile returns the p-th percentile latency of op, or zero when op was
// never timed.
func (r *Result) Percentile(op Op, p float64) time.Duration {
	samples := slices.Clone(r.Latency[op])
	if len(samples) == 0 {
		return 0
	}
	slices.Sort(samples)
	return samples[int(p*float64(len(samples)-1))]
}

// Replay replays a recorded workload against a Target. Values and block
// payloads are regenerated from their recorded lengths; a block keeps one
// deterministic payload, so its replay reference is stable across operations
// even though it differs from the recorded one.
type Replay struct {
	// records is the recorded workload in trace order.
	records []Record
	// blocks maps a recorded block key to its regenerated payload.
	blocks map[string]*replayBlock
	// seedValues holds key-value entries the workload reads before writing.
	seedValues map[string]int
	// seedBlocks holds blocks the workload reads before writing.
	seedBlocks []*replayBlock
	// values is a shared random buffer sliced for key-value values.
	values []byte

	// Ordered commits every key-value commit that writes no shared object head,
	// and every journal append, with write ordering only, so only block Sync
	// and head commits flush.
	Ordered bool
}

// replayBlock is one regenerated block.
type replayBlock struct {
	// ref is the regenerated payload's reference.
	ref *block.BlockRef
	// data is the regenerated payload.
	data []byte
}

// defaultBlockSize is the size of a block the workload only tests for.
const defaultBlockSize = 1024

// defaultValueSize is the size of a value the workload only tests for.
const defaultValueSize = 64

// seedBatch bounds the entries of one seeding batch or transaction.
const seedBatch = 1024

// NewReplay prepares a replay of records, regenerating every block payload and
// finding the state the workload reads before writing it.
func NewReplay(records []Record) (*Replay, error) {
	r := &Replay{
		records:    records,
		blocks:     make(map[string]*replayBlock),
		seedValues: make(map[string]int),
	}

	// Size every block from its first known length.
	sizes := make(map[string]int64)
	maxValue := defaultValueSize
	for _, rec := range records {
		switch rec.Op {
		case OpPut, OpGetBlock, OpStatBlock:
			if _, ok := sizes[string(rec.Key)]; !ok && rec.Size > 0 {
				sizes[string(rec.Key)] = rec.Size
			}
		case OpSet, OpGet:
			if rec.Op == OpGet && rec.Size == -1 {
				continue
			}
			size, err := replaySize(rec.Size)
			if err != nil {
				return nil, err
			}
			maxValue = max(maxValue, size)
		}
	}
	r.values = randomBytes([]byte("values"), maxValue)

	// Seed what the workload finds before it writes it.
	written := make(map[string]bool)
	writtenBlocks := make(map[string]bool)
	prefixes := make(map[uint64][]byte)
	for _, rec := range records {
		key := string(rec.Key)
		switch rec.Op {
		case OpSet, OpDelete:
			written[key] = true
		case OpGet, OpExists:
			if written[key] || rec.Size <= 0 {
				continue
			}
			size := defaultValueSize
			if rec.Op == OpGet {
				var err error
				size, err = replaySize(rec.Size)
				if err != nil {
					return nil, err
				}
			}
			r.seedValues[key] = max(r.seedValues[key], size)
		case OpIterate:
			prefixes[rec.ID] = rec.Key
		case OpIterEnd:
			size, err := replaySize(rec.Size)
			if err != nil {
				return nil, err
			}
			r.seedPrefix(prefixes[rec.ID], size, written)
		case OpPut:
			writtenBlocks[key] = true
		case OpGetBlock, OpStatBlock, OpBlockExists:
			if writtenBlocks[key] || rec.Size <= 0 {
				continue
			}
			b, err := r.block(rec.Key, sizes)
			if err != nil {
				return nil, err
			}
			writtenBlocks[key] = true
			r.seedBlocks = append(r.seedBlocks, b)
		}
	}

	// Regenerate every remaining block the workload names.
	for _, rec := range records {
		switch rec.Op {
		case OpPut, OpTombstone, OpRm, OpGetBlock, OpStatBlock, OpBlockExists:
			if _, err := r.block(rec.Key, sizes); err != nil {
				return nil, err
			}
		}
	}
	return r, nil
}

// seedPrefix seeds synthetic keys under prefix until an iterator over it can
// visit n entries.
func (r *Replay) seedPrefix(prefix []byte, n int, written map[string]bool) {
	have := 0
	for key := range written {
		if bytes.HasPrefix([]byte(key), prefix) {
			have++
		}
	}
	for key := range r.seedValues {
		if !written[key] && bytes.HasPrefix([]byte(key), prefix) {
			have++
		}
	}
	for i := 0; have < n; i++ {
		key := string(prefix) + "\xffreplay-" + strconv.Itoa(i)
		if _, ok := r.seedValues[key]; ok {
			continue
		}
		r.seedValues[key] = defaultValueSize
		have++
	}
}

// block returns the regenerated block for a recorded key.
func (r *Replay) block(key []byte, sizes map[string]int64) (*replayBlock, error) {
	// Reuse the first regenerated payload for each recorded block key.
	if b := r.blocks[string(key)]; b != nil {
		return b, nil
	}

	// Validate the recorded length before generating and hashing the payload.
	size := sizes[string(key)]
	if size <= 0 {
		size = defaultBlockSize
	}
	n, err := replaySize(size)
	if err != nil {
		return nil, err
	}
	data := randomBytes(key, n)
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		return nil, err
	}
	b := &replayBlock{ref: ref, data: data}
	r.blocks[string(key)] = b
	return b, nil
}

// Seed writes the state the workload reads before writing it, then syncs.
func (r *Replay) Seed(ctx context.Context, t Target) error {
	// Write the seeded blocks in bounded batches.
	for chunk := range slices.Chunk(r.seedBlocks, seedBatch) {
		entries := make([]*block.PutBatchEntry, len(chunk))
		for i, b := range chunk {
			entries[i] = &block.PutBatchEntry{Ref: b.ref, Data: b.data}
		}
		if err := t.PutBlockBatch(ctx, entries); err != nil {
			return errors.Wrap(err, "seed blocks")
		}
	}
	if _, err := t.Sync(ctx); err != nil {
		return errors.Wrap(err, "seed sync")
	}

	// Write the seeded values in bounded transactions.
	keys := make([]string, 0, len(r.seedValues))
	for key := range r.seedValues {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for chunk := range slices.Chunk(keys, seedBatch) {
		if err := r.seedValueChunk(ctx, t, chunk); err != nil {
			return err
		}
	}
	return nil
}

// seedValueChunk writes one transaction of seeded values.
func (r *Replay) seedValueChunk(ctx context.Context, t Target, keys []string) error {
	tx, err := t.NewTransaction(ctx, true)
	if err != nil {
		return errors.Wrap(err, "seed values")
	}
	defer tx.Discard()
	for _, key := range keys {
		if err := tx.Set(ctx, []byte(key), r.values[:r.seedValues[key]]); err != nil {
			return errors.Wrap(err, "seed value")
		}
	}
	return errors.Wrap(tx.Commit(ctx), "seed commit")
}

// Fill writes n filler blocks of size bytes in bounded batches and syncs once,
// so a replay runs against a durable volume of realistic scale.
func Fill(ctx context.Context, t Target, n, size int) error {
	for start := 0; start < n; start += seedBatch {
		entries := make([]*block.PutBatchEntry, min(seedBatch, n-start))
		for i := range entries {
			data := randomBytes([]byte("fill-"+strconv.Itoa(start+i)), size)
			ref, err := block.BuildBlockRef(data, nil)
			if err != nil {
				return err
			}
			entries[i] = &block.PutBatchEntry{Ref: ref, Data: data}
		}
		if err := t.PutBlockBatch(ctx, entries); err != nil {
			return errors.Wrap(err, "fill blocks")
		}
	}
	_, err := t.Sync(ctx)
	return errors.Wrap(err, "fill sync")
}

// Run replays the workload against t in record order on one goroutine, with
// no think time, and reports its timing.
func (r *Replay) Run(ctx context.Context, t Target) (*Result, error) {
	run := &replayRun{
		replay: r,
		target: t,
		txs:    make(map[uint64]kvtx.Tx),
		heads:  make(map[uint64]bool),
		iters:  make(map[uint64]kvtx.Iterator),
		scopes: make(map[uint64]replayScope),
		result: &Result{Latency: make(map[Op][]time.Duration)},
	}
	defer run.release()

	start := time.Now()
	for i, rec := range r.records {
		if err := run.apply(ctx, rec); err != nil {
			return nil, errors.Wrapf(err, "replay record %d (%s)", i, rec.Op)
		}
	}
	run.result.Ops = len(r.records)
	run.result.Wall = time.Since(start)
	return run.result, nil
}

// replayRun is the state of one replay.
type replayRun struct {
	// replay is the prepared workload.
	replay *Replay
	// target receives the operations.
	target Target
	// txs holds open transactions by recorded ID.
	txs map[uint64]kvtx.Tx
	// heads holds the IDs of open transactions that write a shared object
	// head.
	heads map[uint64]bool
	// iters holds open iterators by recorded ID.
	iters map[uint64]kvtx.Iterator
	// scopes holds open read scopes by recorded ID.
	scopes map[uint64]replayScope
	// batch gathers the entries of an open put batch.
	batch *replayBatch
	// exists gathers the references of an open existence batch.
	exists *replayBatch
	// journals numbers the journal entries this replay appended.
	journals int
	// result accumulates the report.
	result *Result
}

// replayScope is an open read scope.
type replayScope struct {
	// ops reads within the scope.
	ops block.StoreOps
	// release ends the scope.
	release func()
}

// replayBatch gathers the recorded entries of one batch call.
type replayBatch struct {
	// id is the recorded batch or scope ID the entries carry.
	id uint64
	// want is the recorded entry count.
	want int64
	// entries holds gathered put entries.
	entries []*block.PutBatchEntry
	// refs holds gathered existence references.
	refs []*block.BlockRef
}

// apply replays one record.
func (run *replayRun) apply(ctx context.Context, rec Record) error {
	switch rec.Op {
	case OpTxRead, OpTxWrite:
		tx, err := run.target.NewTransaction(ctx, rec.Op == OpTxWrite)
		if err != nil {
			return err
		}
		run.txs[rec.ID] = tx
		return nil
	case OpGet, OpExists, OpSet, OpDelete, OpIterate, OpCommit, OpDiscard:
		return run.applyTx(ctx, rec)
	case OpSeek:
		if it := run.iters[rec.ID]; it != nil {
			return it.Seek(rec.Key)
		}
		return nil
	case OpIterEnd:
		it := run.iters[rec.ID]
		if it == nil {
			return nil
		}
		delete(run.iters, rec.ID)
		defer it.Close()
		for n := int64(0); n < rec.Size && it.Next(); n++ {
			if _, err := it.Value(); err != nil {
				return err
			}
		}
		return it.Err()
	case OpJournalAppend:
		return run.appendJournal(ctx, rec.Size)
	case OpJournalReplay:
		return run.timed(OpJournalReplay, func() error { return run.target.ReplayJournal(ctx, DiscardJournal) })
	}
	return run.applyBlock(ctx, rec)
}

// applyTx replays one operation of an open transaction.
func (run *replayRun) applyTx(ctx context.Context, rec Record) error {
	id := rec.ID
	if rec.Op == OpIterate {
		id = rec.Parent
	}
	tx, err := run.tx(ctx, id)
	if err != nil {
		return err
	}
	switch rec.Op {
	case OpGet:
		_, _, err = tx.Get(ctx, rec.Key)
		return err
	case OpExists:
		_, err = tx.Exists(ctx, rec.Key)
		return err
	case OpSet:
		run.result.ValueBytes += rec.Size
		run.heads[id] = run.heads[id] || bytes.HasSuffix(rec.Key, []byte(headSuffix))
		return tx.Set(ctx, rec.Key, run.replay.values[:rec.Size])
	case OpDelete:
		run.heads[id] = run.heads[id] || bytes.HasSuffix(rec.Key, []byte(headSuffix))
		return tx.Delete(ctx, rec.Key)
	case OpIterate:
		run.iters[rec.ID] = tx.Iterate(ctx, rec.Key, true, rec.Size == 1)
		return nil
	case OpCommit:
		if err := run.timed(OpCommit, func() error { return run.commit(ctx, tx, run.heads[id]) }); err != nil {
			run.result.CommitErrors++
		}
		return nil
	}
	tx.Discard()
	delete(run.txs, id)
	delete(run.heads, id)
	return nil
}

// commit commits tx, with write ordering only under the ordered policy unless
// it writes a head.
func (run *replayRun) commit(ctx context.Context, tx kvtx.Tx, head bool) error {
	if !run.replay.Ordered || head {
		return tx.Commit(ctx)
	}
	run.result.OrderedCommits++
	return kvtx.CommitOrdered(ctx, tx)
}

// tx returns the open transaction id. A transaction the trace began inside
// opens here as a write transaction, which serves every operation it records.
func (run *replayRun) tx(ctx context.Context, id uint64) (kvtx.Tx, error) {
	if tx := run.txs[id]; tx != nil {
		return tx, nil
	}
	tx, err := run.target.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	run.txs[id] = tx
	return tx, nil
}

// applyBlock replays one block store operation.
func (run *replayRun) applyBlock(ctx context.Context, rec Record) error {
	b := run.replay.blocks[string(rec.Key)]
	switch rec.Op {
	case OpPutBatch:
		run.batch = &replayBatch{id: rec.ID, want: rec.Size}
		return run.flushBatch(ctx)
	case OpPut, OpTombstone:
		if rec.ID != 0 && run.batch != nil && run.batch.id == rec.ID {
			entry := &block.PutBatchEntry{Ref: b.ref, Tombstone: rec.Op == OpTombstone}
			if !entry.Tombstone {
				entry.Data = b.data
				run.result.BlockBytes += int64(len(b.data))
			}
			run.batch.entries = append(run.batch.entries, entry)
			return run.flushBatch(ctx)
		}
		run.result.BlockBytes += int64(len(b.data))
		return run.timed(OpPut, func() error {
			_, _, err := run.target.PutBlock(ctx, b.data, nil)
			return err
		})
	case OpRm:
		return run.target.RmBlock(ctx, b.ref)
	case OpSync:
		return run.timed(OpSync, func() error {
			_, err := run.target.Sync(ctx)
			return err
		})
	case OpReadBegin:
		ops, release, err := run.target.BeginReadOperation(ctx)
		if err != nil {
			return err
		}
		run.scopes[rec.ID] = replayScope{ops: ops, release: release}
		return nil
	case OpReadEnd:
		if scope, ok := run.scopes[rec.ID]; ok {
			scope.release()
			delete(run.scopes, rec.ID)
		}
		return nil
	case OpExistsBatch:
		run.exists = &replayBatch{id: rec.ID, want: rec.Size}
		return run.flushExists(ctx)
	}

	// Read through the record's scope, or the store for direct calls.
	var ops block.StoreOps = run.target
	if scope, ok := run.scopes[rec.ID]; ok {
		ops = scope.ops
	}
	switch rec.Op {
	case OpGetBlock:
		return run.timed(OpGetBlock, func() error {
			_, _, err := ops.GetBlock(ctx, b.ref)
			return err
		})
	case OpStatBlock:
		_, err := ops.StatBlock(ctx, b.ref)
		return err
	case OpBlockExists:
		if run.exists != nil && run.exists.id == rec.ID {
			run.exists.refs = append(run.exists.refs, b.ref)
			return run.flushExists(ctx)
		}
		_, err := ops.GetBlockExists(ctx, b.ref)
		return err
	}
	return errors.Errorf("unknown operation %q", rec.Op)
}

// flushBatch writes the open put batch once it holds every recorded entry.
func (run *replayRun) flushBatch(ctx context.Context) error {
	batch := run.batch
	if int64(len(batch.entries)) < batch.want {
		return nil
	}
	run.batch = nil
	return run.timed(OpPutBatch, func() error { return run.target.PutBlockBatch(ctx, batch.entries) })
}

// flushExists checks the open existence batch once it holds every reference.
func (run *replayRun) flushExists(ctx context.Context) error {
	batch := run.exists
	if int64(len(batch.refs)) < batch.want {
		return nil
	}
	run.exists = nil
	var ops block.StoreOps = run.target
	if scope, ok := run.scopes[batch.id]; ok {
		ops = scope.ops
	}
	_, err := ops.GetBlockExistsBatch(ctx, batch.refs)
	return err
}

// journalEdgeBytes approximates one generated edge's encoded length.
const journalEdgeBytes = 256

// journalPadding pads generated edge objects to journalEdgeBytes.
var journalPadding = strings.Repeat("o", journalEdgeBytes)

// appendJournal journals generated edges of about size encoded bytes.
func (run *replayRun) appendJournal(ctx context.Context, size int64) error {
	var adds []block_gc.RefEdge
	for n := int64(0); n < size; n += journalEdgeBytes {
		run.journals++
		subject := "replay/" + strconv.Itoa(run.journals)
		adds = append(adds, block_gc.RefEdge{Subject: subject, Object: journalPadding[:journalEdgeBytes-len(subject)-8]})
	}
	return run.timed(OpJournalAppend, func() error {
		if oj, ok := run.target.(OrderedJournal); ok && run.replay.Ordered {
			run.result.OrderedCommits++
			return oj.AppendJournalOrdered(ctx, adds, nil)
		}
		return run.target.AppendJournal(ctx, adds, nil)
	})
}

// timed runs fn and records its latency under op.
func (run *replayRun) timed(op Op, fn func() error) error {
	start := time.Now()
	err := fn()
	run.result.Latency[op] = append(run.result.Latency[op], time.Since(start))
	return err
}

// release closes everything the recording left open.
func (run *replayRun) release() {
	for _, it := range run.iters {
		it.Close()
	}
	for _, scope := range run.scopes {
		scope.release()
	}
	for _, tx := range run.txs {
		tx.Discard()
	}
}

// replaySize converts a recorded allocation size or entry count to a native int.
func replaySize(size int64) (int, error) {
	// Recorded sizes must fit the host allocation and indexing range.
	if size < 0 || size > math.MaxInt {
		return 0, errors.Errorf("workload replay size out of range: %d", size)
	}

	// Narrow only after both bounds have been checked.
	return int(size), nil
}

// randomBytes returns size deterministic pseudorandom bytes seeded by seed.
func randomBytes(seed []byte, size int) []byte {
	rng := rand.NewChaCha8(sha256.Sum256(seed))
	out := make([]byte, size)
	_, _ = rng.Read(out) // ChaCha8.Read always fills out and returns a nil error.
	return out
}
