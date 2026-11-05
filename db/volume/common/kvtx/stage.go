package kvtx

import (
	"context"

	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// openStage is the in-process state of one stage.
type openStage struct {
	// released records that the stage's release was called; the next changing
	// direct transaction releases its roots.
	released bool
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
		return "", nil, block.ErrPublicationClosed
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
	if st := v.stages[stage]; st != nil {
		st.released = true
	}
}

// useStage returns block.ErrStageReleased unless the stage is open: a released
// stage's roots would hold blocks nothing reaches.
func (v *Volume) useStage(ctx context.Context, stage string) error {
	// Read the stage under the pin lock.
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		return err
	}
	defer unlock()
	if st := v.stages[stage]; st == nil || st.released {
		return block.ErrStageReleased
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
	if !v.SupportsAtomicPublication() {
		return nil, block.ErrAtomicPublicationUnsupported
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

	// Store the entries and move the stage's roots in one transaction.
	var existed []bool
	err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
		roots, err := v.stageMoves(ctx, p, stage, entries)
		if err != nil {
			return err
		}
		existed, err = v.land(ctx, p, "", entries, roots)
		return err
	})
	return existed, err
}

// stageMoves returns the root moves that hold each entry under stage and
// release the stage's roots on the blocks the entries name.
func (v *Volume) stageMoves(ctx context.Context, p *physicalTx, stage string, entries []*block.PutBatchEntry) ([]block.NamedRoot, error) {
	// Find the entries and the blocks they name.
	expires := v.leaseExpiry()
	put := make(map[string]*block.BlockRef)
	named := make(map[string]*block.BlockRef)
	for _, entry := range entries {
		if entry == nil || entry.Tombstone || len(entry.Data) == 0 {
			continue
		}
		put[stageRootName(stage, entry.Ref)] = entry.Ref
		for _, child := range entry.Refs {
			named[stageRootName(stage, child)] = child
		}
	}

	// Hold each entry the stage does not hold yet, unless another entry
	// names it.
	var roots []block.NamedRoot
	for name, ref := range put {
		if _, ok := named[name]; ok {
			continue
		}
		entry, err := p.rec.GetRoot(ctx, "", name)
		if err != nil {
			return nil, err
		}
		if entry == nil {
			roots = append(roots, block.NamedRoot{Name: name, Ref: ref, Expires: expires})
		}
	}

	// Release the stage's roots on the named blocks.
	for name, ref := range named {
		entry, err := p.rec.GetRoot(ctx, "", name)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			roots = append(roots, block.NamedRoot{Name: name, Previous: ref})
		}
	}
	return roots, nil
}

// ReleaseStageRoots releases the open stage's roots on refs in one
// transaction, once a parent written outside the stage holds them. The stage
// stays open.
func (v *Volume) ReleaseStageRoots(ctx context.Context, stage string, refs []*block.BlockRef) error {
	return v.withDirectAtomic(ctx, func(p *physicalTx) error {
		// Find the stage roots held on refs.
		var releases []block.NamedRoot
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
	err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
		// Hold each root whose proof is recorded or pending.
		expires := v.leaseExpiry()
		var holds []block.NamedRoot
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
	return complete, nil
}
