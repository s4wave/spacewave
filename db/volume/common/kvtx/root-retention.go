package kvtx

import (
	"context"
	"encoding/base64"
	"slices"
	"strings"
	"sync"

	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/coord"
)

const rootPinPrefix = "reader:"

// completeWorldNode is a graph-only proof target. Sweeping a root removes its
// proof edge together with its other immutable dependencies.
const completeWorldNode = "world:complete"

// completionNode separates World proofs from copy decoder domains.
func completionNode(domain string) string {
	if domain == "" {
		return completeWorldNode
	}
	return "copy:complete:" + base64.RawURLEncoding.EncodeToString([]byte(domain))
}

// MarkRootsComplete persists a copy's proof batch without per-block commits.
func (v *Volume) MarkRootsComplete(ctx context.Context, proofs []block.RootProof) error {
	return v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) (bool, error) {
		var adds []block_gc.RefEdge
		for _, proof := range proofs {
			if proof.Ref.GetEmpty() {
				continue
			}
			found, err := blocks.GetBlockExists(ctx, proof.Ref)
			if err != nil {
				return false, err
			}
			if !found {
				return false, block.ErrNotFound
			}
			node := block_gc.BlockIRI(proof.Ref)
			refs, err := rg.GetOutgoingRefs(ctx, node)
			if err != nil {
				return false, err
			}
			marker := completionNode(proof.Domain)
			if !slices.Contains(refs, marker) {
				adds = append(adds, block_gc.RefEdge{Subject: node, Object: marker})
			}
		}
		if len(adds) == 0 {
			return false, nil
		}
		err := rg.ApplyRefBatch(ctx, adds, nil)
		return err == nil, err
	})
}

// RootComplete checks the proof in the volume ownership graph.
func (v *Volume) RootComplete(ctx context.Context, ref *block.BlockRef, domain ...string) (bool, error) {
	refs, err := v.refGraph.GetOutgoingRefs(ctx, block_gc.BlockIRI(ref))
	var name string
	if len(domain) != 0 {
		name = domain[0]
	}
	return slices.Contains(refs, completionNode(name)), err
}

// SetBucketRoot replaces one durable root within a bucket. Ownership changes
// share a physical transaction with sweep eligibility and existence checks.
func (v *Volume) SetBucketRoot(ctx context.Context, bucketID, name string, ref *block.BlockRef) error {
	return v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) (bool, error) {
		err := setBucketRoot(ctx, blocks, rg, bucketID, name, ref)
		return err == nil, err
	})
}

// setBucketRoot requires a shared physical transaction for bytes and graph.
func setBucketRoot(ctx context.Context, blocks block.StoreOps, rg *block_gc.RefGraph, bucketID, name string, ref *block.BlockRef) error {
	owner := "head:" + base64.RawURLEncoding.EncodeToString([]byte(bucketID)) + "/" + base64.RawURLEncoding.EncodeToString([]byte(name))
	bucket := block_gc.BucketIRI(bucketID)
	old, err := rg.GetOutgoingRefs(ctx, owner)
	if err != nil {
		return err
	}
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
	for _, previous := range old {
		if previous != next {
			removes = append(removes, block_gc.RefEdge{Subject: owner, Object: previous})
		}
	}
	return rg.ApplyRefBatch(ctx, adds, removes)
}

// PinBucketRoot couples a durable reader edge to the volume's existing
// cross-process lease. A process crash releases the lease; the next sweep
// removes its abandoned edge without disturbing another process's readers.
func (v *Volume) PinBucketRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	_, release, err := v.pinRoot(ctx, ref, false)
	return release, err
}

// rootPin counts the in-process readers of one root.
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
		if v.rootPinsClosed {
			unlock()
			return "", nil, block.ErrPublicationClosed
		}
		if v.rootPinLease == nil {
			owner := rootPinPrefix + ulid.NewULID()
			lease, err := v.WaitAcquireWriteLease(ctx, v.rootPinScope(owner))
			if err != nil {
				unlock()
				return "", nil, err
			}
			v.rootPinOwner, v.rootPinLease = owner, lease
			v.rootPins = make(map[string]*rootPin)
		}
		owner := v.rootPinOwner

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
		err = v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) (bool, error) {
			found, err := blocks.GetBlockExists(ctx, ref)
			if err != nil {
				return false, err
			}
			if !found {
				return false, block.ErrNotFound
			}
			err = rg.ApplyRefBatch(ctx, []block_gc.RefEdge{{Subject: block_gc.NodeGCRoot, Object: owner}, {Subject: owner, Object: node}}, nil)
			return err == nil, err
		})
		v.settleRootPin(node, pin, err != nil)
		if err != nil {
			return "", nil, err
		}
		return owner, release, nil
	}
}

// unpinRoot releases one reader and removes the durable edge after the last.
func (v *Volume) unpinRoot(node string) {
	ctx := context.Background()
	unlock, _ := v.rootPinMu.Lock(ctx)
	pin := v.rootPins[node]
	if v.rootPinsClosed || pin == nil || pin.count == 0 {
		unlock()
		return
	}
	pin.count--
	if pin.count != 0 {
		unlock()
		return
	}
	pin.settled = make(chan struct{})
	owner := v.rootPinOwner
	unlock()

	// Failed cleanup remains retained until this volume closes or its lease is reaped.
	_ = v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) (bool, error) {
		err := rg.ApplyRefBatch(ctx, nil, []block_gc.RefEdge{{Subject: owner, Object: node}})
		return err == nil, err
	})
	v.settleRootPin(node, pin, true)
}

// settleRootPin finishes pin's edge write and wakes pins waiting on it. A
// failed add or a completed removal forgets the root.
func (v *Volume) settleRootPin(node string, pin *rootPin, forget bool) {
	unlock, _ := v.rootPinMu.Lock(context.Background())
	defer unlock()
	if forget && v.rootPins[node] == pin {
		delete(v.rootPins, node)
	}
	close(pin.settled)
	pin.settled = nil
}

func (v *Volume) closeRootPins() error {
	ctx := context.Background()
	unlock, _ := v.rootPinMu.Lock(ctx)
	defer unlock()
	v.rootPinsClosed = true
	if v.rootPinLease == nil {
		return nil
	}
	err := v.releaseRootPin(ctx, v.rootPinOwner)
	leaseErr := v.rootPinLease.Release(ctx)
	clear(v.rootPins)
	if err != nil {
		return err
	}
	return leaseErr
}

func (v *Volume) rootPinScope(owner string) coord.Scope {
	return coord.Scope{VolumeID: v.GetID(), Key: owner}
}

func (v *Volume) releaseRootPin(ctx context.Context, owner string) error {
	return v.withDirectAtomic(ctx, func(_ block.StoreOps, rg *block_gc.RefGraph) (bool, error) {
		if _, err := rg.RemoveNodeRefs(ctx, owner, true); err != nil {
			return false, err
		}
		err := rg.RemoveRef(ctx, block_gc.NodeGCRoot, owner)
		return err == nil, err
	})
}

// ReapRootPins drops only pins whose coordination lease is no longer held.
// It scans root owners, not the block inventory or retained history.
func (v *Volume) ReapRootPins(ctx context.Context) error {
	if !v.SupportsAtomicPublication() {
		return nil
	}
	roots, err := v.refGraph.GetOutgoingRefs(ctx, block_gc.NodeGCRoot)
	if err != nil {
		return err
	}
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
