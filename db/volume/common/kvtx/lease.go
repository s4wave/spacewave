package kvtx

import (
	"context"
	"errors"
	"maps"
	"math"
	"slices"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// This process holds its reader pins and stage roots under one lease, as
// block_gc's leases.go describes, and renews the lease, not each root. The
// Volume keeps these invariants on top of the records' own:
//
//  1. Only renewRoots starts the process lease, and it moves every root this
//     process holds onto the new lease in the transaction that starts it: the
//     roots the store indexes under the old lease and the roots its pins and
//     stages hold in memory. A writer that started a lease itself would leave
//     the roots of the lapsed one for the collector, and the next renewal,
//     finding the new lease live, would not rescue them.
//  2. A writer moves a root onto the process lease only while at least
//     leaseMargin of it is left, renewing first otherwise, so a root it
//     writes outlives the moment its pin or stage records the root in memory,
//     where the next renewal finds it.
//  3. A root the collector released while this process stalled past the
//     lease is held again while its block is stored with holders; otherwise
//     its stage is lost, since a block at zero may have released its
//     children already.
//  4. One renewRoots runs at a time, from its snapshot of the held roots
//     through its commit to publishing the lease. Each reads the published
//     lease in its transaction and renews it while it is live, so the
//     process holds its roots under one lease: a second lease started
//     before the first was published would leave the first's roots to lapse
//     while this process still holds them, and the collector would release
//     them under the live published lease, where no rehold rescues them.

// rootLease is how long the process lease lives without renewal. The
// collector releases every root of a lapsed lease, so a process that dies
// releases its pins and stages by silence. It must be shorter than the
// collector's grace period, so a reader whose pin lapsed while its process
// stalled still finds the blocks it pinned.
const rootLease = 5 * time.Minute

// rootRenewInterval is how often the Volume renews the process lease, a fifth
// of the lease, so four renewals can fail before it lapses.
const rootRenewInterval = rootLease / 5

// rootRenewForeground is the age of the last renewal past which the next
// renewal runs as a foreground write, half the lease, so writer admission
// stops deferring it while the expiry approaches. This sets admission policy;
// caller bodies, storage and scheduling still have no finite time bound.
const rootRenewForeground = rootLease / 2

// leaseMargin is the part of the process lease that must be left for a
// writer to move a root onto it, as invariant 2 requires.
const leaseMargin = rootLease / 2

// errLeaseShort is returned inside a writer's transaction when less than
// leaseMargin of the process lease is left, or there is none yet.
var errLeaseShort = errors.New("process lease is short")

// runRootRenewal renews the process lease every rootRenewInterval until ctx
// ends. A renewal is one changing transaction, which also carries the
// deferred releases. It uses background admission and gives up after
// rootRenewInterval. Once the last renewal is older than rootRenewForeground
// it runs in the foreground. That promotion retains the lifecycle context and
// overrides any inherited background marker.
func (v *Volume) runRootRenewal(ctx context.Context) {
	// Tick every rootRenewInterval. renewed is the base of the last renewal's
	// expiry, or, until one lands, the start, one interval before the first
	// tick. The loop reads the clock only after a tick, since the clock may
	// be set after the Volume opens.
	defer close(v.renewDone)
	ticker := time.NewTicker(rootRenewInterval) //gostyle:timer deadline
	defer ticker.Stop()
	var renewed time.Time
	for {
		// Wait for the next renewal.
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		if renewed.IsZero() {
			renewed = v.now().Add(-rootRenewInterval)
		}

		// Defer renewal as background work only while the lease has time left.
		renewCtx := block.WithReadClass(ctx, block.ReadInteractive)
		if v.now().Sub(renewed) < rootRenewForeground {
			renewCtx = block.WithBackground(ctx)
		}
		renewCtx, cancel := context.WithTimeout(renewCtx, rootRenewInterval)
		base, err := v.renewRoots(renewCtx, false)
		cancel()
		switch {
		case err == nil:
			renewed = base
		case ctx.Err() == nil && !errors.Is(err, context.DeadlineExceeded) && v.le != nil:
			v.le.WithError(err).Warn("renewing the process lease failed")
		}
	}
}

// renewRoots moves the expiry of the process lease forward while it is live,
// one read and one write however many roots it holds. Otherwise, after a
// stall longer than the lease, or for the first lease, it starts a new lease
// and moves every root this process holds onto it, as invariants 1 and 3
// describe. With no pin or stage root held and start unset it writes
// nothing, and the lease may lapse. It returns the time the written lease
// counts from, which precedes the write, or now when it wrote nothing.
//
// Renewals run one at a time, as invariant 4 requires: two callers that both
// find no live lease, such as two stages opening together or the ticker and
// a writer after a stall, start one lease, and the second renews it.
func (v *Volume) renewRoots(ctx context.Context, start bool) (time.Time, error) {
	// Wait for any other renewal to publish its lease.
	unlock, err := v.lease.renew.Lock(ctx)
	if err != nil {
		return time.Time{}, err
	}
	defer unlock()

	// Collect the pins with readers and the open stages' roots.
	pins, stageOf, err := v.heldRoots(ctx)
	if err != nil {
		return time.Time{}, err
	}
	if len(pins) == 0 && !start {
		return v.now(), nil
	}

	// Renew the live lease, or start a new one holding every root.
	base := v.now()
	expires := base.Add(rootLease).UnixMilli()
	var id string
	var lost []string
	err = v.withDirectAtomic(ctx, func(p *physicalTx) error {
		// Renew a live lease.
		old := v.processLeaseID()
		if old != "" {
			renewed, err := p.rec.RenewLease(ctx, old, base.UnixMilli(), expires)
			if err != nil || renewed {
				id = old
				return err
			}
		}

		// Start a new one.
		id = ulid.NewULID()
		if err := p.rec.StartLease(ctx, id, expires); err != nil {
			return err
		}

		// Move every root this process holds onto it.
		current, err := v.stillHeld(ctx, pins, stageOf)
		if err != nil {
			return err
		}
		var moves []block.NamedRoot
		moves, lost, err = rehold(ctx, p, old, id, current, stageOf)
		if err != nil {
			return err
		}
		_, err = v.land(ctx, p, "", nil, moves)
		return err
	})
	if err != nil {
		return time.Time{}, err
	}

	// Use the lease from now on, and fail the stages that lost a root with
	// its block.
	v.lease.mu.Lock()
	v.lease.id = id
	v.lease.mu.Unlock()
	for _, stage := range lost {
		v.loseStage(stage)
	}
	return base, nil
}

// processLeaseID returns the ID of the process lease, empty before the first.
func (v *Volume) processLeaseID() string {
	v.lease.mu.Lock()
	defer v.lease.mu.Unlock()
	return v.lease.id
}

// withProcessLease runs fn in one direct atomic transaction with the ID of
// the process lease, which has at least leaseMargin left. When it has less,
// or there is none yet, it renews or starts the lease first and runs fn
// again, so fn may run twice.
func (v *Volume) withProcessLease(ctx context.Context, fn func(p *physicalTx, lease string) error) error {
	for renewed := false; ; renewed = true {
		// Run fn under the lease when enough of it is left.
		err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
			// Refuse a missing lease or one with less than the margin left.
			id := v.processLeaseID()
			if id == "" {
				return errLeaseShort
			}
			expires, _, err := p.rec.GetLease(ctx, id)
			if err != nil {
				return err
			}
			if expires-v.now().UnixMilli() < leaseMargin.Milliseconds() {
				return errLeaseShort
			}
			return fn(p, id)
		})
		if renewed || !errors.Is(err, errLeaseShort) {
			return err
		}

		// Renew or start the lease, then run fn again.
		if _, err := v.renewRoots(ctx, true); err != nil {
			return err
		}
	}
}

// heldRoots returns the roots this process holds in memory: each pin with
// readers and each root of an open stage, with the stage of each stage root.
func (v *Volume) heldRoots(ctx context.Context) ([]block.NamedRoot, map[string]string, error) {
	// Collect the pins with readers and the open stages.
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		return nil, nil, err
	}
	var roots []block.NamedRoot
	for name, pin := range v.rootPins {
		if pin.count != 0 && pin.settled == nil {
			roots = append(roots, block.NamedRoot{Name: name, Ref: pin.ref, Previous: pin.ref})
		}
	}
	open := make(map[string]*openStage)
	for stage, st := range v.stages {
		if !st.released && !st.lost {
			open[stage] = st
		}
	}
	unlock()

	// Read each open stage's roots under its own lock.
	stageOf := make(map[string]string)
	for stage, st := range open {
		st.mu.Lock()
		for name, ref := range st.roots {
			roots = append(roots, block.NamedRoot{Name: name, Ref: ref, Previous: ref})
			stageOf[name] = stage
		}
		st.mu.Unlock()
	}
	return roots, stageOf, nil
}

// rehold returns the moves that hold every root this process holds under the
// new lease id, and the stages lost: each root the store indexes under the
// old lease, and each of current, which this process holds in memory. A root
// of current that the collector released is held again while its block is
// stored with holders, and its stage is lost otherwise. A root moved since
// the snapshot is left alone.
func rehold(
	ctx context.Context,
	p *physicalTx,
	old, id string,
	current []block.NamedRoot,
	stageOf map[string]string,
) ([]block.NamedRoot, []string, error) {
	// Move each root the store holds under the old lease.
	moves := make(map[string]block.NamedRoot)
	if old != "" {
		indexed, err := p.rec.ListLeaseRoots(ctx, old, math.MaxInt)
		if err != nil {
			return nil, nil, err
		}
		for _, root := range indexed {
			if root.BucketID != "" {
				continue
			}
			ref, err := heldRoot(ctx, p, "", root.Name)
			if err != nil {
				return nil, nil, err
			}
			if !ref.GetEmpty() {
				moves[root.Name] = block.NamedRoot{Name: root.Name, Ref: ref, Previous: ref, Lease: id}
			}
		}
	}

	// Move each root held in memory, or hold a released one again.
	var lost []string
	for _, root := range current {
		if _, ok := moves[root.Name]; ok {
			continue
		}
		held, err := heldRoot(ctx, p, "", root.Name)
		if err != nil {
			return nil, nil, err
		}
		switch {
		case block_gc.SameRef(held, root.Ref):
		case !held.GetEmpty():
			continue
		default:
			stored, err := p.rec.Held(ctx, root.Ref)
			if err != nil {
				return nil, nil, err
			}
			if !stored {
				if stage, ok := stageOf[root.Name]; ok {
					lost = append(lost, stage)
				}
				continue
			}
			root.Previous = nil
		}
		root.Lease = id
		moves[root.Name] = root
	}

	// Return the moves in name order.
	out := make([]block.NamedRoot, 0, len(moves))
	for _, name := range slices.Sorted(maps.Keys(moves)) {
		out = append(out, moves[name])
	}
	return out, lost, nil
}

// stillHeld returns the roots among roots this process still holds, read
// under the pin lock inside the renewal's transaction: a pin with a reader
// on the same block, or a root an open stage's record holds on the same
// block. stageOf names the stage of each stage root.
func (v *Volume) stillHeld(ctx context.Context, roots []block.NamedRoot, stageOf map[string]string) ([]block.NamedRoot, error) {
	// Read the pins and find the open stages under the pin lock.
	unlock, err := v.rootPinMu.Lock(ctx)
	if err != nil {
		return nil, err
	}
	refs := make([]*block.BlockRef, len(roots))
	stages := make([]*openStage, len(roots))
	for i, root := range roots {
		if stage, ok := stageOf[root.Name]; ok {
			if st := v.stages[stage]; st != nil && !st.released && !st.lost {
				stages[i] = st
			}
		} else if pin := v.rootPins[root.Name]; pin != nil && pin.count != 0 && pin.settled == nil {
			refs[i] = pin.ref
		}
	}
	unlock()

	// Read each stage root under its stage's lock, and keep each root its
	// pin or stage still holds on the same block.
	var out []block.NamedRoot
	for i, root := range roots {
		if st := stages[i]; st != nil {
			st.mu.Lock()
			refs[i] = st.roots[root.Name]
			st.mu.Unlock()
		}
		if block_gc.SameRef(refs[i], root.Ref) && !refs[i].GetEmpty() {
			out = append(out, root)
		}
	}
	return out, nil
}

// fillLease returns, in p, the lease a cache fill's roots join: one with at
// least hold left, started in p when the last fill lease has less. A fill
// lease is never renewed, so its roots hold their blocks for at least hold
// and at most hold plus rootRenewInterval, and a process opens about one fill
// lease per rootRenewInterval while it fills.
func (v *Volume) fillLease(ctx context.Context, p *physicalTx, hold time.Duration) (string, error) {
	// Reuse the last fill lease while its record holds enough.
	now := v.now().UnixMilli()
	v.lease.mu.Lock()
	id, expires := v.lease.fill, v.lease.fillExpires
	v.lease.mu.Unlock()
	if id != "" && expires-now >= hold.Milliseconds() {
		stored, _, err := p.rec.GetLease(ctx, id)
		if err != nil || stored == expires {
			return id, err
		}
	}

	// Start the next one.
	id, expires = ulid.NewULID(), now+(hold+rootRenewInterval).Milliseconds()
	if err := p.rec.StartLease(ctx, id, expires); err != nil {
		return "", err
	}
	v.lease.mu.Lock()
	v.lease.fill, v.lease.fillExpires = id, expires
	v.lease.mu.Unlock()
	return id, nil
}

// isRootMoved reports whether err is block.ErrRootMoved.
func isRootMoved(err error) bool {
	return errors.Is(err, block.ErrRootMoved)
}
