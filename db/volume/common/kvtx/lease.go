package kvtx

import (
	"context"
	"errors"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// rootLease is how long a reader pin or a stage root of this process lives
// without renewal. The collector releases a leased root whose expiry passed,
// so a process that dies releases its pins and stages by silence. It must be
// shorter than the collector's grace period, so a reader whose pin lapsed
// while its process stalled still finds the blocks it pinned.
const rootLease = 5 * time.Minute

// rootRenewInterval is how often the Volume renews the leased roots this
// process holds, a fifth of the lease, so four renewals can fail before a
// root lapses.
const rootRenewInterval = rootLease / 5

// leaseExpiry returns the expiry a leased root written now carries.
func (v *Volume) leaseExpiry() time.Time {
	return v.now().Add(rootLease)
}

// rootRenewForeground is the age of the last renewal past which the next
// renewal runs as a foreground write, half the lease, so constant foreground
// writes delay renewals but never starve one until its lease lapses.
const rootRenewForeground = rootLease / 2

// runRootRenewal renews the leased roots this process holds every
// rootRenewInterval until ctx ends. A renewal is one changing transaction,
// which also carries the deferred releases. It runs as background work, so it
// waits behind foreground writers, and gives up after rootRenewInterval; once
// the last renewal is older than rootRenewForeground it runs in the
// foreground instead. A foreground writer waits at most for one renewal.
func (v *Volume) runRootRenewal(ctx context.Context) {
	// Tick every rootRenewInterval, counting the lease from the start.
	defer close(v.renewDone)
	ticker := time.NewTicker(rootRenewInterval)
	defer ticker.Stop()
	renewed := v.now()
	for {
		// Wait for the next renewal.
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		// Renew behind the foreground writers while the lease has time left.
		renewCtx := block.WithBackground(ctx)
		if v.now().Sub(renewed) < rootRenewForeground {
			renewCtx = backgroundPass(ctx)
		}
		renewCtx, cancel := context.WithTimeout(renewCtx, rootRenewInterval)
		err := v.renewRoots(renewCtx)
		cancel()
		switch {
		case err == nil:
			renewed = v.now()
		case ctx.Err() == nil && !errors.Is(err, context.DeadlineExceeded) && v.le != nil:
			v.le.WithError(err).Warn("renewing reader pins and stages failed")
		}
	}
}

// renewRoots moves the expiry of every reader pin and stage root this process
// holds a lease forward, in one transaction. A pin or stage root a collector
// released for a lapsed lease while this process still holds it, after a
// stall longer than the lease, is held again while its block is stored. A
// stage whose released root's block is gone is lost, as Journal.tla's
// StageRenew fails it: its later writes would name a freed block.
func (v *Volume) renewRoots(ctx context.Context) error {
	// Collect the pins with readers and the open stages' roots, noting the
	// stage of each stage root.
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		return err
	}
	var pins []block.NamedRoot
	for name, pin := range v.rootPins {
		if pin.count != 0 && pin.settled == nil {
			pins = append(pins, block.NamedRoot{Name: name, Ref: pin.ref, Previous: pin.ref})
		}
	}
	stageOf := make(map[string]string)
	for stage, st := range v.stages {
		if st.released || st.lost {
			continue
		}
		for name, ref := range st.roots {
			pins = append(pins, block.NamedRoot{Name: name, Ref: ref, Previous: ref})
			stageOf[name] = stage
		}
	}
	unlock()
	if len(pins) == 0 {
		return nil
	}

	// Write the later expiry of each pin and stage root. One a collector
	// released is held again while its block is stored, and dropped
	// otherwise: its holder stalled past the lease and the grace period.
	expires := v.leaseExpiry()
	var lost []string
	err = v.withDirectAtomic(ctx, func(p *physicalTx) error {
		// Keep only the roots this process still holds: a reader may have
		// released its pin, or a stage may have moved or released a root,
		// since the snapshot.
		current, err := v.stillHeld(ctx, pins, stageOf)
		if err != nil {
			return err
		}

		// Renew each root, or hold a released one again.
		var renew []block.NamedRoot
		lost = lost[:0]
		for _, root := range current {
			held, err := heldRoot(ctx, p, "", root.Name)
			if err != nil {
				return err
			}
			switch {
			case block_gc.SameRef(held, root.Ref):
			case !held.GetEmpty():
				continue
			default:
				listed, err := p.rec.HasChildren(ctx, root.Ref)
				if err != nil {
					return err
				}
				if !listed {
					if stage, ok := stageOf[root.Name]; ok {
						lost = append(lost, stage)
					}
					continue
				}
				root.Previous = nil
			}
			root.Expires = expires
			renew = append(renew, root)
		}

		// Write them all with the later expiry.
		_, err = v.land(ctx, p, "", nil, renew)
		return err
	})

	// Fail the stages that lost a root with its block.
	if err == nil {
		for _, stage := range lost {
			v.loseStage(stage)
		}
	}
	return err
}

// stillHeld returns the roots among roots this process still holds, read
// under the pin lock inside the renewal's transaction: a pin with a reader
// on the same block, or a root an open stage's record holds on the same
// block. stageOf names the stage of each stage root.
func (v *Volume) stillHeld(ctx context.Context, roots []block.NamedRoot, stageOf map[string]string) ([]block.NamedRoot, error) {
	// Read the pins and stages under the pin lock.
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer unlock()

	// Keep each root its pin or stage still holds on the same block.
	var out []block.NamedRoot
	for _, root := range roots {
		var ref *block.BlockRef
		if stage, ok := stageOf[root.Name]; ok {
			if st := v.stages[stage]; st != nil && !st.released && !st.lost {
				ref = st.roots[root.Name]
			}
		} else if pin := v.rootPins[root.Name]; pin != nil && pin.count != 0 && pin.settled == nil {
			ref = pin.ref
		}
		if block_gc.SameRef(ref, root.Ref) && !ref.GetEmpty() {
			out = append(out, root)
		}
	}
	return out, nil
}

// unixMilliTime returns the time ms Unix milliseconds name, or the zero time
// for zero.
func unixMilliTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// isRootMoved reports whether err is block.ErrRootMoved.
func isRootMoved(err error) bool {
	return errors.Is(err, block.ErrRootMoved)
}
