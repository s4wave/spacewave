package kvtx

import (
	"context"
	"errors"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/hash"
)

// withDirectAtomic runs fn in one physical transaction of the Volume's store,
// the domain the publication writer commits in, and commits it. It joins
// direct writes on Close, and takes no coordinator lease or publication guard.
//
// Every commit costs a write barrier, so a transaction fn leaves unchanged ends
// without one. A changing transaction also carries the Volume's deferred
// edits, the releases of reader pins and stages and up to rootProofRide
// pending root proofs, which therefore cost no barrier of their own. A nil fn
// applies only the deferred edits.
//
// It commits with write ordering when the store supports it. A direct write is
// safe to lose in a crash as long as no earlier commit is lost: the journal
// record it appends goes with it. The head publication that follows commits
// fully and makes these writes durable, as does Sync.
func (v *Volume) withDirectAtomic(ctx context.Context, fn func(p *physicalTx) error) error {
	return v.withDirect(ctx, false, fn)
}

// withDirectPlain runs fn as withDirectAtomic does, for the plain block
// writes of the block.StoreOps contract, with a full commit: such a write is
// durable when it returns unless ctx asks for write ordering.
func (v *Volume) withDirectPlain(ctx context.Context, fn func(p *physicalTx) error) error {
	return v.withDirect(ctx, true, fn)
}

// withDirect runs fn as withDirectAtomic describes, committing fully when
// durable is set.
//
// A Land in fn that holds a pending block whose promotion needs more reads
// than one transaction makes returns a block_gc.PromoteError; withDirect then
// promotes the block in slices, outside the transaction, and runs fn again,
// so fn must be safe to run again. Each promotion leaves its block clean for
// good, so the retries end.
func (v *Volume) withDirect(ctx context.Context, durable bool, fn func(p *physicalTx) error) error {
	for {
		err := v.withDirectOnce(ctx, durable, fn)
		var pe *block_gc.PromoteError
		if !errors.As(err, &pe) {
			return err
		}
		if err := block_gc.Promote(ctx, collectorStore{v: v}, pe.Ref); err != nil {
			return err
		}
	}
}

// withDirectOnce runs fn once as withDirect describes.
func (v *Volume) withDirectOnce(ctx context.Context, durable bool, fn func(p *physicalTx) error) error {
	// Join direct Volume operations and reject writes after storage closes.
	v.directMu.RLock()
	defer v.directMu.RUnlock()
	if v.directClosed {
		return volume.ErrVolumeClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Open one physical transaction for block bytes and liveness records,
	// loading the suspect gate before its snapshot, to close the gate on, and
	// after, holding the store's writer, to skip suspect reads on, as
	// suspectGate describes.
	epoch, _ := v.gate.load()
	p, err := v.openPhysical(ctx)
	if err != nil {
		return err
	}
	defer p.discard()
	if _, closed := v.gate.load(); closed {
		p.rec.AssumeNoSuspects()
	}

	// Apply the direct operation.
	if fn != nil {
		if err := fn(p); err != nil {
			return err
		}
	}

	// Carry the deferred edits in a changing transaction only, and end an
	// unchanged one without a commit.
	var edits deferredEdits
	if fn == nil || p.wrote() {
		edits, err = v.applyDeferred(ctx, p)
	}
	d := &directTx{v: v, p: p, durable: durable, epoch: epoch}
	if err == nil {
		err = d.Commit(ctx)
	}

	// Settle the deferred edits, report the committed markers and wake Volume
	// statistics watchers.
	v.settleDeferred(edits, d.committed)
	d.settle()
	return err
}

// withDirectFill runs fn as withDirectAtomic does, for cache fills, in a
// transaction the store may open optimistically: fn reads without the store's
// writer, so a fill reading cold pages after a restart delays no other
// writer, and a commit after its snapshot that changed a record it read runs
// it again. After repeated conflicts, or while kvtx.OptimisticEnv is "off", it
// holds the writer throughout as withDirectAtomic does.
//
// A fill lands through block_gc.LandFill, which reads only the child list of
// each block it puts and the Volume's hole records, and writes its pairs to a
// fill record of its own, blind, so it conflicts only with a commit that
// stores or frees one of its blocks, or with a write of a hole record.
//
// It carries no deferred edits: they reserve in-memory pins once per
// transaction, and the writers holding the store's writer carry them. fn runs
// once per attempt and must be safe to run again.
func (v *Volume) withDirectFill(ctx context.Context, fn func(p *physicalTx) error) error {
	// Join direct Volume operations and reject writes after storage closes.
	v.directMu.RLock()
	defer v.directMu.RUnlock()
	if v.directClosed {
		return volume.ErrVolumeClosed
	}

	// Skip suspect reads while the gate is closed, registering with it, since
	// an optimistic attempt holds no writer, as suspectGate describes.
	assume, release := v.gate.assume()
	defer release()

	// Run fn in each attempt, keeping the last.
	var last *directTx
	open := func(ctx context.Context) (*directTx, error) {
		// Load the gate before the snapshot opens, as withDirectOnce does.
		epoch, _ := v.gate.load()
		p, err := v.openPhysical(ctx)
		if err != nil {
			return nil, err
		}
		if assume {
			p.rec.AssumeNoSuspects()
		}
		last = &directTx{v: v, p: p, epoch: epoch}
		return last, nil
	}
	err := kvtx.RunOptimisticTransaction(ctx, open, func(_ context.Context, d *directTx) error {
		return fn(d.p)
	}, func(error) bool { return false })

	// Report the committed markers and wake Volume statistics watchers.
	if last != nil {
		last.settle()
	}
	return err
}

// directTx commits a physical transaction under the Volume's write-ordering
// contract.
type directTx struct {
	// v is the Volume.
	v *Volume
	// p is the transaction.
	p *physicalTx
	// durable commits p fully, as the store's Commit does for ctx, rather
	// than with write ordering.
	durable bool
	// committed records that Commit committed p.
	committed bool
	// epoch is the suspect gate's epoch loaded before p's snapshot opened.
	epoch uint64
}

// Commit commits p with write ordering when the Volume orders writes and the
// transaction is not durable, and ends an unchanged transaction without a
// commit.
func (d *directTx) Commit(ctx context.Context) error {
	// Skip the commit of an unchanged transaction.
	if !d.p.wrote() {
		return nil
	}

	// Commit using the Volume write-ordering contract.
	var err error
	if d.v.ordered != nil && !d.durable {
		err = kvtx.CommitOrdered(ctx, d.p.tx)
	} else {
		err = d.p.tx.Commit(ctx)
	}
	d.committed = err == nil
	return err
}

// Discard ends p.
func (d *directTx) Discard() {
	d.p.discard()
}

// settle reports the committed markers, forgets the pending proofs of the
// blocks p removed, wakes Volume statistics watchers, closes the suspect gate
// when p saw no suspect records and, when p returned the Volume to
// unadopted, starts the recount, once p committed.
func (d *directTx) settle() {
	if d.committed {
		if d.p.rec.NoSuspects() {
			d.v.gate.close(d.epoch)
		}
		d.p.blocks.marks.committed()
		d.v.forgetFreedProofs(d.p.blocks.removedRefs())
		d.v.broadcastStorageStatsChanged()
		if d.p.rec.Unadopted() {
			d.v.wakeAdopt()
		}
	}
}

// land stores entries and moves roots of bucketID in p, as block_gc.Land
// describes, at the Volume's clock.
func (v *Volume) land(ctx context.Context, p *physicalTx, bucketID string, entries []*block.PutBatchEntry, roots []block.NamedRoot) ([]bool, error) {
	return v.landMoves(ctx, p, entries, block_gc.BucketRoots{BucketID: bucketID, Roots: roots})
}

// landMoves stores entries and moves the roots of each bucket in p in one
// Land, as block_gc.Land describes, at the Volume's clock.
func (v *Volume) landMoves(ctx context.Context, p *physicalTx, entries []*block.PutBatchEntry, moves ...block_gc.BucketRoots) ([]bool, error) {
	return block_gc.Land(ctx, p.rec, p.blocks, v.now(), entries, moves...)
}

// now returns the Volume's clock.
func (v *Volume) now() time.Time {
	if v.clock != nil {
		return v.clock()
	}
	return time.Now()
}

// PrepareOwnedBlock durably stores a potentially oversized block in one
// physical transaction. Unlike queue admission this call retains no caller
// data after return. bucketID names the bucket the write came through; a put
// holds nothing in the bucket, so a block no root reaches stays until the
// audit finds it.
func (v *Volume) PrepareOwnedBlock(ctx context.Context, bucketID string, data []byte, opts *block.PutOpts) (ref *block.BlockRef, existed bool, err error) {
	// Send a cache fill to the fill writer, which counts it.
	putOpts, _ := block.PutOptsWithoutSync(opts)
	if putOpts.GetCacheFill() {
		v.noteUnknownRefs(putFill, data, putOpts)
		return v.prepareCacheFill(ctx, bucketID, data, putOpts)
	}
	v.noteUnknownRefs(putOwned, data, putOpts)

	// Store the block and count it.
	entry, err := putEntry(data, putOpts, v.GetHashType())
	if err == nil {
		var found []bool
		found, err = v.putEntries(ctx, v.withDirectAtomic, []*block.PutBatchEntry{entry})
		if err == nil {
			ref, existed = entry.Ref, found[0]
			v.countPut(putOwned, len(data), existed)
		}
	}
	v.countRefused(putOwned, err)
	return ref, existed, err
}

// PrepareOwnedBlockBatch stores a batch in one physical transaction, as
// PrepareOwnedBlock does for one block.
func (v *Volume) PrepareOwnedBlockBatch(ctx context.Context, _ string, entries []*block.PutBatchEntry) (existed []bool, err error) {
	// An empty batch writes nothing.
	if len(entries) == 0 && v.HoldsRoots() {
		return nil, nil
	}

	// Store the entries and count them.
	v.noteUnknownRefsBatch(putOwned, entries)
	existed, err = v.putEntries(ctx, v.withDirectAtomic, entries)
	if err == nil {
		v.countPutBatch(putOwned, entries, existed)
	}
	v.countRefused(putOwned, err)
	return existed, err
}

// putEntries stores entries through direct, withDirectAtomic or
// withDirectFill, after refusing a block without its child list and new data
// below the disk reserve.
func (v *Volume) putEntries(
	ctx context.Context,
	direct func(context.Context, func(*physicalTx) error) error,
	entries []*block.PutBatchEntry,
) ([]bool, error) {
	// Require atomic publication, refuse new data below the disk reserve and
	// refuse a block without its child list.
	if !v.HoldsRoots() {
		return nil, block.ErrAtomicCommitUnsupported
	}
	if !onlyTombstones(entries) {
		if err := v.checkReserve(ctx, batchBytes(entries)); err != nil {
			return nil, err
		}
	}
	if err := requireRefsKnown(entries); err != nil {
		return nil, err
	}

	// Store them in one transaction.
	var existed []bool
	err := direct(ctx, func(p *physicalTx) error {
		var err error
		existed, err = v.land(ctx, p, "", entries, nil)
		return err
	})
	return existed, err
}

// putEntry returns the batch entry of a PutBlock, named the way the store
// names it.
func putEntry(data []byte, opts *block.PutOpts, hashType hash.HashType) (*block.PutBatchEntry, error) {
	ref := opts.GetForceBlockRef()
	if ref.GetEmpty() {
		var err error
		ref, err = block.BuildBlockRef(data, &block.PutOpts{HashType: opts.SelectHashType(hashType)})
		if err != nil {
			return nil, err
		}
	}
	return &block.PutBatchEntry{Ref: ref, Data: data, Refs: opts.GetRefs(), RefsKnown: opts.GetRefsKnown()}, nil
}
