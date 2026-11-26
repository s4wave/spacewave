package kvtx

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/volume"
)

// openStage is the in-process state of one stage.
type openStage struct {
	// released records that the stage's release was called; the next changing
	// direct transaction releases its roots.
	released bool
	// mu guards roots, apart from rootPinMu, so a staged batch's lookups never
	// hold the lock every changing transaction takes. It is taken after
	// rootPinMu, never before.
	mu sync.Mutex
	// roots are the roots the stage's committed transactions hold, by name.
	// Renewal holds a root again from here when a collector released it for
	// a lapsed lease while the stage was open, and a put releases a root it
	// names with this record's block as the root's Previous.
	roots map[string]*block.BlockRef
	// lost records that a root the stage held is gone with its block: its
	// process stalled past the lease and the grace period, so the stage can
	// no longer build on what it wrote.
	lost bool
}

// errStageLost rejects a write through a stage that lost a root it held.
var errStageLost = errors.Wrap(block.ErrStageReleased, "stage lost a root it held while its process stalled past the lease and grace period")

// stageNames are the root names a staged batch touches, each encoded once
// and outside any lock.
type stageNames struct {
	// put names the root that holds each entry.
	put map[string]*block.BlockRef
	// named names the roots on the blocks the entries name.
	named map[string]struct{}
}

// newStageNames names the roots of entries and of the blocks they name.
func newStageNames(stage string, entries []*block.PutBatchEntry) stageNames {
	n := stageNames{put: make(map[string]*block.BlockRef), named: make(map[string]struct{})}
	for _, entry := range entries {
		if entry == nil || entry.Tombstone || len(entry.Data) == 0 {
			continue
		}
		n.put[stageRootName(stage, entry.Ref)] = entry.Ref
		for _, child := range entry.Refs {
			n.named[stageRootName(stage, child)] = struct{}{}
		}
	}
	return n
}

// openStageNamed returns the open stage named stage, or nil.
func (v *Volume) openStageNamed(stage string) *openStage {
	unlock, _ := v.rootPinMu.Lock(context.Background())
	defer unlock()
	return v.stages[stage]
}

// stageHeld returns the roots the stage's record holds among names, reading
// only those, not every root the stage holds, under the stage's own lock. It
// refuses as useStage does unless the stage is open, so a release or a loss
// that comes between a batch's slices refuses the next one.
func (v *Volume) stageHeld(ctx context.Context, stage string, names stageNames) (map[string]*block.BlockRef, error) {
	// Require the open stage.
	st, err := v.useStage(ctx, stage)
	if err != nil {
		return nil, err
	}

	// Look up each name under the stage's lock.
	held := make(map[string]*block.BlockRef)
	st.mu.Lock()
	defer st.mu.Unlock()
	for name := range names.put {
		if root, ok := st.roots[name]; ok {
			held[name] = root
		}
	}
	for name := range names.named {
		if root, ok := st.roots[name]; ok {
			held[name] = root
		}
	}
	return held, nil
}

// loseStage marks a stage lost, so every later write through it fails.
func (v *Volume) loseStage(stage string) {
	unlock, _ := v.rootPinMu.Lock(context.Background())
	defer unlock()
	if st := v.stages[stage]; st != nil {
		st.lost = true
	}
}

// noteStageRoots records the root moves a committed transaction of stage
// made, under the stage's lock.
func (v *Volume) noteStageRoots(stage string, moves []block.NamedRoot) {
	// Find the open stage.
	st := v.openStageNamed(stage)
	if st == nil {
		return
	}

	// Apply each move to its record.
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, move := range moves {
		if move.Ref.GetEmpty() {
			delete(st.roots, move.Name)
			continue
		}
		if st.roots == nil {
			st.roots = make(map[string]*block.BlockRef)
		}
		st.roots[move.Name] = move.Ref
	}
}

// OpenStage opens a stage of this process and returns its name. A stage holds
// each block written through it under a leased root of its own, from the put
// that stores the block until a block written through it names the block, or
// until the stage releases it. The first stage of a process starts the
// process lease, one transaction, so the stage's first batch lands in one;
// a later stage that writes nothing costs no transaction. The release leaves
// the stage's roots for the next changing direct transaction to release, like
// a reader pin; closing the Volume releases its open stages, and the
// collector releases the roots of a process that died once their lease
// lapses.
func (v *Volume) OpenStage(ctx context.Context) (string, func(), error) {
	// Refuse new stages after the Volume closes.
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		return "", nil, err
	}
	if v.rootPinsClosed {
		unlock()
		return "", nil, volume.ErrVolumeClosed
	}

	// Register the stage.
	stage := stageRootPrefix + v.procID + "/" + ulid.NewULID()
	v.stages[stage] = &openStage{}
	unlock()

	// Start the process lease if this process has none yet.
	if v.processLeaseID() == "" {
		if _, err := v.renewRoots(ctx, true); err != nil {
			v.releaseStage(stage)
			return "", nil, err
		}
	}
	return stage, func() { v.releaseStage(stage) }, nil
}

// releaseStage marks a stage released, for the next changing direct
// transaction to release its roots.
func (v *Volume) releaseStage(stage string) {
	unlock, _ := v.rootPinMu.Lock(context.Background())
	defer unlock()
	if st := v.stages[stage]; st != nil && !st.released {
		st.released = true
		v.releasedStages[stage] = struct{}{}
	}
}

// useStage returns the open stage, or block.ErrStageReleased unless it is
// open: a released stage's roots would hold blocks nothing reaches, and a lost
// one's writes would name blocks the collector freed.
func (v *Volume) useStage(ctx context.Context, stage string) (*openStage, error) {
	// Read the stage under the pin lock.
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()
	st := v.stages[stage]
	switch {
	case st == nil || st.released:
		return nil, block.ErrStageReleased
	case st.lost:
		return nil, errStageLost
	}
	return st, nil
}

// stageRootName returns the name of the root through which stage holds ref.
func stageRootName(stage string, ref *block.BlockRef) string {
	return stage + "/" + ref.MarshalString()
}

// listStageRoots returns a release of every root stage holds in p.
func listStageRoots(ctx context.Context, p *physicalTx, stage string) ([]block.NamedRoot, error) {
	var releases []block.NamedRoot
	err := p.rec.ListRoots(ctx, "", stage+"/", func(name string, entry *block_gc.RootEntry) error {
		ref, _, err := block_gc.ParseBlockKey(entry.GetRef())
		if err != nil {
			return err
		}
		releases = append(releases, block.NamedRoot{Name: name, Previous: ref})
		return nil
	})
	return releases, err
}

// PrepareStagedBlock stores a block the open stage holds. It returns
// block.ErrStageReleased after the stage is released.
func (v *Volume) PrepareStagedBlock(ctx context.Context, stage string, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Name the block, then store it under the stage.
	ctx = block.WithPutOpts(ctx, opts)
	putOpts, _ := block.PutOptsWithoutSync(opts)
	v.noteUnknownRefs(putStaged, data, putOpts)
	entry, err := putEntry(data, putOpts, v.GetHashType())
	if err != nil {
		return nil, false, err
	}
	existed, err := v.prepareStaged(ctx, stage, []*block.PutBatchEntry{entry})

	// Count the block it stored, or the refusal.
	if err == nil {
		v.countPut(putStaged, len(data), existed[0])
		return entry.Ref, existed[0], nil
	}
	v.countRefused(putStaged, err)
	return nil, false, err
}

// PrepareStagedBlockBatch stores a batch the open stage holds.
func (v *Volume) PrepareStagedBlockBatch(ctx context.Context, stage string, entries []*block.PutBatchEntry) ([]bool, error) {
	// An empty batch writes nothing.
	if len(entries) == 0 {
		return nil, nil
	}

	// Store the batch under the stage and count the blocks it stored, those
	// of the slices that landed before a refusal too, and the refusal.
	v.noteUnknownRefsBatch(putStaged, entries)
	existed, err := v.prepareStaged(ctx, stage, entries)
	v.countPutBatch(putStaged, entries[:len(existed)], existed)
	v.countRefused(putStaged, err)
	if err != nil {
		return nil, err
	}
	return existed, nil
}

// Bounds of one transaction of a staged batch.
const (
	// stageSliceEntries bounds the entries one transaction of a staged batch
	// lands.
	stageSliceEntries = 256
	// stageSliceMin is the fewest entries pacing cuts a slice to. A commit
	// has a fixed cost, an fsync on a durable store, that no smaller slice
	// removes: on a store whose commit alone exceeds stageSliceTarget, halving
	// past this floor would add commits without shortening a hold, and the
	// batch would land one entry per commit.
	stageSliceMin = 16
	// stageSliceTarget is how long one transaction of a staged batch aims to
	// hold the store's writer, from its open to its commit. A slice that ran
	// longer halves the next one, down to stageSliceMin entries, and one that
	// ran under half of it doubles the next, up to stageSliceEntries.
	stageSliceTarget = 10 * time.Millisecond
)

// prepareStaged stores entries under the stage. The stage takes a root on
// each entry it does not hold yet and releases its root on each block an
// entry names, whose holder the entry now is: so a stage holds the tops of
// what it built, and a block written through it stays held from its put on.
//
// A batch lands in slices, consecutive transactions of at most stageSlice
// entries in batch order, so no transaction holds the store's writer for more
// than one slice. Each slice keeps the caller's class: a caller that marks
// ctx with block.WithBackground, as upkeep ahead of its adopting write does,
// queues each slice behind every waiting foreground writer, so the longest
// wait it adds to one is a slice's hold; a foreground caller's slices, such
// as a World write's as it builds its state, queue in arrival order among the
// foreground writers, so a writer already waiting goes before the next one. A
// store's readers never wait on a writer. A slice ends only where no entry
// in it names a block a later entry stores, so a batch that names later
// entries lands in fewer, longer slices, and one whose first entry names its
// last lands whole. With stageSlice zero, under Config.StageOneTransaction, the
// batch lands in one transaction.
//
// It returns the existence of the entries of the slices that landed, those
// before a refusal too, with the refusal.
//
// Invariants of the sliced landing:
//
//   - A block's children land before it, in the same transaction as the stage
//     roots that hold them, or in an earlier one: slices cut a batch only
//     between entries, in order, and only after an entry up to which no
//     entry names a block a later one stores (stageCuts). A child an earlier slice landed stays held by its stage
//     root until the slice that lands its parent releases that root, so no
//     collector pass between slices can free it (heldByStage relies on this).
//   - Each transaction moves only the stage roots of its own entries and of
//     the blocks they name: its moves are computed from its slice alone,
//     after it finds the stage still open.
//   - A refusal or a cancellation partway leaves the earlier slices
//     committed, their blocks held by the stage's roots, and returns the
//     error. A retry of the batch finds those blocks held; the stage's
//     release, or its lease's expiry, lets the collector take what no parent
//     names.
func (v *Volume) prepareStaged(ctx context.Context, stage string, entries []*block.PutBatchEntry) ([]bool, error) {
	// Require an open stage, atomic publication and room above the disk
	// reserve; Land decides what a block without its child list does.
	if _, err := v.useStage(ctx, stage); err != nil {
		return nil, err
	}
	if !v.HoldsRoots() {
		return nil, block.ErrAtomicCommitUnsupported
	}
	if err := v.checkReserve(ctx, batchBytes(entries)); err != nil {
		return nil, err
	}

	// Name every entry, so the stage can name its roots.
	for _, entry := range entries {
		if entry != nil && !entry.Tombstone && entry.Ref.GetEmpty() {
			ref, err := block.BuildBlockRef(entry.Data, &block.PutOpts{HashType: v.GetHashType()})
			if err != nil {
				return nil, err
			}
			entry.Ref = ref
		}
	}

	// Land the batch whole, or in slices paced by their holds.
	if v.stageSlice == 0 || len(entries) <= v.stageSlice {
		existed, _, err := v.landStaged(ctx, stage, entries)
		return existed, err
	}
	cuts := stageCuts(entries)
	existed := make([]bool, 0, len(entries))
	n := v.stageSlice
	for s := 0; s != len(entries); {
		// Land the next slice, cut where no landed entry names a later one.
		e := nextCut(cuts, s, min(s+n, len(entries)))
		got, hold, err := v.landStaged(ctx, stage, entries[s:e])
		if err != nil {
			return existed, err
		}
		existed = append(existed, got...)
		s = e

		// Pace the slice after it by its hold.
		switch {
		case hold > stageSliceTarget:
			n = max(n/2, min(stageSliceMin, v.stageSlice))
		case hold < stageSliceTarget/2:
			n = min(n*2, v.stageSlice)
		}
	}
	return existed, nil
}

// stageCuts reports, for each i, whether entries may be cut after i: whether
// no entry up to i names a block that only a later entry stores. A slice that
// ends at such a cut holds each of its blocks' children itself, or finds them
// in an earlier slice. The last entry is always a cut.
func stageCuts(entries []*block.PutBatchEntry) []bool {
	// Index the first entry that stores each block.
	at := make(map[string]int, len(entries))
	for i, entry := range entries {
		if entry == nil || entry.Tombstone || len(entry.Data) == 0 {
			continue
		}
		key := entry.Ref.MarshalString()
		if _, ok := at[key]; !ok {
			at[key] = i
		}
	}

	// Carry the furthest entry named so far; a cut lies where it is behind.
	cuts := make([]bool, len(entries))
	reach := 0
	for i, entry := range entries {
		if entry != nil && !entry.Tombstone {
			for _, child := range entry.Refs {
				if j, ok := at[child.MarshalString()]; ok {
					reach = max(reach, j)
				}
			}
		}
		cuts[i] = reach <= i
	}
	return cuts
}

// nextCut returns the end of the slice that starts at s: the latest cut that
// keeps it to want, or past want the first cut, when none does.
func nextCut(cuts []bool, s, want int) int {
	for e := want; e > s; e-- {
		if cuts[e-1] {
			return e
		}
	}
	for e := want + 1; ; e++ {
		if cuts[e-1] {
			return e
		}
	}
}

// landStaged stores entries under the open stage and moves the stage's roots
// in one transaction, once the Volume no longer sweeps, then records the
// moves. It returns how long the last transaction held the writer. A root the
// stage's record holds that the table no longer does refuses the put, and the
// stage is lost.
func (v *Volume) landStaged(ctx context.Context, stage string, entries []*block.PutBatchEntry) ([]bool, time.Duration, error) {
	var existed []bool
	var hold time.Duration
	err := v.retrySwept(ctx, func() error {
		var err error
		existed, hold, err = v.landStagedOnce(ctx, stage, entries)
		return err
	})
	return existed, hold, err
}

// landStagedOnce is one attempt of landStaged, reading the stage's roots
// again.
func (v *Volume) landStagedOnce(ctx context.Context, stage string, entries []*block.PutBatchEntry) ([]bool, time.Duration, error) {
	// Land the entries with the moves that hold them.
	names := newStageNames(stage, entries)
	held, err := v.stageHeld(ctx, stage, names)
	if err != nil {
		return nil, 0, err
	}
	var roots []block.NamedRoot
	var existed []bool
	var opened time.Time
	err = v.withProcessLease(ctx, func(p *physicalTx, lease string) error {
		// Land the entries, holding the stage's new roots under the lease.
		opened = time.Now()
		roots = stageMoves(names, held, lease)
		var err error
		existed, err = v.land(ctx, p, "", entries, roots)
		return err
	})

	// Measure the hold, then record the moves or lose the stage.
	var hold time.Duration
	if !opened.IsZero() {
		hold = time.Since(opened)
	}
	switch {
	case err == nil:
		v.noteStageRoots(stage, roots)
	case isRootMoved(err):
		v.loseStage(stage)
		return nil, hold, errors.Wrap(errStageLost, err.Error())
	}
	return existed, hold, err
}

// stageMoves returns the root moves that hold each entry of a staged batch
// and release the stage's roots on the blocks the entries name. held is the
// stage's record of its roots among names: a release carries the record's
// block as its Previous, so a root a collector took refuses the put rather
// than leaving the parent over a freed block.
func stageMoves(names stageNames, held map[string]*block.BlockRef, lease string) []block.NamedRoot {
	// Hold each entry the stage does not hold yet, unless another entry
	// names it.
	put, named := names.put, names.named
	var roots []block.NamedRoot
	for _, name := range slices.Sorted(maps.Keys(put)) {
		_, isNamed := named[name]
		if _, isHeld := held[name]; !isNamed && !isHeld {
			roots = append(roots, block.NamedRoot{Name: name, Ref: put[name], Lease: lease})
		}
	}

	// Release the stage's roots on the named blocks.
	for _, name := range slices.Sorted(maps.Keys(named)) {
		if ref, isHeld := held[name]; isHeld {
			roots = append(roots, block.NamedRoot{Name: name, Previous: ref})
		}
	}
	return roots
}

// ReleaseStageRoots releases the open stage's roots on refs in one
// transaction, once a parent written outside the stage holds them. The stage
// stays open.
func (v *Volume) ReleaseStageRoots(ctx context.Context, stage string, refs []*block.BlockRef) error {
	var releases []block.NamedRoot
	err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
		// Find the stage roots held on refs.
		releases = nil
		for _, ref := range refs {
			if ref.GetEmpty() {
				continue
			}
			name := stageRootName(stage, ref)
			entry, err := p.rec.GetRoot(ctx, "", name)
			if err != nil {
				return err
			}
			if entry != nil {
				releases = append(releases, block.NamedRoot{Name: name, Previous: ref})
			}
		}

		// Release them.
		if len(releases) == 0 {
			return nil
		}
		_, err := v.land(ctx, p, "", nil, releases)
		return err
	})
	if err == nil {
		v.noteStageRoots(stage, releases)
	}
	return err
}

// HoldStageCompleteRoots reports which roots have a completion proof, durable
// or pending, and holds each of them under the open stage in the same
// transaction. The collector frees a proof with its root, so a held root
// keeps the whole graph its proof names until the stage releases it.
func (v *Volume) HoldStageCompleteRoots(ctx context.Context, stage string, refs []*block.BlockRef) ([]bool, error) {
	// Require an open stage.
	if _, err := v.useStage(ctx, stage); err != nil {
		return nil, err
	}

	// Check the proofs and hold the proved roots in one transaction.
	complete := make([]bool, len(refs))
	var holds []block.NamedRoot
	err := v.withProcessLease(ctx, func(p *physicalTx, lease string) error {
		// Hold each root whose proof is recorded or pending.
		holds = nil
		clear(complete)
		for i, ref := range refs {
			if ref.GetEmpty() {
				continue
			}
			ok, err := v.rootCompleteTx(ctx, p, ref)
			if err != nil || !ok {
				if err != nil {
					return err
				}
				continue
			}
			complete[i] = true
			name := stageRootName(stage, ref)
			entry, err := p.rec.GetRoot(ctx, "", name)
			if err != nil {
				return err
			}
			if entry == nil {
				holds = append(holds, block.NamedRoot{Name: name, Ref: ref, Lease: lease})
			}
		}
		if len(holds) == 0 {
			return nil
		}
		_, err := v.land(ctx, p, "", nil, holds)
		return err
	})
	if err != nil {
		return nil, err
	}
	v.noteStageRoots(stage, holds)
	return complete, nil
}
