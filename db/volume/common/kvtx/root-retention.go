package kvtx

import (
	"context"
	"encoding/base64"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/coord"
)

// rootPinPrefix prefixes the owner node of one process's reader pins and
// stages.
const rootPinPrefix = "reader:"

// RootOwnerPrefix prefixes the owner node of each named bucket root. A bucket
// that owns such a node retains its blocks through its named roots.
const RootOwnerPrefix = "head:"

// completeWorldNode is a graph-only proof target. Sweeping a root removes its
// proof edge together with its other immutable dependencies.
const completeWorldNode = "world:complete"

// MarkRootsComplete records completion proofs for stored roots. A proof stays
// pending in memory until the next changing direct transaction carries it, so
// marking costs no write barrier of its own. A crash loses only pending proofs,
// which a later retention recomputes.
func (v *Volume) MarkRootsComplete(ctx context.Context, roots []*block.BlockRef) error {
	for {
		// Note the sweeps attempted before checking that the roots exist.
		v.proofMu.Lock()
		sweeps := v.sweeps
		v.proofMu.Unlock()

		// Require stored bytes before recording a root completion proof.
		for _, root := range roots {
			if root.GetEmpty() {
				continue
			}
			found, err := v.GetBlockExists(ctx, root)
			if err != nil {
				return err
			}
			if !found {
				return block.ErrNotFound
			}
		}

		// Record the proofs unless a sweep may have removed a root after its
		// check, in which case check again.
		v.proofMu.Lock()
		recorded := v.sweeps == sweeps
		if recorded {
			if v.rootProofs == nil {
				v.rootProofs = make(map[string]*block.BlockRef)
			}
			for _, root := range roots {
				if !root.GetEmpty() {
					v.rootProofs[block_gc.BlockIRI(root)] = root
				}
			}
		}
		v.proofMu.Unlock()
		if recorded {
			return nil
		}
	}
}

// RootComplete checks for a pending proof, then for the proof in the volume
// ownership graph.
func (v *Volume) RootComplete(ctx context.Context, ref *block.BlockRef) (bool, error) {
	// Accept a proof still waiting for a changing transaction.
	node := block_gc.BlockIRI(ref)
	v.proofMu.Lock()
	_, pending := v.rootProofs[node]
	v.proofMu.Unlock()
	if pending {
		return true, nil
	}

	// Otherwise read the durable proof edge.
	refs, err := v.refGraph.GetOutgoingRefs(ctx, node)
	return slices.Contains(refs, completeWorldNode), err
}

// forgetSweptProofs drops the pending proofs of nodes a sweep transaction may
// have removed and counts the sweep, once the transaction has finished.
func (v *Volume) forgetSweptProofs(nodes []string) {
	// A transaction that swept no nodes invalidates no proof.
	if len(nodes) == 0 {
		return
	}
	v.proofMu.Lock()
	defer v.proofMu.Unlock()
	v.sweeps++
	for _, node := range nodes {
		delete(v.rootProofs, node)
	}
}

// SetBucketRoots writes entries owned by a bucket and replaces its named
// roots in one physical transaction, so a root may hold a block entries
// writes. Ownership changes share the transaction with sweep eligibility and
// existence checks.
func (v *Volume) SetBucketRoots(ctx context.Context, bucketID string, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	if !v.SupportsAtomicPublication() {
		return block.ErrAtomicPublicationUnsupported
	}
	return v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) error {
		// Write the entries under the bucket's ownership.
		if len(entries) != 0 {
			bucket := block_gc.BucketIRI(bucketID)
			if err := claimBucket(ctx, rg, bucket); err != nil {
				return err
			}
			gc := block_gc.NewGCStoreOpsWithParentAndTraceTask(blocks, rg, bucket, block_gc.BucketFlushTask())
			if err := gc.PutBlockBatch(ctx, entries); err != nil {
				return err
			}
			if err := gc.FlushPending(ctx); err != nil {
				return err
			}
		}

		// Move each named root.
		for _, root := range roots {
			if err := setBucketRoot(ctx, blocks, rg, bucketID, root.Name, root.Ref); err != nil {
				return err
			}
		}
		return nil
	})
}

// setBucketRoot requires a shared physical transaction for bytes and graph.
func setBucketRoot(ctx context.Context, blocks block.StoreOps, rg *block_gc.RefGraph, bucketID, name string, ref *block.BlockRef) error {
	// Read the root the owner node holds now.
	owner := RootOwnerPrefix + base64.RawURLEncoding.EncodeToString([]byte(bucketID)) + "/" + base64.RawURLEncoding.EncodeToString([]byte(name))
	bucket := block_gc.BucketIRI(bucketID)
	old, err := rg.GetOutgoingRefs(ctx, owner)
	if err != nil {
		return err
	}

	// Move a present root under the owner node, or drop an emptied owner.
	var adds, removes []block_gc.RefEdge
	next := block_gc.BlockIRI(ref)
	if !ref.GetEmpty() {
		found, err := blocks.GetBlockExists(ctx, ref)
		if err != nil {
			return err
		}
		if !found {
			return block.ErrNotFound
		}
		// Named roots belong to the bucket so normal account/Space deletion
		// also releases them. Reader pins have an independent lifetime.
		adds = append(adds,
			block_gc.RefEdge{Subject: block_gc.NodeGCRoot, Object: bucket},
			block_gc.RefEdge{Subject: bucket, Object: owner},
			block_gc.RefEdge{Subject: owner, Object: next},
		)
		removes = append(removes, block_gc.RefEdge{Subject: bucket, Object: next})
	} else {
		removes = append(removes, block_gc.RefEdge{Subject: bucket, Object: owner})
	}

	// Release the previous roots in the same batch.
	for _, previous := range old {
		if previous != next {
			removes = append(removes, block_gc.RefEdge{Subject: owner, Object: previous})
		}
	}
	return rg.ApplyRefBatch(ctx, adds, removes)
}

// ReleaseBucketRoots drops the bucket's staging edges to roots in one
// transaction. A root another block, named root or pin holds survives; the
// next sweep collects the rest. A lost release only leaves the roots staged.
func (v *Volume) ReleaseBucketRoots(ctx context.Context, bucketID string, refs []*block.BlockRef) error {
	return v.releaseOwnerRoots(ctx, block_gc.BucketIRI(bucketID), refs)
}

// ReleaseStageRoots drops an open stage's edges to roots in one transaction,
// as ReleaseBucketRoots does for a bucket. The stage stays open.
func (v *Volume) ReleaseStageRoots(ctx context.Context, stage string, refs []*block.BlockRef) error {
	return v.releaseOwnerRoots(ctx, stage, refs)
}

// releaseOwnerRoots drops the owner node's edges to roots in one transaction.
func (v *Volume) releaseOwnerRoots(ctx context.Context, owner string, refs []*block.BlockRef) error {
	// Collect the staging edges to remove.
	removes := make([]block_gc.RefEdge, 0, len(refs))
	for _, ref := range refs {
		if !ref.GetEmpty() {
			removes = append(removes, block_gc.RefEdge{Subject: owner, Object: block_gc.BlockIRI(ref)})
		}
	}
	if len(removes) == 0 {
		return nil
	}

	// Remove them, marking roots left without an owner for the sweep.
	return v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) error {
		return rg.ApplyRefBatch(ctx, nil, removes)
	})
}

// PinBucketRoot couples a durable reader edge to the volume's existing
// cross-process lease. A process crash releases the lease; the next sweep
// removes its abandoned edge without disturbing another process's readers.
func (v *Volume) PinBucketRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	_, release, err := v.pinRoot(ctx, ref, false)
	return release, err
}

// rootPin counts the in-process readers of one root. A root whose last reader
// leaves keeps its durable edge at count zero until a changing direct
// transaction carries the removal, and a new reader meanwhile reuses the edge.
type rootPin struct {
	// count is the number of unreleased pins.
	count int
	// settled is non-nil while the node's durable edge is being written or
	// removed, and closes when that write finishes.
	settled chan struct{}
}

// pinRoot reserves an in-process reader count. Prepared roots get their
// persistent edge from the publication transaction; ordinary readers write it
// before returning. Both use the same crash-recoverable volume lease.
//
// rootPinMu guards only the counts. Edge writes run outside it, so pinning an
// already retained root never waits for another root's volume transaction.
func (v *Volume) pinRoot(ctx context.Context, ref *block.BlockRef, prepared bool) (string, func(), error) {
	node := block_gc.BlockIRI(ref)
	for {
		unlock, err := v.rootPinMu.Lock(ctx)
		if err != nil {
			return "", nil, err
		}
		owner, err := v.rootPinOwnerLocked(ctx)
		if err != nil {
			unlock()
			return "", nil, err
		}

		// Wait out an edge write in flight, then observe its result.
		pin := v.rootPins[node]
		if pin != nil && pin.settled != nil {
			settled := pin.settled
			unlock()
			select {
			case <-ctx.Done():
				return "", nil, context.Cause(ctx)
			case <-settled:
			}
			continue
		}
		release := sync.OnceFunc(func() { v.unpinRoot(node) })

		// A retained root only gains a reader.
		if pin != nil {
			pin.count++
			unlock()
			return owner, release, nil
		}

		// The publication transaction writes a prepared root's edge.
		if prepared {
			v.rootPins[node] = &rootPin{count: 1}
			unlock()
			return owner, release, nil
		}

		// Write the reader edge. Concurrent pins of this root wait for it.
		pin = &rootPin{count: 1, settled: make(chan struct{})}
		v.rootPins[node] = pin
		unlock()
		err = v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) error {
			// Require the reader root to exist before retaining its graph edge.
			found, err := blocks.GetBlockExists(ctx, ref)
			if err != nil {
				return err
			}
			if !found {
				return block.ErrNotFound
			}

			// Retain the reader root under the process lease in the same transaction.
			return rg.ApplyRefBatch(ctx, []block_gc.RefEdge{
				{Subject: block_gc.NodeGCRoot, Object: owner},
				{Subject: owner, Object: node},
			}, nil)
		})
		v.settleRootPin(node, pin, err != nil)
		if err != nil {
			return "", nil, err
		}
		return owner, release, nil
	}
}

// rootPinOwnerLocked returns the process owner node of reader pins and stages,
// acquiring its lease on first use. The caller holds rootPinMu.
func (v *Volume) rootPinOwnerLocked(ctx context.Context) (string, error) {
	// Refuse new owners after the pins close.
	if v.rootPinsClosed {
		return "", block.ErrPublicationClosed
	}
	if v.rootPinLease != nil {
		return v.rootPinOwner, nil
	}

	// Hold the owner's lease for the life of the process, so a crash lets
	// ReapRootPins release everything the owner holds.
	owner := rootPinPrefix + ulid.NewULID()
	lease, err := v.WaitAcquireWriteLease(ctx, v.rootPinScope(owner))
	if err != nil {
		return "", err
	}
	v.rootPinOwner, v.rootPinLease = owner, lease
	v.rootPins = make(map[string]*rootPin)
	return owner, nil
}

// unpinRoot releases one reader. The last release leaves the durable edge for
// the next changing direct transaction to remove, so it costs no write barrier
// of its own.
func (v *Volume) unpinRoot(node string) {
	unlock, _ := v.rootPinMu.Lock(context.Background())
	defer unlock()
	if pin := v.rootPins[node]; !v.rootPinsClosed && pin != nil && pin.count != 0 {
		pin.count--
	}
}

// settleRootPin finishes pin's edge write and wakes pins waiting on it. A
// failed add or a completed removal forgets the root.
func (v *Volume) settleRootPin(node string, pin *rootPin, forget bool) {
	// Finish the root edge transition under the Volume pin lock.
	unlock, _ := v.rootPinMu.Lock(context.Background())
	defer unlock()
	if forget && v.rootPins[node] == pin {
		delete(v.rootPins, node)
	}
	close(pin.settled)
	pin.settled = nil
}

// deferredEdits are the deferred edits one direct transaction carries.
type deferredEdits struct {
	// unpins are the released pins whose edge removals the transaction carries,
	// keyed by root node.
	unpins map[string]*rootPin
	// proofs are the root nodes of the pending proofs the transaction carries.
	proofs []string
}

// applyDeferred writes the removals of released reader pins and the pending
// root proofs into a direct transaction. A proof whose root is gone is
// dropped. The caller settles the returned edits once the transaction ends,
// also on error.
func (v *Volume) applyDeferred(ctx context.Context, blocks block.StoreOps, rg *block_gc.RefGraph) (deferredEdits, error) {
	// Reserve the edge removals of released reader pins.
	var edits deferredEdits
	var adds, removes []block_gc.RefEdge
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		return edits, err
	}
	for node, pin := range v.rootPins {
		if pin.count != 0 || pin.settled != nil {
			continue
		}
		if edits.unpins == nil {
			edits.unpins = make(map[string]*rootPin)
		}
		pin.settled = make(chan struct{})
		edits.unpins[node] = pin
		removes = append(removes, block_gc.RefEdge{Subject: v.rootPinOwner, Object: node})
	}
	unlock()

	// Take the pending root proofs.
	v.proofMu.Lock()
	proofs := maps.Clone(v.rootProofs)
	v.proofMu.Unlock()

	// Add the proof of each root still stored.
	for node, root := range proofs {
		edits.proofs = append(edits.proofs, node)
		found, err := blocks.GetBlockExists(ctx, root)
		if err != nil {
			return edits, err
		}
		if found {
			adds = append(adds, block_gc.RefEdge{Subject: node, Object: completeWorldNode})
		}
	}
	if len(adds) == 0 && len(removes) == 0 {
		return edits, nil
	}

	// Apply the edits together, marking roots left without an owner.
	return edits, rg.ApplyRefBatch(ctx, adds, removes)
}

// settleDeferred finishes a transaction's deferred edits. A committed
// transaction forgets its released pins and pending proofs; otherwise the
// next changing transaction carries them again.
func (v *Volume) settleDeferred(edits deferredEdits, committed bool) {
	// Settle the released pins, forgetting them only after a commit.
	for node, pin := range edits.unpins {
		v.settleRootPin(node, pin, committed)
	}

	// Forget the proofs a committed transaction wrote.
	if !committed || len(edits.proofs) == 0 {
		return
	}
	v.proofMu.Lock()
	defer v.proofMu.Unlock()
	for _, node := range edits.proofs {
		delete(v.rootProofs, node)
	}
}

// closeRootPins stops new reader pins, then releases this process's reader
// edges, stages and lease.
func (v *Volume) closeRootPins() error {
	// Stop new reader pins and take the owner under the Volume pin lock.
	ctx := context.Background()
	unlock, _ := v.rootPinMu.Lock(ctx)
	v.rootPinsClosed = true
	owner, lease := v.rootPinOwner, v.rootPinLease
	clear(v.rootPins)
	unlock()
	if lease == nil {
		return nil
	}

	// Release the process reader edges and their coordination lease.
	err := v.releaseRootPin(ctx, owner)
	leaseErr := lease.Release(ctx)
	if err != nil {
		return err
	}
	return leaseErr
}

func (v *Volume) rootPinScope(owner string) coord.Scope {
	return coord.Scope{VolumeID: v.GetID(), Key: owner}
}

func (v *Volume) releaseRootPin(ctx context.Context, owner string) error {
	return v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) error {
		if _, err := rg.RemoveNodeRefs(ctx, owner, true); err != nil {
			return err
		}
		return rg.RemoveRef(ctx, block_gc.NodeGCRoot, owner)
	})
}

// ReapRootPins drops only pins whose coordination lease is no longer held.
// It scans root owners, not the block inventory or retained history. It first
// writes this process's deferred edits, so a collection that follows sees its
// released pins.
func (v *Volume) ReapRootPins(ctx context.Context) error {
	// Skip reader-pin recovery outside the atomic publication domain.
	if !v.SupportsAtomicPublication() {
		return nil
	}

	// Write the released pins and pending proofs of this process.
	if err := v.withDirectAtomic(ctx, nil); err != nil {
		return err
	}

	// Read retained root owners from the Volume reference graph.
	roots, err := v.refGraph.GetOutgoingRefs(ctx, block_gc.NodeGCRoot)
	if err != nil {
		return err
	}

	// Remove reader owners whose process leases can be acquired.
	for _, owner := range roots {
		if !strings.HasPrefix(owner, rootPinPrefix) {
			continue
		}
		lease, acquired, err := v.TryAcquireWriteLease(ctx, v.rootPinScope(owner))
		if err != nil {
			return err
		}
		if !acquired {
			continue
		}
		err = v.releaseRootPin(ctx, owner)
		leaseErr := lease.Release(context.WithoutCancel(ctx))
		if err != nil {
			return err
		}
		if leaseErr != nil {
			return leaseErr
		}
	}
	return nil
}
