//go:build !js && !wasip1 && !windows

package main

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"slices"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/sirupsen/logrus"
)

// syntheticPackBytes bounds a synthetic pack of verified blocks: a
// PackStore batch takes half of it, so a full one holds about 64 MiB of
// blocks.
const syntheticPackBytes = 128 << 20

// lookupChunk is the refs looked up in one index read transaction.
const lookupChunk = 4096

// options configure a rescue run.
type options struct {
	// Bucket is the server directory holding Prefix; Prefix is the key
	// prefix of the source PackStore, whose packs sit at
	// {Bucket}/{Prefix}packs/{id}.
	Bucket, Prefix string
	// Index is the source's pack index, a view under its PackIndexPrefix.
	// It is only read.
	Index kvtx.Store
	// RefList is the list of wanted refs.
	RefList string
	// Ledger is the ledger file.
	Ledger string
	// OutDir is the output pack set's directory.
	OutDir string
	// Sessions is the packs fetched at once.
	Sessions int
	// Plan chooses between whole packs and ranges.
	Plan planOptions
	// MinWanted takes only the packs holding more than MinWanted wanted
	// refs this run; the rest stay unrecorded for a later run.
	MinWanted int
	// Fetch bounds the requests.
	Fetch fetchOptions
	// DiskFloorBytes stops the run when OutDir's file system has less free.
	DiskFloorBytes uint64
	// Progress is the time between progress lines; zero prints none.
	Progress time.Duration
	// FlushInterval is the longest time verified blocks wait for their write
	// and ledger sync; zero writes them only when a synthetic pack fills and
	// at the end. Each flush closes a synthetic pack.
	FlushInterval time.Duration
	// RetryFailed fetches the refs and packs the ledger records as failed.
	RetryFailed bool
	// Log receives the progress lines.
	Log io.Writer
}

// Stop reasons of a run that ended early but cleanly.
const (
	stopByteCap   = "byte-cap"
	stopDiskFloor = "disk-floor"
)

// result is the outcome of a run.
type result struct {
	// Stop is why the run ended early and cleanly, empty when it finished.
	Stop string
	// PacksTotal and PacksDone count this run's packs.
	PacksTotal, PacksDone int
	// Whole and Ranged count this run's packs by plan.
	Whole, Ranged int
	// Done, Failed, Absent and NotInPack count the refs the ledger records.
	// NotInPack are refs the index placed in a pack that lacks them.
	Done, Failed, Absent, NotInPack int
	// Deferred counts the refs of the packs MinWanted left for a later run.
	Deferred int
	// NoExtent counts this run's wanted refs whose h row had no extent;
	// NoExtentDone the ones of them stored.
	NoExtent, NoExtentDone int
	// PackErrors counts the packs whose fetch failed after retries; their
	// refs stay unrecorded for the next run.
	PackErrors int
	// Bytes is the bytes fetched across runs.
	Bytes uint64
	// Requests is this run's request attempts.
	Requests uint64
}

// refReason is a ref that finished without its block, and why.
type refReason struct {
	key    []byte
	state  string
	reason string
}

// packResult is what fetching one pack produced.
type packResult struct {
	id string
	// blocks are the verified blocks; keys their block keys; noExt counts
	// those whose h row had no extent.
	blocks []*block.PutBatchEntry
	keys   [][]byte
	noExt  int
	// finished are the refs that failed.
	finished []refReason
	// complete reports that every wanted ref of the pack is in blocks or
	// finished.
	complete bool
	// bytes is the verified blocks' size, fetched the bytes read for the pack.
	bytes   int64
	fetched uint64
}

// notInPack returns the refs that failed as not in the pack.
func (res *packResult) notInPack() int {
	var n int
	for _, f := range res.finished {
		if f.state == stateNotInPack {
			n++
		}
	}
	return n
}

// rescuer runs one rescue.
type rescuer struct {
	o      *options
	le     *logrus.Entry
	fetch  *fetcher
	ledger *ledger
	out    *block_store_s3.PackStore
	limits block.BatchLimits
	res    result
	start  time.Time
	// pending are the results waiting for their flush.
	pending      []*packResult
	pendingBytes int64
	pendingCount int
	// bytes0 is the byte total the ledger held at the start.
	bytes0 uint64
	// statsMtx guards the plan counts the workers add.
	statsMtx sync.Mutex
}

// run runs a rescue against client.
func run(ctx context.Context, o *options, client objectReader) (*result, error) {
	// Refuse limits that would run unbounded or past the server's caps.
	if o.Fetch.CapBytes == 0 {
		return nil, errors.New("--max-bytes is 0: set a byte cap to run")
	}
	if o.Sessions < 1 || o.Sessions > maxSessions {
		return nil, errors.Errorf("--sessions must be 1 to %d, got %d", maxSessions, o.Sessions)
	}
	if o.Log == nil {
		o.Log = io.Discard
	}

	// Open the ledger and the output pack set.
	l, err := openLedger(o.Ledger)
	if err != nil {
		return nil, err
	}
	defer l.close()
	le := logrus.NewEntry(logrus.StandardLogger())
	out := block_store_s3.NewPackStore(le, dirObjects{}, o.OutDir, "", &block_store_s3.PackStoreOptions{KeepPacks: true, SoleWriter: true, MaxPackBytes: syntheticPackBytes})
	defer out.Close()
	r := &rescuer{
		o: o, le: le, ledger: l, out: out, limits: out.GetBatchLimits(), start: time.Now(),
		fetch: newFetcher(client, o.Bucket, o.Fetch, l.bytes), bytes0: l.bytes,
	}
	for _, s := range l.refs {
		r.count(s, 1)
	}

	// Stop before fetching when the disk is already at its floor.
	if stop, err := r.belowFloor(0); err != nil || stop {
		r.res.Stop = stopDiskFloor
		return r.finish(), err
	}

	// Group the unfinished refs by pack, recording those the index lacks.
	packs, err := r.group(ctx)
	if err != nil {
		return r.finish(), err
	}
	r.res.PacksTotal = len(packs)
	err = r.fetchAll(ctx, packs)
	return r.finish(), err
}

// count adds n to the counter of a ledger state.
func (r *rescuer) count(state string, n int) {
	switch state {
	case stateDone:
		r.res.Done += n
	case stateFailed:
		r.res.Failed += n
	case stateAbsent:
		r.res.Absent += n
	case stateNotInPack:
		r.res.NotInPack += n
	}
}

// finish fills the run's totals.
func (r *rescuer) finish() *result {
	r.res.Bytes = r.fetch.fetched.Load()
	r.res.Requests = r.fetch.requests.Load()
	return &r.res
}

// belowFloor reports whether writing extra bytes would leave the output's
// file system below the disk floor.
func (r *rescuer) belowFloor(extra int64) (bool, error) {
	if r.o.DiskFloorBytes == 0 {
		return false, nil
	}
	free, err := freeBytes(r.o.OutDir)
	if err != nil {
		return false, err
	}
	return free < r.o.DiskFloorBytes+uint64(max(extra, 0)), nil //nolint:gosec // extra is not negative.
}

// skip reports whether the ledger finished the ref.
func (r *rescuer) skip(key []byte) bool {
	switch r.ledger.refs[string(key)] {
	case stateDone, stateAbsent, stateNotInPack:
		return true
	case stateFailed:
		return !r.o.RetryFailed
	}
	return false
}

// group reads the ref list and places each unfinished ref in its pack.
// Refs without an h row are recorded absent, as are refs whose pack number
// has no p row (no-pack-id), and the refs of a pack the plan cannot read
// (no-pack-entry): it has no e row, so its size is unknown, and ranges are off
// or a wanted ref has no extent. The packs are ordered by wanted refs, most
// first; those with MinWanted or fewer are left for a later run.
func (r *rescuer) group(ctx context.Context) ([]*packWork, error) {
	// Read the pack table.
	tx, err := r.o.Index.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	ids, err := block_store_s3.ReadIndexPackIDs(ctx, tx)
	tx.Discard()
	if err != nil {
		return nil, err
	}

	// Place the refs a chunk at a time.
	byID := make(map[string]*packWork)
	var chunk []want
	place := func() error {
		// Open a read transaction for the chunk.
		tx, err := r.o.Index.NewTransaction(ctx, false)
		if err != nil {
			return err
		}
		defer tx.Discard()

		// Look up each ref's h row and p row, recording those that fail.
		for _, w := range chunk {
			value, found, err := tx.Get(ctx, block_store_s3.IndexHashRowKey(w.key))
			if err != nil {
				return err
			}
			row, ok := block_store_s3.ParseIndexHashRow(value)
			if !found || !ok {
				if err := r.absent(w.key, "no-index-row"); err != nil {
					return err
				}
				continue
			}
			id, ok := ids[row.Num]
			if !ok {
				if err := r.absent(w.key, "no-pack-id"); err != nil {
					return err
				}
				continue
			}

			// Add it to its pack, reading the pack's entry the first time.
			p := byID[id]
			if p == nil {
				p = &packWork{id: id}
				if p.entry, err = block_store_s3.ReadIndexEntry(ctx, tx, id); err != nil {
					return err
				}
				byID[id] = p
			}
			if row.Extent {
				w.off, w.size, w.extent = row.Off, row.Size, true
			}
			p.wants = append(p.wants, w)
		}
		chunk = chunk[:0]
		return nil
	}
	err = readRefList(r.o.RefList, func(key []byte, ref *block.BlockRef) error {
		if r.skip(key) {
			return nil
		}
		chunk = append(chunk, want{key: key, ref: ref})
		if len(chunk) == lookupChunk {
			return place()
		}
		return nil
	})
	if err == nil && len(chunk) != 0 {
		err = place()
	}
	if err != nil {
		return nil, err
	}

	// Drop finished packs and those left empty; record the refs of packs the
	// plan cannot read; defer the packs at or below MinWanted.
	packs := make([]*packWork, 0, len(byID))
	for id, p := range byID {
		if len(p.wants) == 0 || (r.ledger.packs[id] && !r.o.RetryFailed) {
			continue
		}
		if _, _, ok := choosePlan(p, r.o.Plan); !ok {
			for _, w := range p.wants {
				if err := r.absent(w.key, "no-pack-entry"); err != nil {
					return nil, err
				}
			}
			continue
		}
		if len(p.wants) <= r.o.MinWanted {
			r.res.Deferred += len(p.wants)
			continue
		}
		_, noExt := p.counts()
		r.res.NoExtent += noExt
		packs = append(packs, p)
	}
	if err := r.ledger.sync(); err != nil {
		return nil, err
	}
	orderPacks(packs)
	return packs, nil
}

// orderPacks orders packs by wanted refs, most first, then by id.
func orderPacks(packs []*packWork) {
	slices.SortFunc(packs, func(a, b *packWork) int {
		if c := cmp.Compare(len(b.wants), len(a.wants)); c != 0 {
			return c
		}
		return cmp.Compare(a.id, b.id)
	})
}

// absent records a ref the index cannot place.
func (r *rescuer) absent(key []byte, reason string) error {
	r.count(stateAbsent, 1)
	return r.ledger.ref(key, stateAbsent, reason)
}

// fetchAll fetches the packs with Sessions workers and writes what they
// verify, until every pack is fetched or a stop.
func (r *rescuer) fetchAll(ctx context.Context, packs []*packWork) error {
	// Let the first stop cancel the rest.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Feed the packs to the workers.
	work := make(chan *packWork)
	type outcome struct {
		res *packResult
		err error
	}
	results := make(chan outcome, r.o.Sessions)
	var wg sync.WaitGroup
	go func() {
		defer close(work)
		for _, p := range packs {
			select {
			case work <- p:
			case <-ctx.Done():
				return
			}
		}
	}()
	for range r.o.Sessions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range work {
				res, err := r.fetchPack(ctx, p)
				results <- outcome{res, err}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect the results, flushing a batch at a time, until the workers
	// end. The first stop cancels the rest; their partial results still land.
	var tick <-chan time.Time
	if r.o.Progress > 0 {
		t := time.NewTicker(r.o.Progress)
		defer t.Stop()
		tick = t.C
	}
	var flushTick <-chan time.Time
	if r.o.FlushInterval > 0 {
		t := time.NewTicker(r.o.FlushInterval)
		defer t.Stop()
		flushTick = t.C
	}
	var stopErr error
	for {
		select {
		case out, ok := <-results:
			if !ok {
				err := r.flush(context.WithoutCancel(ctx))
				r.progress()
				if errors.Is(err, errStopped) {
					err = nil
				}
				if stopErr != nil {
					return stopErr
				}
				return err
			}
			if out.res != nil {
				r.add(out.res)
			}
			if r.pendingBytes >= r.limits.Bytes || r.pendingCount >= r.limits.Blocks {
				stopErr = r.flushOrStop(ctx, stopErr, cancel)
			}
			err := out.err
			var hard *hardStopError
			switch {
			case err == nil:
			case errors.As(err, &hard):
				if stopErr == nil {
					stopErr = err
				}
				cancel()
			case errors.Is(err, errCapReached):
				if r.res.Stop == "" {
					r.res.Stop = stopByteCap
				}
				cancel()
			case ctx.Err() != nil:
			default:
				r.res.PackErrors++
				r.le.WithError(err).Warn("pack fetch failed; its refs stay for the next run")
			}
		case <-tick:
			r.progress()
		case <-flushTick:
			stopErr = r.flushOrStop(ctx, stopErr, cancel)
		}
	}
}

// flushOrStop flushes, cancelling the run on a stop or an error, and
// returns the run's first error.
func (r *rescuer) flushOrStop(ctx context.Context, stopErr error, cancel func()) error {
	// Flush, keeping the run going when it succeeds.
	err := r.flush(context.WithoutCancel(ctx))
	if err == nil {
		return stopErr
	}

	// Cancel the run and return its first error.
	cancel()
	if errors.Is(err, errStopped) || stopErr != nil {
		return stopErr
	}
	return err
}

// add queues a result for the next flush.
func (r *rescuer) add(res *packResult) {
	r.pending = append(r.pending, res)
	r.pendingBytes += res.bytes
	r.pendingCount += len(res.blocks)
}

// flush writes the pending blocks to the output as packs, then records them
// and the byte total in the ledger and syncs it. Below the disk floor it
// records the byte total alone and stops the run.
func (r *rescuer) flush(ctx context.Context) error {
	// Stop at the disk floor, keeping the byte count.
	if stop, err := r.belowFloor(r.pendingBytes); err != nil || stop {
		if err == nil {
			r.res.Stop = stopDiskFloor
			err = errStopped
		}
		if terr := r.ledger.total(r.fetch.fetched.Load()); terr == nil {
			_ = r.ledger.sync()
		}
		r.pending, r.pendingBytes, r.pendingCount = nil, 0, 0
		return err
	}

	// Write the blocks in batches within the output's limits.
	var batch []*block.PutBatchEntry
	var batchBytes int64
	put := func() error {
		if len(batch) == 0 {
			return nil
		}
		_, err := r.out.PutBlockBatch(ctx, batch)
		batch, batchBytes = nil, 0
		return err
	}
	for _, res := range r.pending {
		for _, b := range res.blocks {
			if len(batch) != 0 && (batchBytes+int64(len(b.Data)) > r.limits.Bytes || len(batch) >= r.limits.Blocks) {
				if err := put(); err != nil {
					return err
				}
			}
			batch = append(batch, b)
			batchBytes += int64(len(b.Data))
		}
	}
	if err := put(); err != nil {
		return err
	}

	// Record the blocks, the failures and the finished packs, then sync.
	for _, res := range r.pending {
		for _, k := range res.keys {
			if err := r.ledger.ref(k, stateDone, ""); err != nil {
				return err
			}
		}
		for _, f := range res.finished {
			if err := r.ledger.ref(f.key, f.state, f.reason); err != nil {
				return err
			}
			r.count(f.state, 1)
		}
		if res.complete {
			if err := r.ledger.pack(res.id, res.fetched, len(res.keys), res.notInPack()); err != nil {
				return err
			}
			r.res.PacksDone++
		}
		r.count(stateDone, len(res.keys))
		r.res.NoExtentDone += res.noExt
	}
	if err := r.ledger.total(r.fetch.fetched.Load()); err != nil {
		return err
	}
	r.pending, r.pendingBytes, r.pendingCount = nil, 0, 0
	return r.ledger.sync()
}

// errStopped ends a run at a clean stop; result.Stop says which.
var errStopped = errors.New("stopped")

// progress prints a progress line.
func (r *rescuer) progress() {
	secs := max(time.Since(r.start).Seconds(), 1e-3)
	fetched := r.fetch.fetched.Load()
	_, _ = fmt.Fprintf(r.o.Log,
		"packs %d/%d (whole %d, ranged %d, errors %d) refs done %d failed %d not-in-pack %d absent %d deferred %d no-extent %d/%d bytes %d (%.0f B/s) requests %d (%.1f/s)\n",
		r.res.PacksDone, r.res.PacksTotal, r.res.Whole, r.res.Ranged, r.res.PackErrors,
		r.res.Done, r.res.Failed, r.res.NotInPack, r.res.Absent, r.res.Deferred, r.res.NoExtentDone, r.res.NoExtent,
		fetched, float64(fetched-r.bytes0)/secs, r.fetch.requests.Load(), float64(r.fetch.requests.Load())/secs)
}

// packKey returns the object key of a pack.
func (r *rescuer) packKey(id string) string {
	return r.o.Prefix + "packs/" + id
}

// fetchPack fetches the wanted blocks of a pack and verifies them, keeping
// only them: the rest of a whole pack is dropped with its buffer. On an error
// it returns what it verified before it, with complete unset.
func (r *rescuer) fetchPack(ctx context.Context, p *packWork) (*packResult, error) {
	// Choose the plan and count it.
	res := &packResult{id: p.id}
	mode, _, _ := choosePlan(p, r.o.Plan)
	key := r.packKey(p.id)
	r.countPlan(mode)

	// Read each wanted block's extent: the plan chose ranges only when every
	// wanted block has one.
	if mode == planRanges {
		for _, w := range p.wants {
			data, err := r.fetch.rangeRead(ctx, key, int64(w.off), int(w.size)) //nolint:gosec // extents fit int.
			res.fetched += uint64(len(data))
			if err != nil {
				return r.missing(res, p, err)
			}
			res.take(w, data)
		}
		res.complete = true
		return res, nil
	}

	// Read the whole pack.
	data, err := r.fetch.whole(ctx, key, p.entry.GetSizeBytes())
	res.fetched = uint64(len(data))
	if err != nil {
		return r.missing(res, p, err)
	}

	// Map the pack's blocks through its own index.
	index, err := packfile.ReadIndex(bytes.NewReader(data), uint64(len(data)))
	if err != nil {
		return r.failAll(res, p, "bad-pack-index"), nil
	}
	type span struct{ off, size uint64 }
	at := make(map[string]span, len(index))
	for _, ie := range index {
		at[string(ie.GetKey())] = span{off: ie.GetOffset(), size: ie.GetSize()}
	}

	// Take the wanted blocks, recording those the pack lacks.
	for _, w := range p.wants {
		e, ok := at[string(w.key)]
		if !ok || e.off+e.size > uint64(len(data)) {
			res.finished = append(res.finished, refReason{key: w.key, state: stateNotInPack})
			continue
		}
		res.take(w, data[e.off:e.off+e.size])
	}
	res.complete = true
	return res, nil
}

// countPlan counts a pack's plan.
func (r *rescuer) countPlan(mode planMode) {
	r.statsMtx.Lock()
	defer r.statsMtx.Unlock()
	if mode == planWhole {
		r.res.Whole++
	} else {
		r.res.Ranged++
	}
}

// missing ends a pack's fetch on err: a missing pack fails its refs not yet
// taken; any other error returns the partial result with it.
func (r *rescuer) missing(res *packResult, p *packWork, err error) (*packResult, error) {
	if errors.Is(err, block_store_s3.ErrNotFound) {
		taken := make(map[string]bool, len(res.keys)+len(res.finished))
		for _, k := range res.keys {
			taken[string(k)] = true
		}
		for _, f := range res.finished {
			taken[string(f.key)] = true
		}
		for _, w := range p.wants {
			if !taken[string(w.key)] {
				res.finished = append(res.finished, refReason{key: w.key, state: stateFailed, reason: "pack-missing"})
			}
		}
		res.complete = true
		return res, nil
	}
	return res, err
}

// failAll fails every wanted ref of the pack.
func (r *rescuer) failAll(res *packResult, p *packWork, reason string) *packResult {
	for _, w := range p.wants {
		res.finished = append(res.finished, refReason{key: w.key, state: stateFailed, reason: reason})
	}
	res.complete = true
	return res
}

// take verifies a block's pack value against its ref and keeps it, or fails
// the ref. A block whose bytes do not hash to its ref is never stored.
func (res *packResult) take(w want, value []byte) {
	// Decode the pack value into the block.
	stored, err := block.DecodeBlockObject(value)
	if err != nil {
		res.finished = append(res.finished, refReason{key: w.key, state: stateFailed, reason: "decode"})
		return
	}

	// Verify its bytes against the ref.
	if err := w.ref.VerifyData(stored.Data, false); err != nil {
		res.finished = append(res.finished, refReason{key: w.key, state: stateFailed, reason: "hash-mismatch"})
		return
	}

	// Keep it.
	res.blocks = append(res.blocks, &block.PutBatchEntry{Ref: w.ref, Data: stored.Data, Refs: stored.Refs, RefsKnown: stored.RefsKnown})
	res.keys = append(res.keys, w.key)
	res.bytes += int64(len(stored.Data))
	if !w.extent {
		res.noExt++
	}
}
