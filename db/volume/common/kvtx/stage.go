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
// only those, not every root the stage holds, under the stage's own lock.
func (v *Volume) stageHeld(stage string, names stageNames) map[string]*block.BlockRef {
	// Find the open stage.
	st := v.openStageNamed(stage)
	if st == nil {
		return nil
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
	return held
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
// until the stage releases it. A stage that writes nothing costs no
// transaction. The release leaves the stage's roots for the next changing
// direct transaction to release, like a reader pin; closing the Volume
// releases its open stages, and the collector releases the roots of a process
// that died once their lease lapses.
func (v *Volume) OpenStage(ctx context.Context) (string, func(), error) {
	// Refuse new stages after the Volume closes.
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		return "", nil, err
	}
	defer unlock()
	if v.rootPinsClosed {
		return "", nil, volume.ErrVolumeClosed
	}

	// Register the stage.
	stage := stageRootPrefix + v.procID + "/" + ulid.NewULID()
	v.stages[stage] = &openStage{}
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

// useStage returns block.ErrStageReleased unless the stage is open: a released
// stage's roots would hold blocks nothing reaches, and a lost one's writes
// would name blocks the collector freed.
func (v *Volume) useStage(ctx context.Context, stage string) error {
	// Read the stage under the pin lock.
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	st := v.stages[stage]
	switch {
	case st == nil || st.released:
		return block.ErrStageReleased
	case st.lost:
		return errStageLost
	}
	return nil
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

	// Store the batch under the stage and count the blocks it stored, or the
	// refusal.
	v.noteUnknownRefsBatch(putStaged, entries)
	existed, err := v.prepareStaged(ctx, stage, entries)
	if err == nil {
		v.countPutBatch(putStaged, entries, existed)
	}
	v.countRefused(putStaged, err)
	return existed, err
}

// prepareStaged stores entries in one transaction. The stage takes a root on
// each entry it does not hold yet and releases its root on each block an
// entry names, whose holder the entry now is: so a stage holds the tops of
// what it built, and a block written through it stays held from its put on.
func (v *Volume) prepareStaged(ctx context.Context, stage string, entries []*block.PutBatchEntry) ([]bool, error) {
	// Require an open stage, atomic publication, room above the disk reserve
	// and the child list of every block.
	if err := v.useStage(ctx, stage); err != nil {
		return nil, err
	}
	if !v.HoldsRoots() {
		return nil, block.ErrAtomicCommitUnsupported
	}
	if err := v.checkReserve(ctx, batchBytes(entries)); err != nil {
		return nil, err
	}
	if err := requireRefsKnown(entries); err != nil {
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

	// Store the entries and move the stage's roots in one transaction, then
	// record the moves. A root the stage's record holds that the table no
	// longer does refuses the put, and the stage is lost.
	names := newStageNames(stage, entries)
	roots := stageMoves(names, v.stageHeld(stage, names), v.leaseExpiry())
	var existed []bool
	err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
		var err error
		existed, err = v.land(ctx, p, "", entries, roots)
		return err
	})
	switch {
	case err == nil:
		v.noteStageRoots(stage, roots)
	case isRootMoved(err):
		v.loseStage(stage)
		return nil, errors.Wrap(errStageLost, err.Error())
	}
	return existed, err
}

// stageMoves returns the root moves that hold each entry of a staged batch
// and release the stage's roots on the blocks the entries name. held is the
// stage's record of its roots among names: a release carries the record's
// block as its Previous, so a root a collector took refuses the put rather
// than leaving the parent over a freed block.
func stageMoves(names stageNames, held map[string]*block.BlockRef, expires time.Time) []block.NamedRoot {
	// Hold each entry the stage does not hold yet, unless another entry
	// names it.
	put, named := names.put, names.named
	var roots []block.NamedRoot
	for _, name := range slices.Sorted(maps.Keys(put)) {
		_, isNamed := named[name]
		if _, isHeld := held[name]; !isNamed && !isHeld {
			roots = append(roots, block.NamedRoot{Name: name, Ref: put[name], Expires: expires})
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
	if err := v.useStage(ctx, stage); err != nil {
		return nil, err
	}

	// Check the proofs and hold the proved roots in one transaction.
	complete := make([]bool, len(refs))
	var holds []block.NamedRoot
	err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
		// Hold each root whose proof is recorded or pending.
		expires := v.leaseExpiry()
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
				holds = append(holds, block.NamedRoot{Name: name, Ref: ref, Expires: expires})
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
