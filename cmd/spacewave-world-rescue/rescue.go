//go:build !js && !wasip1 && !windows

package main

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
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
	// Layout is the output's layout, layoutPackStore when empty or
	// layoutAdopt.
	Layout string
	// Sessions is the packs fetched at once.
	Sessions int
	// Plan chooses between whole packs and ranges.
	Plan planOptions
	// MaxPackBytes is the largest pack read whole; a larger one is skipped
	// and its refs recorded too-large. Zero is no limit.
	MaxPackBytes uint64
	// MinWanted takes only the packs holding more than MinWanted wanted
	// refs this run; the rest stay unrecorded for a later run.
	MinWanted int
	// Order is the pack order, orderWanted or orderTime; OrderOverhead the
	// per-pack bytes orderTime adds to each pack's size.
	Order         string
	OrderOverhead uint64
	// MaxPacks takes only the first MaxPacks packs in order, leaving the
	// rest's refs for a later run; 0 takes all.
	MaxPacks int
	// Packs, when set, plans only the packs it names, leaving the other
	// packs' refs for a later run, so a restart takes the same packs.
	Packs map[string]bool
	// Keep keeps the keep set's blocks of each whole pack beside the wanted.
	Keep keepOptions
	// WholePacks are pack ids read whole even when no wanted ref places a
	// block in them, for their keep set blocks: packs the index cannot
	// place, such as the unindexed and those of a shared number. They go
	// after every pack the refs place, in list order. They need Keep.Set
	// and refuse Plan.Ranges.
	WholePacks []string
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
	// PlanOut, when set, plans only: the run writes the plan there, with the
	// ledger lines planning writes, and returns with no request. The client
	// is ignored, so no request, dial or redial hook can happen.
	PlanOut string
	// PlanIn, when set, takes the plan from that file instead of planning:
	// neither the ref list nor Index is read to plan, and Index may be nil.
	PlanIn string
	// IndexPath is the index file, whose identity the plan's inputs hash
	// holds; empty names no index.
	IndexPath string
	// IndexID is the --index-id sidecar, sha256sum's output for the index
	// and its -wal; see readIndexID. When set, the plan's inputs hold its
	// hashes in place of IndexPath's path, size and modification time.
	// PlanOut and PlanIn need it; PlanOut hashes IndexPath and its -wal and
	// refuses a sidecar that differs, and PlanIn trusts it.
	IndexID string
	// IndexTable and StoreID name the index rows planning reads; the plan's
	// inputs hold them.
	IndexTable, StoreID string
	// PlanSize is whether the fetch's client will have SIZE, which planning
	// with no client, under PlanOut, assumes.
	PlanSize bool
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
	// TooLarge are refs of packs past MaxPackBytes, not fetched.
	// Ambiguous are refs whose shared pack number no candidate's bloom filter
	// may hold.
	Done, Failed, Absent, NotInPack, TooLarge, Ambiguous int
	// SharedNum counts this run's refs placed in several candidate packs of a
	// shared number.
	SharedNum int
	// Sized counts this run's packs whose size the server gave with SIZE.
	Sized int
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
	// reqs times and counts the pack's requests.
	reqs reqStats
	// skip says why the pack was not fetched, empty when it was.
	skip string
	// extras are the keep set's blocks of the pack.
	extras []extra
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
	o *options
	// keep takes the keep set's blocks, nil for none.
	keep   *keeper
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
	// multi holds, for each ref placed in several candidate packs, the
	// candidates whose results have not landed; multiDone the ones of them
	// stored. Only the flush reads and writes them after group.
	multi     map[string]int
	multiDone map[string]bool
	// written holds the tails of the keep set's keys stored this run,
	// wanted or kept, so the flush stores each at most once. Only the flush
	// reads and writes it.
	written map[keyTail]struct{}
	// dropped are the --packs ids the plan does not fetch, by id.
	dropped []droppedPack
	// index is the run's index identity, read once by runIndexIdentity.
	index string
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
	if o.PlanOut != "" && o.PlanIn != "" {
		return nil, errors.New("--plan-out and --plan-in exclude each other")
	}
	if (o.PlanOut != "" || o.PlanIn != "") && o.IndexID == "" {
		return nil, errors.New("--plan-out and --plan-in need --index-id")
	}
	if len(o.WholePacks) != 0 {
		if o.Plan.Ranges {
			return nil, errors.New("--whole-packs reads whole packs: drop --ranges")
		}
		if o.Keep.Set == nil {
			return nil, errors.New("--whole-packs needs --keep-set: a listed pack no ref places gives only keep set blocks")
		}
		if err := wholeInPacks(o.WholePacks, o.Packs); err != nil {
			return nil, err
		}
	}

	// Read the index identity once, checking a plan out's sidecar against
	// the index file and its -wal.
	index, err := runIndexIdentity(o)
	if err != nil {
		return nil, err
	}

	// Drop the client when planning only, so nothing past here can reach
	// the server: the fetcher gets no client and installs no hook.
	if o.PlanOut != "" {
		client = nil
	}

	// Choose the output layout, finishing the pairs a crash left staged in
	// the adopt layout.
	var objects block_store_s3.ObjectStore
	var recovered []string
	switch o.Layout {
	case "", layoutPackStore:
		objects = dirObjects{}
	case layoutAdopt:
		var orphans []string
		var err error
		recovered, orphans, err = recoverAdopt(o.OutDir)
		if err != nil {
			return nil, err
		}
		for _, name := range orphans {
			logrus.WithField("file", name).Warn("staged pack with no entry left in place: its blocks are fetched again, and the file may be removed by hand")
		}
		objects = adoptObjects{}
	default:
		return nil, errors.Errorf("--layout must be %s or %s, got %q", layoutPackStore, layoutAdopt, o.Layout)
	}

	// Open the ledger and the output pack set.
	l, err := openLedger(o.Ledger)
	if err != nil {
		return nil, err
	}
	defer l.close()
	if err := ledgerAdopted(l, o.OutDir, recovered); err != nil {
		return nil, err
	}

	// Build the rescuer, its fetcher reserving bytes in the ledger.
	le := logrus.NewEntry(logrus.StandardLogger())
	out := block_store_s3.NewPackStore(le, objects, o.OutDir, "", &block_store_s3.PackStoreOptions{KeepPacks: true, SoleWriter: true, MaxPackBytes: syntheticPackBytes})
	defer out.Close()
	fo := o.Fetch
	fo.Reserved = l.reserve
	fo.Log = cmp.Or[io.Writer](fo.Log, o.Log)
	r := &rescuer{
		o: o, le: le, ledger: l, out: out, multi: make(map[string]int), multiDone: make(map[string]bool), limits: out.GetBatchLimits(), start: time.Now(),
		fetch: newFetcher(client, o.Bucket, fo, l.bytes), bytes0: l.bytes,
		keep: newKeeper(o.Keep), written: make(map[keyTail]struct{}), index: index,
	}
	for _, s := range l.refs {
		r.count(s, 1)
	}

	// Stop before fetching when the disk is already at its floor; a plan
	// only writes no blocks.
	if o.PlanOut == "" {
		if stop, err := r.belowFloor(0); err != nil || stop {
			r.res.Stop = stopDiskFloor
			return r.finish(), err
		}
	}

	// Load the plan, or group the unfinished refs by pack, recording those
	// the index lacks; a plan only ends here.
	var packs []*packWork
	if o.PlanIn != "" {
		packs, err = r.loadPlan()
	} else {
		packs, err = r.plan(ctx)
	}
	if err != nil || o.PlanOut != "" {
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
	case stateTooLarge:
		r.res.TooLarge += n
	case stateAmbiguous:
		r.res.Ambiguous += n
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
	case stateDone, stateAbsent, stateNotInPack, stateAmbiguous:
		return true
	case stateFailed, stateTooLarge:
		return !r.o.RetryFailed
	}
	return false
}

// group reads the ref list and places each unfinished ref in its pack.
// Refs without an h row are recorded absent, as are refs whose pack number
// has no p row (no-pack-id), and the refs of a pack the plan cannot read
// (no-pack-entry): it has no e row, so its size is unknown, the client has no
// SIZE, and ranges are off or a wanted ref has no extent. A ref of a number
// several packs share goes to each whose bloom filter may hold it, and is
// recorded ambiguous-num when none may. The packs are ordered by wanted refs,
// most first; those with MinWanted or fewer are left for a later run.
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
			cands := ids[row.Num]
			if len(cands) == 0 {
				if err := r.absent(w.key, "no-pack-id"); err != nil {
					return err
				}
				continue
			}

			// Load the packs of the number, reading each entry the first
			// time.
			packs := make([]*packWork, 0, len(cands))
			for _, id := range cands {
				p := byID[id]
				if p == nil {
					p = &packWork{id: id}
					if p.entry, err = block_store_s3.ReadIndexEntry(ctx, tx, id); err != nil {
						return err
					}
					byID[id] = p
				}
				packs = append(packs, p)
			}

			// Keep, of a number several packs share, the packs whose bloom
			// filters may hold the ref, recording it ambiguous-num when none
			// may.
			if len(packs) > 1 {
				packs = slices.DeleteFunc(packs, func(p *packWork) bool { return !p.mayHold(w.key) })
				if len(packs) == 0 {
					r.count(stateAmbiguous, 1)
					if err := r.ledger.ref(w.key, stateAmbiguous, ""); err != nil {
						return err
					}
					continue
				}
			}

			// Add it to each candidate pack. The row's extent is the block's
			// only in the sole candidate, which the filters cannot rule out.
			if row.Extent && len(packs) == 1 {
				w.off, w.size, w.extent = row.Off, row.Size, true
			}
			if len(packs) > 1 {
				r.multi[string(w.key)] = len(packs)
				r.res.SharedNum++
			}
			for _, p := range packs {
				p.wants = append(p.wants, w)
			}
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

	// Add the listed whole packs no ref placed, each with its entry, nil
	// when it has no e row: the server's SIZE sizes it then.
	listed := make(map[string]bool, len(r.o.WholePacks))
	if len(r.o.WholePacks) != 0 {
		tx, err := r.o.Index.NewTransaction(ctx, false)
		if err != nil {
			return nil, err
		}
		for _, id := range r.o.WholePacks {
			listed[id] = true
			if byID[id] != nil {
				continue
			}
			p := &packWork{id: id}
			if p.entry, err = block_store_s3.ReadIndexEntry(ctx, tx, id); err != nil {
				tx.Discard()
				return nil, err
			}
			byID[id] = p
		}
		tx.Discard()
	}

	// Drop the packs left empty; defer, unrecorded, the packs outside Packs,
	// which run made hold every listed whole pack; record the refs of packs
	// the plan cannot read; defer the packs at or below MinWanted. A pack
	// the ledger
	// finished is kept when it has wants: they are refs no earlier run
	// recorded, added to the list since, and every candidate pack of a ref
	// stays counted in r.multi. A listed whole pack is neither dropped
	// empty nor deferred, unless the ledger finished it and it has no
	// wants; one the refs alone would not take goes to the tail.
	packs := make([]*packWork, 0, len(byID))
	tail := make(map[string]*packWork)
	for _, p := range byID {
		whole := listed[p.id]
		if len(p.wants) == 0 && (!whole || r.ledger.packs[p.id]) {
			r.drop(p.id, dropNoWants, 0)
			continue
		}
		if r.o.Packs != nil && !r.o.Packs[p.id] {
			r.res.Deferred += len(p.wants)
			continue
		}
		if _, _, ok := choosePlan(p, r.o.Plan); !ok && !r.canSize() {
			if len(p.wants) == 0 {
				r.le.Warn("a listed whole pack has no entry and the client has no SIZE; not fetched")
			}
			for _, w := range p.wants {
				if !r.settle(w.key, false) {
					continue
				}
				if err := r.absent(w.key, "no-pack-entry"); err != nil {
					return nil, err
				}
			}
			r.drop(p.id, dropNoEntry, len(p.wants))
			continue
		}
		_, noExt := p.counts()
		if len(p.wants) <= r.o.MinWanted {
			if !whole {
				r.res.Deferred += len(p.wants)
				r.drop(p.id, dropMinWanted, len(p.wants))
				continue
			}
			r.res.NoExtent += noExt
			tail[p.id] = p
			continue
		}
		r.res.NoExtent += noExt
		packs = append(packs, p)
	}
	if err := r.ledger.sync(); err != nil {
		return nil, err
	}

	// Order the packs the refs place, then add the listed tail in list
	// order: it never moves a placed pack, and --max-packs counts it last.
	orderPacksBy(packs, r.o.Order, r.o.OrderOverhead)
	for _, id := range r.o.WholePacks {
		if p := tail[id]; p != nil {
			packs = append(packs, p)
			delete(tail, id)
		}
	}
	if r.o.MaxPacks > 0 && len(packs) > r.o.MaxPacks {
		for _, p := range packs[r.o.MaxPacks:] {
			r.res.Deferred += len(p.wants)
			r.drop(p.id, dropMaxPacks, len(p.wants))
		}
		packs = packs[:r.o.MaxPacks]
	}

	// Record the --packs ids no ref placed as having no wants, then sort the
	// drops by id.
	for id := range r.o.Packs {
		if byID[id] == nil {
			r.drop(id, dropNoWants, 0)
		}
	}
	slices.SortFunc(r.dropped, func(a, b droppedPack) int { return cmp.Compare(a.id, b.id) })
	return packs, nil
}

// drop records a --packs id the plan does not fetch; other packs are not
// recorded.
func (r *rescuer) drop(id, reason string, wants int) {
	if r.o.Packs[id] {
		r.dropped = append(r.dropped, droppedPack{id: id, reason: reason, wants: wants})
	}
}

// plan groups the refs, prints the plan line and, under PlanOut, writes the
// plan; else it writes the dropped --packs ids to OutDir, as the plan line
// names none.
func (r *rescuer) plan(ctx context.Context) ([]*packWork, error) {
	// Group, noting the counters planning adds.
	base := r.res
	packs, err := r.group(ctx)
	if err != nil {
		return nil, err
	}
	inputs, parts, err := planInputs(r.o, r.canSize(), r.index)
	if err != nil {
		return nil, err
	}

	// Write the plan, or the dropped ids of an in-process plan.
	if r.o.PlanOut != "" {
		pf := &planFile{
			Inputs: inputs, Parts: parts, Packs: packs, Multi: r.multi, Dropped: r.dropped,
			Counts: planCounts{
				Absent: r.res.Absent - base.Absent, Deferred: r.res.Deferred - base.Deferred,
				SharedNum: r.res.SharedNum - base.SharedNum, NoExtent: r.res.NoExtent - base.NoExtent,
				Ambiguous: r.res.Ambiguous - base.Ambiguous,
			},
		}
		pf.Wanted, pf.Bytes = planTotals(packs, r.o.Plan, r.multi)
		if err := writePlanFile(r.o.PlanOut, pf); err != nil {
			return nil, err
		}
	} else if len(r.dropped) != 0 {
		if err := writeDropped(filepath.Join(r.o.OutDir, droppedListName), r.dropped); err != nil {
			return nil, err
		}
	}

	// Print the plan line.
	_, _ = io.WriteString(r.o.Log, planLine(packs, r.o.Plan, r.multi, r.dropped, inputs))
	return packs, nil
}

// loadPlan reads the PlanIn plan, checks it was made from this run's inputs,
// and leaves out what the ledger finished since, as group would: each
// finished want, and each pack left with none unless it is a listed whole
// pack the ledger has not finished. It prints the plan line.
func (r *rescuer) loadPlan() ([]*packWork, error) {
	// Name the ledger the plan resumes against, as the inputs do not hold
	// it: its path, the packs it has p lines for and the refs it has r
	// lines for.
	r.ledger.mtx.Lock()
	donePacks, doneRefs := len(r.ledger.packs), len(r.ledger.refs)
	r.ledger.mtx.Unlock()
	_, _ = fmt.Fprintf(r.o.Log, "plan ledger %s p %d r %d\n", r.o.Ledger, donePacks, doneRefs)

	// Read the plan and check its inputs and its --packs set.
	pf, err := checkPlan(r.o, r.canSize(), r.index)
	if err != nil {
		return nil, err
	}

	// Leave out the finished wants and the packs left empty.
	listed := make(map[string]bool, len(r.o.WholePacks))
	for _, id := range r.o.WholePacks {
		listed[id] = true
	}
	gone := make(map[string]bool)
	var goneNoExt int
	packs := make([]*packWork, 0, len(pf.Packs))
	for _, p := range pf.Packs {
		kept := p.wants[:0]
		for _, w := range p.wants {
			if !r.skip(w.key) {
				kept = append(kept, w)
				continue
			}
			if !w.extent {
				goneNoExt++
			}
			if _, ok := pf.Multi[string(w.key)]; ok {
				gone[string(w.key)] = true
			}
		}
		p.wants = kept
		if len(p.wants) == 0 && (!listed[p.id] || r.ledger.packs[p.id]) {
			continue
		}
		packs = append(packs, p)
	}
	for k := range gone {
		delete(pf.Multi, k)
	}

	// Take the plan's shared numbers and the counters the ledger lacks.
	r.multi = pf.Multi
	r.dropped = pf.Dropped
	r.res.Deferred += pf.Counts.Deferred
	r.res.SharedNum += max(pf.Counts.SharedNum-len(gone), 0)
	r.res.NoExtent += max(pf.Counts.NoExtent-goneNoExt, 0)

	// Print the plan line.
	_, _ = io.WriteString(r.o.Log, planLine(packs, r.o.Plan, r.multi, r.dropped, pf.Inputs))
	return packs, nil
}

// writeDropped writes the dropped ids, one a line as id, reason and wants,
// tab separated, replacing the file atomically.
func writeDropped(path string, dropped []droppedPack) error {
	var b strings.Builder
	for _, d := range dropped {
		_, _ = fmt.Fprintf(&b, "%s\t%s\t%d\n", d.id, d.reason, d.wants)
	}
	return dirObjects{}.PutObject(context.Background(), filepath.Dir(path), filepath.Base(path), []byte(b.String()), "")
}

// canSize reports whether the client can ask the server for a pack's size,
// for a pack the index gives none.
func (r *rescuer) canSize() bool {
	if r.fetch.client == nil {
		return r.o.PlanSize
	}
	_, ok := r.fetch.client.(objectSizer)
	return ok
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
				if stopErr == nil {
					stopErr = r.fetch.stopError()
				}
				if stopErr != nil {
					return stopErr
				}
				return err
			}
			// Queue the result and make the byte total durable at once, so a
			// crash loses no finished pack's bytes.
			if out.res != nil {
				r.add(out.res)
				err := r.ledger.total(r.fetch.fetched.Load())
				if err == nil {
					err = r.ledger.sync()
				}
				if err != nil && stopErr == nil {
					stopErr = err
					cancel()
				}
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
			case errors.Is(err, errHardStopped):
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
	stored := make(map[string]bool)
	for _, res := range r.pending {
		for i, b := range res.blocks {
			// Store a ref of several candidate packs once.
			k := string(res.keys[i])
			if _, ok := r.multi[k]; ok {
				if r.multiDone[k] || stored[k] {
					continue
				}
				stored[k] = true
			}
			// Store a keep set key once in the run, wanted or kept.
			if r.keep != nil && r.keep.o.Set.has(res.keys[i]) && !r.once(res.keys[i]) {
				continue
			}
			if len(batch) != 0 && (batchBytes+int64(len(b.Data)) > r.limits.Bytes || len(batch) >= r.limits.Blocks) {
				if err := put(); err != nil {
					return err
				}
			}
			batch = append(batch, b)
			batchBytes += int64(len(b.Data))
		}
		for j := range res.extras {
			x := &res.extras[j]
			if x.blk == nil {
				continue
			}
			if !r.once(x.key) {
				x.dup = true
				continue
			}
			if len(batch) != 0 && (batchBytes+int64(len(x.blk.Data)) > r.limits.Bytes || len(batch) >= r.limits.Blocks) {
				if err := put(); err != nil {
					return err
				}
			}
			batch = append(batch, x.blk)
			batchBytes += int64(len(x.blk.Data))
		}
	}
	if err := put(); err != nil {
		return err
	}

	// Record the blocks, the failures and the finished packs, then sync.
	for _, res := range r.pending {
		var done int
		for _, k := range res.keys {
			if !r.settle(k, true) {
				continue
			}
			if err := r.ledger.ref(k, stateDone, ""); err != nil {
				return err
			}
			done++
		}
		for _, f := range res.finished {
			if !r.settle(f.key, false) {
				continue
			}
			if err := r.ledger.ref(f.key, f.state, f.reason); err != nil {
				return err
			}
			r.count(f.state, 1)
		}
		if r.keep != nil && len(res.extras) != 0 {
			if err := r.keep.record(res.id, res.extras); err != nil {
				return err
			}
		}
		if res.complete {
			if err := r.ledger.pack(res.id, res.fetched, len(res.keys), res.notInPack(), res.skip); err != nil {
				return err
			}
			if elapsed, n := res.reqs.result(); n != 0 {
				if err := r.ledger.timing(res.id, elapsed, n); err != nil {
					return err
				}
			}
			r.res.PacksDone++
		}
		r.count(stateDone, done)
		r.res.NoExtentDone += res.noExt
	}
	if err := r.ledger.total(r.fetch.fetched.Load()); err != nil {
		return err
	}
	r.pending, r.pendingBytes, r.pendingCount = nil, 0, 0
	return r.ledger.sync()
}

// once reports whether key is a keep set key not yet stored this run, and
// marks it stored.
func (r *rescuer) once(key []byte) bool {
	t := tailOf(key)
	if _, ok := r.written[t]; ok {
		return false
	}
	r.written[t] = struct{}{}
	return true
}

// settle reports whether a pack's result for key is the ref's to record. A
// ref of one pack always is; a ref of several candidate packs is recorded
// done by the first that stored it, or by the last result when none did.
func (r *rescuer) settle(key []byte, found bool) bool {
	// Record a ref of one pack, and nothing more of one already stored.
	k := string(key)
	left, ok := r.multi[k]
	if !ok {
		return true
	}
	if r.multiDone[k] {
		return false
	}

	// Record the first store, or else the last candidate's miss.
	if found {
		r.multiDone[k] = true
		return true
	}
	left--
	r.multi[k] = left
	return left == 0
}

// errStopped ends a run at a clean stop; result.Stop says which.
var errStopped = errors.New("stopped")

// progress prints a progress line.
func (r *rescuer) progress() {
	secs := max(time.Since(r.start).Seconds(), 1e-3)
	fetched := r.fetch.fetched.Load()
	_, _ = fmt.Fprintf(r.o.Log,
		"packs %d/%d (whole %d, ranged %d, errors %d) refs done %d failed %d not-in-pack %d too-large %d ambiguous-num %d shared-num %d absent %d deferred %d no-extent %d/%d bytes %d (%.0f B/s) requests %d (%.1f/s)\n",
		r.res.PacksDone, r.res.PacksTotal, r.res.Whole, r.res.Ranged, r.res.PackErrors,
		r.res.Done, r.res.Failed, r.res.NotInPack, r.res.TooLarge, r.res.Ambiguous, r.res.SharedNum, r.res.Absent, r.res.Deferred, r.res.NoExtentDone, r.res.NoExtent,
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
	// Skip a pack the index says is past the memory guard.
	res := &packResult{id: p.id}
	ctx = withReqStats(ctx, &res.reqs)
	key := r.packKey(p.id)
	if p.entry != nil && r.tooLarge(p.entry.GetSizeBytes()) {
		return r.skipAll(res, p, stateTooLarge, "too-large"), nil
	}

	// Choose the plan and count it.
	mode, _, ok := choosePlan(p, r.o.Plan)
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

	// Find the pack's size: from its entry, or else from the server's SIZE
	// on a pooled session, skipping a pack it cannot size or past the guard.
	size := p.entry.GetSizeBytes()
	if !ok {
		var err error
		size, err = r.fetch.size(ctx, key)
		var hard *hardStopError
		if errors.As(err, &hard) || errors.Is(err, errHardStopped) || ctx.Err() != nil {
			return res, err
		}
		if err != nil {
			r.le.WithError(err).Warn("pack size unknown; not fetched")
			return r.skipAll(res, p, stateFailed, "no-size"), nil
		}
		r.statsMtx.Lock()
		r.res.Sized++
		r.statsMtx.Unlock()
		if r.tooLarge(size) {
			return r.skipAll(res, p, stateTooLarge, "too-large"), nil
		}
	}

	// Read the whole pack, charging its size against the byte cap first.
	data, err := r.fetch.whole(ctx, key, size)
	res.fetched = uint64(len(data))
	if errors.Is(err, errOverSize) {
		r.le.WithError(err).Warn("pack larger than its size; not read")
		return r.failAll(res, p, "over-size"), nil
	}
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

	// Take the keep set's other blocks of the pack.
	if r.keep != nil {
		wanted := make(map[string]bool, len(p.wants))
		for _, w := range p.wants {
			wanted[string(w.key)] = true
		}
		res.extras = r.keep.take(index, data, wanted)
		for _, x := range res.extras {
			if x.blk != nil {
				res.bytes += int64(len(x.blk.Data))
			}
		}
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

// tooLarge reports whether a pack of size bytes is past the memory guard.
func (r *rescuer) tooLarge(size uint64) bool {
	return r.o.MaxPackBytes != 0 && size > r.o.MaxPackBytes
}

// skipAll records every wanted ref of a pack not fetched in state, and the
// pack with why.
func (r *rescuer) skipAll(res *packResult, p *packWork, state, why string) *packResult {
	for _, w := range p.wants {
		res.finished = append(res.finished, refReason{key: w.key, state: state, reason: why})
	}
	res.complete, res.skip = true, why
	return res
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
