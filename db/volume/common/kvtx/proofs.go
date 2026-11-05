package kvtx

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// rootProofCarryLimit bounds the pending proofs one transaction carries. Each
// carried proof costs an existence check and a record write under the writer
// lock, so a World commit never pays for more than this many.
const rootProofCarryLimit = 256

// rootProofBacklog bounds the pending proofs. A marker that leaves more than
// one batch over it commits batches itself until the backlog fits, so a copy
// that proves blocks faster than other writers commit waits its turn on the
// writer instead of handing the work to their commits. The batch of slack
// keeps a World commit, which marks one root, from paying for the copy.
const rootProofBacklog = 4 * rootProofCarryLimit

// MarkRootsComplete records completion proofs for stored roots: the graph
// under each is stored. A proof outlives the eviction of a block below its
// root, which RmBlock makes only for bytes the store can fetch again, such as
// the writeback cache's blocks its remote holds: eviction keeps the block's
// child list and count, so the proved graph stays whole in liveness and a
// read refills the bytes. The World copy, the one consumer, skips and holds a
// proved subtree on that basis. The collector deletes a proof with its block. A proof stays pending in memory until a changing
// direct transaction carries it, so marking usually costs no write barrier of
// its own. Past rootProofBacklog the marker carries batches itself. A crash
// loses only pending proofs, which a later copy recomputes.
func (v *Volume) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	// Record the proofs for the next commits to carry.
	if err := v.recordRootProofs(ctx, roots); err != nil {
		return err
	}

	// Carry the backlog past the slack so other commits stay bounded.
	return v.carryRootProofBacklog(ctx)
}

// carryRootProofBacklog commits batches of pending proofs once more than
// rootProofBacklog+rootProofCarryLimit are pending, until at most
// rootProofBacklog remain. It stops when a batch carries nothing, which leaves
// proofs whose roots are gone for a later carry to drop.
func (v *Volume) carryRootProofBacklog(ctx context.Context) error {
	// Leave carrying to other writers when the store has no atomic publication.
	if !v.SupportsRootRetention() {
		return nil
	}

	// Leave a backlog within one batch of slack to other commits.
	pending := v.pendingRootProofs()
	if pending <= rootProofBacklog+rootProofCarryLimit {
		return nil
	}

	// Commit batches until the backlog fits or a batch carries nothing.
	for pending > rootProofBacklog {
		if err := v.withDirectAtomic(ctx, nil); err != nil {
			return err
		}
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

// carryRootProofs writes into p the proofs of at most rootProofCarryLimit
// pending roots whose bytes p still holds, and returns the keys of all it
// took, so the commit forgets them.
func (v *Volume) carryRootProofs(ctx context.Context, p *physicalTx) ([]string, error) {
	// Take at most rootProofCarryLimit pending root proofs.
	proofs := make(map[string]*block.BlockRef)
	v.proofMu.Lock()
	for key, root := range v.rootProofs {
		if len(proofs) == rootProofCarryLimit {
			break
		}
		proofs[key] = root
	}
	v.proofMu.Unlock()

	// Write the proof of each root still stored.
	keys := make([]string, 0, len(proofs))
	for key, root := range proofs {
		keys = append(keys, key)
		found, err := p.blocks.GetBlockExists(ctx, root)
		if err != nil {
			return keys, err
		}
		if found {
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

// RootsComplete checks each root for a pending proof, then for its proof
// record, reading the records of all roots in one snapshot.
func (v *Volume) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Accept the proofs still waiting for a changing transaction, and collect
	// the roots that need their record read.
	complete := make([]bool, len(refs))
	var durable []int
	v.proofMu.Lock()
	for i, ref := range refs {
		if ref.GetEmpty() {
			continue
		}
		if _, pending := v.rootProofs[string(block_gc.BlockKey(ref))]; pending {
			complete[i] = true
		} else {
			durable = append(durable, i)
		}
	}
	v.proofMu.Unlock()
	if len(durable) == 0 || !v.SupportsRootRetention() {
		return complete, nil
	}

	// Read the proof records in one snapshot.
	err := v.readRecords(ctx, func(rec *block_gc.Records) error {
		for _, i := range durable {
			ok, err := rec.HasProof(ctx, refs[i])
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
// proof counts while the transaction still holds the root's bytes.
func (v *Volume) rootCompleteTx(ctx context.Context, p *physicalTx, ref *block.BlockRef) (bool, error) {
	// Accept a pending proof whose root is stored.
	v.proofMu.Lock()
	_, pending := v.rootProofs[string(block_gc.BlockKey(ref))]
	v.proofMu.Unlock()
	if pending {
		return p.blocks.GetBlockExists(ctx, ref)
	}

	// Otherwise read the proof record.
	return p.rec.HasProof(ctx, ref)
}
