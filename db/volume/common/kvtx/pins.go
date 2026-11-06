package kvtx

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume"
)

// pinRootPrefix prefixes the reader pins of the Volume's bucket with the empty
// ID. A pin's name is the prefix, this process's ID, a slash and the pinned
// block, so each process holds its own pin of a block under its own lease.
const pinRootPrefix = "pin:"

// stageRootPrefix prefixes the stages of the Volume's bucket with the empty
// ID. A stage holds each block it holds under the stage's name, a slash and
// the block.
const stageRootPrefix = "stage:"

// rootPin counts the in-process readers of one pinned root. A root whose last
// reader leaves keeps its pin at count zero until a changing direct
// transaction carries the release, and a new reader meanwhile reuses it.
type rootPin struct {
	// ref is the pinned block.
	ref *block.BlockRef
	// count is the number of unreleased pins.
	count int
	// settled is non-nil while the pin is being written or released, and
	// closes when that write finishes.
	settled chan struct{}
}

// pinRootName returns the name of this process's pin of ref.
func (v *Volume) pinRootName(ref *block.BlockRef) string {
	return pinRootPrefix + v.procID + "/" + ref.MarshalString()
}

// PinBucketRoot holds ref under a leased root of this process until the
// returned release, so the collector keeps the graph under it after the named
// root that reached it moves. A process that dies releases its pins when their
// lease lapses.
func (v *Volume) PinBucketRoot(ctx context.Context, ref *block.BlockRef) (func(), error) {
	_, release, err := v.pinRoot(ctx, ref, false)
	return release, err
}

// pinRoot reserves an in-process reader of ref and returns the pin's root
// name. The first reader writes the pin; a prepared pin's root is written by
// the publication transaction that names ref instead. rootPinMu guards only
// the counts: pin writes run outside it, so pinning a pinned root never waits
// for another root's transaction.
func (v *Volume) pinRoot(ctx context.Context, ref *block.BlockRef, prepared bool) (string, func(), error) {
	name := v.pinRootName(ref)
	for {
		// Refuse new pins after the Volume closes.
		unlock, err := v.rootPinMu.Lock(ctx)
		if err != nil {
			return "", nil, err
		}
		if v.rootPinsClosed {
			unlock()
			return "", nil, volume.ErrVolumeClosed
		}

		// Wait out a pin write in flight, then observe its result.
		pin := v.rootPins[name]
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
		release := sync.OnceFunc(func() { v.unpinRoot(name) })

		// A pinned root only gains a reader.
		if pin != nil {
			pin.count++
			unlock()
			return name, release, nil
		}

		// The publication transaction writes a prepared root's pin.
		if prepared {
			v.rootPins[name] = &rootPin{ref: ref, count: 1}
			unlock()
			return name, release, nil
		}

		// Write the pin. Concurrent pins of this root wait for it.
		pin = &rootPin{ref: ref, count: 1, settled: make(chan struct{})}
		v.rootPins[name] = pin
		unlock()
		err = v.withDirectAtomic(ctx, func(p *physicalTx) error {
			_, err := v.land(ctx, p, "", nil, []block.NamedRoot{{Name: name, Ref: ref, Expires: v.leaseExpiry()}})
			return err
		})
		v.settleRootPin(name, pin, err != nil)
		if err != nil {
			return "", nil, err
		}
		return name, release, nil
	}
}

// unpinRoot releases one reader. The last release leaves the pin for the next
// changing direct transaction to release, so it costs no write barrier of its
// own.
func (v *Volume) unpinRoot(name string) {
	unlock, _ := v.rootPinMu.Lock(context.Background())
	defer unlock()
	if pin := v.rootPins[name]; !v.rootPinsClosed && pin != nil && pin.count != 0 {
		pin.count--
	}
}

// settleRootPin finishes a pin write and wakes pins waiting on it. A failed
// write or a completed release forgets the pin.
func (v *Volume) settleRootPin(name string, pin *rootPin, forget bool) {
	// Forget a failed or released pin, then wake its waiters.
	unlock, _ := v.rootPinMu.Lock(context.Background())
	defer unlock()
	if forget && v.rootPins[name] == pin {
		delete(v.rootPins, name)
	}
	close(pin.settled)
	pin.settled = nil
}

// deferredEdits are the deferred edits one direct transaction carries.
type deferredEdits struct {
	// unpins are the released pins whose releases the transaction carries,
	// keyed by root name.
	unpins map[string]*rootPin
	// stages are the released stages whose roots the transaction releases.
	stages []string
	// proofs are the block keys of the pending proofs the transaction
	// carries.
	proofs []string
}

// applyDeferred writes into p the releases of released reader pins and stages
// and up to rootProofCarryLimit pending root proofs. A proof whose root is
// gone is dropped. The caller settles the returned edits once the transaction
// ends, also on error.
func (v *Volume) applyDeferred(ctx context.Context, p *physicalTx) (deferredEdits, error) {
	// Reserve each released pin no transaction carries yet, and take the
	// released stages.
	var edits deferredEdits
	var releases []block.NamedRoot
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		return edits, err
	}
	for name, pin := range v.rootPins {
		if pin.count != 0 || pin.settled != nil {
			continue
		}
		if edits.unpins == nil {
			edits.unpins = make(map[string]*rootPin)
		}
		pin.settled = make(chan struct{})
		edits.unpins[name] = pin
		releases = append(releases, block.NamedRoot{Name: name, Previous: pin.ref})
	}
	for stage, st := range v.stages {
		if st.released {
			edits.stages = append(edits.stages, stage)
		}
	}
	unlock()

	// Release every root of each released stage, and each released pin the
	// collector has not released for a lapsed lease.
	releases, err = heldRoots(ctx, p, releases)
	if err != nil {
		return edits, err
	}
	for _, stage := range edits.stages {
		stageRoots, err := listStageRoots(ctx, p, stage)
		if err != nil {
			return edits, err
		}
		releases = append(releases, stageRoots...)
	}
	if len(releases) != 0 {
		if _, err := v.land(ctx, p, "", nil, releases); err != nil {
			return edits, err
		}
	}

	// Write the proofs of at most rootProofCarryLimit pending roots still
	// stored.
	edits.proofs, err = v.carryRootProofs(ctx, p)
	return edits, err
}

// settleDeferred finishes a transaction's deferred edits. A committed
// transaction forgets its released pins, stages and pending proofs; otherwise
// the next changing transaction carries them again.
func (v *Volume) settleDeferred(edits deferredEdits, committed bool) {
	// Settle the released pins, forgetting them only after a commit.
	for name, pin := range edits.unpins {
		v.settleRootPin(name, pin, committed)
	}

	// Forget the released stages a committed transaction released.
	if committed && len(edits.stages) != 0 {
		unlock, _ := v.rootPinMu.Lock(context.Background())
		for _, stage := range edits.stages {
			delete(v.stages, stage)
		}
		unlock()
	}

	// Forget the proofs a committed transaction wrote.
	if committed {
		v.forgetRootProofs(edits.proofs)
	}
}

// closeRootPins stops new reader pins and stages, then releases every pin and
// stage root of this process in one transaction.
func (v *Volume) closeRootPins() error {
	// Stop new pins and stages.
	ctx := context.Background()
	unlock, _ := v.rootPinMu.Lock(ctx)
	v.rootPinsClosed = true

	// Take every pin and stage this process holds.
	var releases []block.NamedRoot
	for name, pin := range v.rootPins {
		releases = append(releases, block.NamedRoot{Name: name, Previous: pin.ref})
	}
	stages := make([]string, 0, len(v.stages))
	for stage := range v.stages {
		stages = append(stages, stage)
	}
	clear(v.rootPins)
	clear(v.stages)
	unlock()
	if (len(releases) == 0 && len(stages) == 0) || !v.HoldsRoots() {
		return nil
	}

	// Release them together. A pin a crash of an earlier write left unwritten
	// releases nothing.
	return v.withDirectAtomic(ctx, func(p *physicalTx) error {
		// Gather the stages' roots.
		for _, stage := range stages {
			stageRoots, err := listStageRoots(ctx, p, stage)
			if err != nil {
				return err
			}
			releases = append(releases, stageRoots...)
		}

		// Release the roots still held.
		held, err := heldRoots(ctx, p, releases)
		if err != nil {
			return err
		}
		_, err = v.land(ctx, p, "", nil, held)
		return err
	})
}

// heldRoots returns the releases of roots of the bucket with the empty ID
// that still hold their Previous in p, leaving out those the collector
// released for a lapsed lease, or that a crash left unwritten.
func heldRoots(ctx context.Context, p *physicalTx, releases []block.NamedRoot) ([]block.NamedRoot, error) {
	held := releases[:0]
	for _, root := range releases {
		ref, err := heldRoot(ctx, p, "", root.Name)
		if err != nil {
			return nil, err
		}
		if !ref.GetEmpty() && ref.EqualsRef(root.Previous) {
			held = append(held, root)
		}
	}
	return held, nil
}
