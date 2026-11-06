package kvtx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// rootProofRide bounds the pending proofs another writer's commit carries.
// Each carried proof costs an existence check and a record write under the
// writer lock, so a World commit, which marks its own root, rides the next
// commit and pays for a few proofs at most.
const rootProofRide = 16

// rootProofSlice bounds the proofs one transaction of a marker writes, so a
// copy's batch holds the writer in short slices.
const rootProofSlice = 256

// MarkRootsComplete records completion proofs for stored roots: the graph
// under each is stored. A proof outlives the eviction of a block below its
// root, which RmBlock makes only for bytes the store can fetch again, such as
// the writeback cache's blocks its remote holds: eviction keeps the block's
// child list and count, so the proved graph stays whole in liveness and a
// read refills the bytes. The World copy, the one consumer, skips and holds a
// proved subtree on that basis. The collector deletes a proof when it starts
// releasing the block's children, since it may free them before the block,
// and with the block when it frees it; a pending proof counts only while its
// root is held.
//
// A proof stays pending in memory until a transaction carries it. A mark of
// one root, such as the derived proof of a World commit, leaves it for the
// next changing direct transaction, so it costs no write barrier of its own.
// A mark of several roots, such as a copy's batch, or one that leaves more
// than rootProofRide pending, writes the pending proofs itself, rootProofSlice
// per transaction, so other writers' commits never pay for a copy. A crash
// loses only pending proofs, which a later copy recomputes.
func (v *Volume) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	// Record the proofs for a transaction to carry.
	if err := v.recordRootProofs(ctx, roots); err != nil {
		return err
	}

	// Carry them here unless one rides the next commit.
	if len(roots) <= 1 && v.pendingRootProofs() <= rootProofRide {
		return nil
	}
	return v.carryPendingRootProofs(ctx)
}

// carryPendingRootProofs writes the pending proofs in transactions of their
// own, rootProofSlice per transaction, until none remain. It stops when a
// transaction carries nothing, which leaves proofs whose roots are gone for a
// later carry to drop.
func (v *Volume) carryPendingRootProofs(ctx context.Context) error {
	// Leave carrying to other writers when the store has no atomic publication.
	if !v.HoldsRoots() {
		return nil
	}

	// Commit slices until no proof is pending or a slice carries nothing.
	for pending := v.pendingRootProofs(); pending != 0; {
		var keys []string
		err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
			var err error
			keys, err = v.carryRootProofs(ctx, p, rootProofSlice)
			return err
		})
		if err != nil {
			return err
		}
		v.forgetRootProofs(keys)
		next := v.pendingRootProofs()
		if next >= pending {
			return nil
		}
		pending = next
	}
	return nil
}

// pendingRootProofs returns the number of proofs no commit has carried yet.
func (v *Volume) pendingRootProofs() int {
	v.proofMu.Lock()
	defer v.proofMu.Unlock()
	return len(v.rootProofs)
}

// recordRootProofs checks that the roots are stored and records their pending
// proofs.
func (v *Volume) recordRootProofs(ctx context.Context, roots []*block.BlockRef) error {
	for {
		// Note the frees attempted before checking that the roots exist.
		v.proofMu.Lock()
		frees := v.frees
		v.proofMu.Unlock()

		// Require stored bytes before recording a root completion proof. Read
		// the store, not an admitted publication that may still fail.
		for _, root := range roots {
			if root.GetEmpty() {
				continue
			}
			found, err := v.Store.GetBlockExists(ctx, root)
			if err != nil {
				return err
			}
			if !found {
				return errors.Wrap(block.NotFoundError(root), "root proof")
			}
		}

		// Record the proofs unless the collector may have freed a root after
		// its check, in which case check again.
		v.proofMu.Lock()
		recorded := v.frees == frees
		if recorded {
			if v.rootProofs == nil {
				v.rootProofs = make(map[string]*block.BlockRef)
			}
			for _, root := range roots {
				if !root.GetEmpty() {
					v.rootProofs[string(block_gc.BlockKey(root))] = root
				}
			}
		}
		v.proofMu.Unlock()
		if recorded {
			return nil
		}
	}
}

// carryRootProofs writes into p the proofs of at most limit pending roots
// that p still holds, and returns the keys of all it took, so the commit
// forgets them.
func (v *Volume) carryRootProofs(ctx context.Context, p *physicalTx, limit int) ([]string, error) {
	// Take at most limit pending root proofs.
	proofs := make(map[string]*block.BlockRef)
	v.proofMu.Lock()
	for key, root := range v.rootProofs {
		if len(proofs) == limit {
			break
		}
		proofs[key] = root
	}
	v.proofMu.Unlock()

	// Write the proof of each root still held.
	keys := make([]string, 0, len(proofs))
	for key, root := range proofs {
		keys = append(keys, key)
		held, err := p.rec.Held(ctx, root)
		if err != nil {
			return keys, err
		}
		if held {
			if err := p.rec.SetProof(ctx, root); err != nil {
				return keys, err
			}
		}
	}
	return keys, nil
}

// forgetRootProofs drops the pending proofs a committed transaction wrote.
func (v *Volume) forgetRootProofs(keys []string) {
	if len(keys) == 0 {
		return
	}
	v.proofMu.Lock()
	defer v.proofMu.Unlock()
	for _, key := range keys {
		delete(v.rootProofs, key)
	}
}

// forgetFreedProofs drops the pending proofs of blocks the collector freed and
// counts the free, once its transaction has committed, so a proof recorded
// against a check made before the free is checked again.
func (v *Volume) forgetFreedProofs(refs []*block.BlockRef) {
	// Count the free and drop the freed blocks' pending proofs.
	if len(refs) == 0 {
		return
	}
	v.proofMu.Lock()
	defer v.proofMu.Unlock()
	v.frees++
	for _, ref := range refs {
		delete(v.rootProofs, string(block_gc.BlockKey(ref)))
	}
}

// RootsComplete checks each root for a pending proof whose root is held, or
// for its proof record, reading the records of all roots in one snapshot.
func (v *Volume) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Note which roots have a pending proof.
	complete := make([]bool, len(refs))
	pending := make([]bool, len(refs))
	v.proofMu.Lock()
	for i, ref := range refs {
		if !ref.GetEmpty() {
			_, pending[i] = v.rootProofs[string(block_gc.BlockKey(ref))]
		}
	}
	v.proofMu.Unlock()

	// Accept the pending proofs as they are without atomic publication.
	if !v.HoldsRoots() {
		return pending, nil
	}

	// Read whether each pending root is held, or each other root's proof
	// record, in one snapshot.
	err := v.readRecords(ctx, func(rec *block_gc.Records) error {
		for i, ref := range refs {
			if ref.GetEmpty() {
				continue
			}
			var ok bool
			var err error
			if pending[i] {
				ok, err = rec.Held(ctx, ref)
			} else {
				ok, err = rec.HasProof(ctx, ref)
			}
			if err != nil {
				return err
			}
			complete[i] = ok
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return complete, nil
}

// rootCompleteTx checks a root's proof inside a direct transaction. A pending
// proof counts while the transaction still holds the root.
func (v *Volume) rootCompleteTx(ctx context.Context, p *physicalTx, ref *block.BlockRef) (bool, error) {
	// Accept a pending proof whose root is held.
	v.proofMu.Lock()
	_, pending := v.rootProofs[string(block_gc.BlockKey(ref))]
	v.proofMu.Unlock()
	if pending {
		return p.rec.Held(ctx, ref)
	}

	// Otherwise read the proof record.
	return p.rec.HasProof(ctx, ref)
}
