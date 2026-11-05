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

// runRootRenewal renews the leased roots this process holds every
// rootRenewInterval until ctx ends. A renewal is one changing transaction,
// which also carries the deferred releases.
func (v *Volume) runRootRenewal(ctx context.Context) {
	defer close(v.renewDone)
	ticker := time.NewTicker(rootRenewInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if err := v.renewRoots(block.WithBackground(ctx)); err != nil && ctx.Err() == nil && v.le != nil {
			v.le.WithError(err).Warn("renewing reader pins and stages failed")
		}
	}
}

// renewRoots moves the expiry of every reader pin and stage root this process
// holds a lease forward, in one transaction.
func (v *Volume) renewRoots(ctx context.Context) error {
	// Collect the pins with readers and the open stages.
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
	var stages []string
	for stage, st := range v.stages {
		if !st.released {
			stages = append(stages, stage)
		}
	}
	unlock()
	if len(pins) == 0 && len(stages) == 0 {
		return nil
	}

	// Write the later expiry of each pin and stage root. A pin the collector
	// released for a lapsed lease is held again while its block is stored, and
	// dropped otherwise: its reader stalled past the lease and the grace
	// period.
	expires := v.leaseExpiry()
	return v.withDirectAtomic(ctx, func(p *physicalTx) error {
		// Renew each pin, or hold a lapsed one again.
		var renew []block.NamedRoot
		for _, pin := range pins {
			held, err := heldRoot(ctx, p, "", pin.Name)
			if err != nil {
				return err
			}
			if held.GetEmpty() {
				listed, err := p.rec.HasChildren(ctx, pin.Ref)
				if err != nil {
					return err
				}
				if !listed {
					continue
				}
				pin.Previous = nil
			}
			renew = append(renew, pin)
		}

		// Renew each open stage's roots.
		for _, stage := range stages {
			err := p.rec.ListRoots(ctx, "", stage+"/", func(name string, entry *block_gc.RootEntry) error {
				ref, _, err := block_gc.ParseBlockKey(entry.GetRef())
				if err != nil {
					return err
				}
				renew = append(renew, block.NamedRoot{Name: name, Ref: ref, Previous: ref})
				return nil
			})
			if err != nil {
				return err
			}
		}

		// Write them all with the later expiry.
		for i := range renew {
			renew[i].Expires = expires
		}
		_, err := v.land(ctx, p, "", nil, renew)
		return err
	})
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
