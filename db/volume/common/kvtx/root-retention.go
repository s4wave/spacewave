package kvtx

import (
	"context"
	"encoding/base64"
	"fmt"
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

// RootOwnerIRI returns the owner node of the root named name in bucketID.
func RootOwnerIRI(bucketID, name string) string {
	return RootOwnerPrefix + base64.RawURLEncoding.EncodeToString([]byte(bucketID)) + "/" + base64.RawURLEncoding.EncodeToString([]byte(name))
}

// parseRootOwnerIRI returns the bucket and name of a RootOwnerIRI.
func parseRootOwnerIRI(owner string) (bucketID, name string, ok bool) {
	// Split the encoded bucket from the encoded name.
	rest, ok := strings.CutPrefix(owner, RootOwnerPrefix)
	if !ok {
		return "", "", false
	}
	encBucket, encName, ok := strings.Cut(rest, "/")
	if !ok {
		return "", "", false
	}

	// Decode both.
	b, err := base64.RawURLEncoding.DecodeString(encBucket)
	if err != nil {
		return "", "", false
	}
	n, err := base64.RawURLEncoding.DecodeString(encName)
	if err != nil {
		return "", "", false
	}
	return string(b), string(n), true
}

// completeWorldNode is a graph-only proof target. Sweeping a root removes its
// proof edge together with its other immutable dependencies.
const completeWorldNode = "world:complete"

// rootProofCarryLimit bounds the pending proofs one transaction carries. Each
// carried proof costs an existence check and an edge write under the writer
// lock, so a World commit never pays for more than this many.
const rootProofCarryLimit = 256

// rootProofBacklog bounds the pending proofs. A marker that leaves more than
// one batch over it commits batches itself until the backlog fits, so a copy
// that proves blocks faster than other writers commit waits its turn on the
// writer instead of handing the work to their commits. The batch of slack
// keeps a World commit, which marks one root, from paying for the copy.
const rootProofBacklog = 4 * rootProofCarryLimit

// MarkRootsComplete records completion proofs for stored roots. A proof stays
// pending in memory until a changing direct transaction carries it, so
// marking usually costs no write barrier of its own. Past rootProofBacklog the
// marker carries batches itself. A crash loses only pending proofs, which a
// later retention recomputes.
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
// proofs whose roots are gone for a sweep to drop.
func (v *Volume) carryRootProofBacklog(ctx context.Context) error {
	// Leave carrying to other writers when the store has no atomic publication.
	if !v.SupportsAtomicPublication() {
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
		// Note the sweeps attempted before checking that the roots exist.
		v.proofMu.Lock()
		sweeps := v.sweeps
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
				return fmt.Errorf("root proof: %w", block.NotFoundError(root))
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

// RootsComplete checks each root for a pending proof, then for the proof in
// the volume ownership graph. It reads the durable proofs of all roots in one
// transaction.
func (v *Volume) RootsComplete(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Accept the proofs still waiting for a changing transaction, and collect
	// the roots that need their durable proof read.
	complete := make([]bool, len(refs))
	var durable []int
	v.proofMu.Lock()
	for i, ref := range refs {
		if ref.GetEmpty() {
			continue
		}
		if _, pending := v.rootProofs[block_gc.BlockIRI(ref)]; pending {
			complete[i] = true
		} else {
			durable = append(durable, i)
		}
	}
	v.proofMu.Unlock()
	if len(durable) == 0 {
		return complete, nil
	}

	// Read the durable proof edges in one transaction.
	err := (&transactionRefGraph{volume: v}).read(ctx, func(rg *block_gc.RefGraph) error {
		for _, i := range durable {
			edges, err := rg.GetOutgoingRefs(ctx, block_gc.BlockIRI(refs[i]))
			if err != nil {
				return err
			}
			complete[i] = slices.Contains(edges, completeWorldNode)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return complete, nil
}

// rootCompleteTx checks a root's proof inside a direct transaction. A pending
// proof counts while the transaction still sees its root, since a sweep that
// removed the root may not have dropped the proof yet.
func (v *Volume) rootCompleteTx(ctx context.Context, blocks block.StoreOps, rg *block_gc.RefGraph, ref *block.BlockRef) (bool, error) {
	// Accept a pending proof whose root is stored.
	node := block_gc.BlockIRI(ref)
	v.proofMu.Lock()
	_, pending := v.rootProofs[node]
	v.proofMu.Unlock()
	if pending {
		return blocks.GetBlockExists(ctx, ref)
	}

	// Otherwise read the durable proof edge.
	refs, err := rg.GetOutgoingRefs(ctx, node)
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
// existence checks. Entries return volume.ErrReserve while the volume is in
// the reserve state; moving roots alone writes no new data and continues.
func (v *Volume) SetBucketRoots(ctx context.Context, bucketID string, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	// Require atomic publication and refuse new data below the disk reserve.
	if !v.SupportsAtomicPublication() {
		return block.ErrAtomicPublicationUnsupported
	}
	if len(entries) != 0 {
		if err := v.checkReserve(ctx, batchBytes(entries)); err != nil {
			return err
		}
	}

	// Write the entries and move the roots in one transaction.
	v.noteUnknownRefsBatch(putRoots, entries)
	var existed []bool
	err := v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) error {
		// Write the entries under the bucket's ownership.
		if len(entries) != 0 {
			bucket := block_gc.BucketIRI(bucketID)
			if err := claimBucket(ctx, rg, bucket); err != nil {
				return err
			}
			gc := block_gc.NewGCStoreOpsWithParentAndTraceTask(blocks, rg, bucket, block_gc.BucketFlushTask())
			var err error
			if existed, err = gc.PutBlockBatch(ctx, entries); err != nil {
				return err
			}
			if err := v.addExistedHolds(ctx, blocks, rg, entries, existed); err != nil {
				return err
			}
			if err := gc.FlushPending(ctx); err != nil {
				return err
			}
		}

		// Move each named root.
		for _, root := range roots {
			if err := v.setBucketRoot(ctx, blocks, rg, bucketID, root.Name, root.Ref); err != nil {
				return err
			}
		}
		return nil
	})

	// Count the blocks the committed entries stored, or the refusal.
	if err == nil {
		v.countPutBatch(putRoots, entries, existed)
	}
	v.countRefused(putRoots, err)
	return err
}

// setBucketRoot requires a shared physical transaction for bytes and graph.
func (v *Volume) setBucketRoot(ctx context.Context, blocks block.StoreOps, rg *block_gc.RefGraph, bucketID, name string, ref *block.BlockRef) error {
	// Read the root the owner node holds now.
	owner := RootOwnerIRI(bucketID, name)
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
			return fmt.Errorf("named root %s of bucket %s: %w", name, bucketID, block.NotFoundError(ref))
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
	if err := rg.ApplyRefBatch(ctx, adds, removes); err != nil {
		return err
	}

	// Count the held pages of a deadlist record the first time it is named.
	if !v.holdCounts || ref.GetEmpty() || !strings.HasPrefix(name, block.DeadlistRootPrefix) || slices.Contains(old, next) {
		return nil
	}
	return addDeadlistHolds(ctx, blocks, rg, ref)
}

// addDeadlistHolds adds one shared count to each page the deadlist record at
// ref lists as held. Naming the record is the holder's save, so the count
// lands in the transaction that makes the new holders durable, exactly once
// per record: renaming it due, or naming it again, adds nothing.
func addDeadlistHolds(ctx context.Context, blocks block.StoreOps, rg *block_gc.RefGraph, ref *block.BlockRef) error {
	// Read the record from the transaction that stores it.
	_, dead, err := block.ReadDeadlist(ctx, blocks, ref)
	if err != nil {
		return fmt.Errorf("count held pages of deadlist record: %w", err)
	}

	// Count each held page.
	held := make([]string, len(dead.Held))
	for i, page := range dead.Held {
		held[i] = block_gc.BlockIRI(page)
	}
	return rg.AddHolds(ctx, held)
}

// addExistedHolds adds the shared counts a put batch gives the pages it found
// stored. Each found page gains one holder per new page of the batch that
// names it, and one when no unmarked page of the batch names it: its parent
// is a root or a page stored outside the batch. A found page whose parent the
// batch also found gains nothing, since the stored parent already named it.
//
// A writer marks Counted each entry it can prove the store held before its
// interval began, such as a page of the old tree written back. Such an entry
// gains no count, and it never stands in for the parent of a found entry: a
// found child of a marked entry counts on its own, which can only over-count.
//
// A marked entry the put finds absent was freed after the writer's proof, so
// the put stores it afresh and its edges to its children are new: no record
// counts them, because its writer saw it stored. It counts as a new page of
// the batch, and each other stored child gains one count too, unless the
// batch wrote the child afresh, probing the children outside the batch. The
// writer marks only pages its kept World holds, and the collector frees no
// page a named World reaches, so this path should be unreachable; revivedPuts
// counts each entry it takes, and any is an error in that proof.
//
// It adds nothing on a Volume configured to keep no counts.
//
// Every over-count leaks and never frees: a parent and child split across two
// batches each count, and a payload or span block gains a count no record
// takes, which the sweep deletes with the block.
func (v *Volume) addExistedHolds(ctx context.Context, blocks block.StoreOps, rg *block_gc.RefGraph, entries []*block.PutBatchEntry, existed []bool) error {
	// A Volume that keeps no counts skips the graph reads and writes.
	if !v.holdCounts {
		return nil
	}

	// Note which entries the batch found and which it stored afresh, setting
	// aside marked entries. A page listed twice finds its own first copy, so
	// its first entry decides.
	found := make(map[string]bool, len(entries))
	kept := make(map[string]bool)
	var listed, revived []*block.PutBatchEntry
	for i, e := range entries {
		if e.Tombstone {
			continue
		}
		key := block_gc.BlockIRI(e.Ref)
		if _, ok := found[key]; ok || kept[key] {
			continue
		}
		switch {
		case e.Counted && existed[i]:
			kept[key] = true
		case e.Counted:
			found[key] = false
			listed = append(listed, e)
			revived = append(revived, e)
		default:
			found[key] = existed[i]
			listed = append(listed, e)
		}
	}

	// Count each found entry once per new parent, or once with no parent in
	// the batch.
	parents := make(map[string]int, len(listed))
	var holds []string
	for _, e := range listed {
		named := make(map[string]struct{}, len(e.Refs))
		for _, child := range e.Refs {
			key := block_gc.BlockIRI(child)
			if _, dup := named[key]; dup || !found[key] {
				continue
			}
			named[key] = struct{}{}
			parents[key]++
			if !found[block_gc.BlockIRI(e.Ref)] {
				holds = append(holds, key)
			}
		}
	}
	for key, isFound := range found {
		if isFound && parents[key] == 0 {
			holds = append(holds, key)
		}
	}

	// Count the other stored children of each revived entry: marked entries
	// at once, and children outside the batch after a probe.
	v.revivedPuts.Add(uint64(len(revived)))
	var probe []*block.BlockRef
	for _, e := range revived {
		named := make(map[string]struct{}, len(e.Refs))
		for _, child := range e.Refs {
			key := block_gc.BlockIRI(child)
			if _, dup := named[key]; dup {
				continue
			}
			named[key] = struct{}{}
			switch _, listed := found[key]; {
			case kept[key]:
				holds = append(holds, key)
			case !listed:
				probe = append(probe, child)
			}
		}
	}
	if len(probe) != 0 {
		stored, err := blocks.GetBlockExistsBatch(ctx, probe)
		if err != nil {
			return err
		}
		for i, ok := range stored {
			if ok {
				holds = append(holds, block_gc.BlockIRI(probe[i]))
			}
		}
	}
	return rg.AddHolds(ctx, holds)
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
				return fmt.Errorf("reader root pin: %w", block.NotFoundError(ref))
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
	v.stages = make(map[string]*openStage)
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
	// stages are the released stages whose edge removals the transaction
	// carries.
	stages []string
	// proofs are the root nodes of the pending proofs the transaction carries.
	proofs []string
}

// applyDeferred writes the removals of released reader pins and stages and up
// to rootProofCarryLimit pending root proofs into a direct transaction. A
// proof whose root is gone is dropped. The caller settles the returned edits once the transaction ends,
// also on error.
func (v *Volume) applyDeferred(ctx context.Context, blocks block.StoreOps, rg *block_gc.RefGraph) (deferredEdits, error) {
	// Lock the reader pins and stages.
	var edits deferredEdits
	var adds, removes []block_gc.RefEdge
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		return edits, err
	}

	// Reserve each released pin whose edge no transaction carries yet.
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

	// Remove the edges of released stages.
	edits.stages, removes = v.releasedStagesLocked(removes)
	unlock()

	// Take at most rootProofCarryLimit pending root proofs.
	proofs := make(map[string]*block.BlockRef)
	v.proofMu.Lock()
	for node, root := range v.rootProofs {
		if len(proofs) == rootProofCarryLimit {
			break
		}
		proofs[node] = root
	}
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
// transaction forgets its released pins, stages and pending proofs; otherwise
// the next changing transaction carries them again.
func (v *Volume) settleDeferred(edits deferredEdits, committed bool) {
	// Settle the released pins, forgetting them only after a commit.
	for node, pin := range edits.unpins {
		v.settleRootPin(node, pin, committed)
	}

	// Forget the released stages a committed transaction removed.
	if committed && len(edits.stages) != 0 {
		unlock, _ := v.rootPinMu.Lock(context.Background())
		for _, stage := range edits.stages {
			delete(v.stages, stage)
		}
		unlock()
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
	clear(v.stages)
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
