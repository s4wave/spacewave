package space_world

import (
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
)

// The replay payloads root holds the payloads the Space's operations name, so
// a device replays them without the device that wrote them, such as the
// target of a migration, which has no peer to fetch them from.
//
// Invariants, which the Volume's collector relies on to free no payload a
// replay may still read:
//
//  1. Hold appends every payload root it reads of an operation before it
//     records the order, so every resolved operation of stateHold has all its
//     payloads held, and an unresolved one the roots read without its type.
//  2. An operation is unresolved when its type does not resolve yet, such as
//     a plugin's before the plugin registers, or it does not prepare, such as
//     before its key arrives. Hold leaves it out of stateHold.ops, so the next
//     Hold reads it again, and marks stateHold unresolved. While stateHold is
//     unresolved or nil, resetPayloads drops nothing: a payload held before a
//     restart may belong to an operation whose type is not known yet. So the
//     payloads root does not shrink while any operation is unresolved, and
//     Hold logs when one stays unresolved.
//  3. The payloads root also holds the payloads of every position above the
//     installed checkpoint and of every pending admission: Apply and the
//     writers append them, and resetPayloads keeps them. It reads the pending
//     admissions again under rootMtx at its move, since an admission between
//     its plan and the move appends no root the set already holds, and keeps
//     every held root they name.
//  4. Only resetPayloads drops a payload, and only one no operation of
//     stateHold, no position and no pending admission names, once every
//     operation of stateHold resolved and the World of its checkpoint is
//     held: the base root holds it, or Install just kept the positions up to
//     it, whose Worlds the histories hold until the trim moves the base onto
//     it. An operation leaves the state only when a checkpoint covers it, so
//     that World holds its effect.
//  5. Only resetPayloads, on the driver goroutine, removes a root from the
//     payload set, and every other change appends. So the roots the set held
//     when resetPayloads read them stay its first roots until it moves it.
//
// Cost: Hold decodes each operation once, when an order first holds it, and
// appends only the payloads it has not held, as one delta block of the root's
// chain. Each Hold walks the order and the last order once through maps,
// encoding no ref it encoded before. resetPayloads does its work outside
// rootMtx: it encodes the positions' payloads and, on a drop, each held root
// once, and builds the block of the roots it keeps. Under rootMtx it encodes
// only the pending admissions' payloads, twice on a drop, and the roots
// appended since it read the set, then makes one root move.

// payloadState is what the payload hold knows of the last order Hold saw.
type payloadState struct {
	// world is the World of the order's checkpoint.
	world *block.BlockRef
	// ops are the payloads of each resolved operation of the order, by hash.
	ops map[string]opHold
	// keys counts, for each payload key, the resolved operations that name
	// it.
	keys map[string]int
	// unresolved is set when an operation of the order did not resolve.
	unresolved bool
	// unresolvedSince is when the run of orders with an unresolved operation
	// began, zero while none is unresolved.
	unresolvedSince time.Time
	// loggedAt is when Hold last logged the run, zero before it logged it.
	loggedAt time.Time
}

// unresolvedLogAfter is how long an operation stays unresolved before Hold
// logs that the payloads root cannot shrink, and how often it logs again.
const unresolvedLogAfter = 10 * time.Minute

// opHold is the payloads one operation names.
type opHold struct {
	// refs are the payload roots.
	refs []*block.BlockRef
	// keys are the key of each of refs.
	keys []string
}

// newOpHold returns the hold of refs, encoding each key once.
func newOpHold(refs []*block.BlockRef) opHold {
	keys := make([]string, len(refs))
	for i, ref := range refs {
		keys[i] = ref.MarshalString()
	}
	return opHold{refs: refs, keys: keys}
}

// Hold holds the payloads of every operation of ev above its checkpoint,
// decoding each operation the last order did not hold. The driver calls it
// before it installs or applies anything of ev. An operation that does not
// resolve or prepare is left unresolved for the next Hold, so apply reaches
// the operations before it and reports its failure there.
func (a *Adapter) Hold(ctx context.Context, ev *sobject.OrderEvent) error {
	// Wait for the claim; a transient replay holds nothing.
	if err := a.waitOpened(ctx); err != nil {
		return err
	}
	if a.opts.transient {
		return nil
	}

	// Decode the checkpoint's World.
	base := &InnerState{}
	if err := base.UnmarshalVT(ev.Checkpoint.GetStateData()); err != nil {
		return errors.Wrap(err, "checkpoint World state")
	}
	next := &payloadState{
		world: base.GetHeadRef().GetRootRef(),
		ops:   make(map[string]opHold, len(ev.Order)),
	}

	// Read the payloads of the operations the last order did not hold.
	prev := a.stateHold
	if prev == nil {
		prev = &payloadState{keys: make(map[string]int)}
	}
	var extra []*block.BlockRef
	var added []string
	unresolved := 0
	for _, h := range ev.Order {
		key := string(h)
		if op, ok := prev.ops[key]; ok {
			next.ops[key] = op
			continue
		}
		refs, known, err := opPayloads(ctx, ev.Snapshot, ev.Set, h, a.lookupOp)
		if err != nil && ctx.Err() != nil {
			return context.Cause(ctx)
		}
		extra = append(extra, refs...)
		if err != nil || !known {
			unresolved++
			continue
		}
		next.ops[key] = newOpHold(refs)
		added = append(added, key)
	}

	// Hold them, then record the order, counting the keys of the operations
	// it added and dropped.
	if len(extra) != 0 {
		if err := a.addPayloads(ctx, extra); err != nil {
			return err
		}
	}
	next.keys = prev.keys
	for _, key := range added {
		for _, k := range next.ops[key].keys {
			next.keys[k]++
		}
	}
	for key, op := range prev.ops {
		if _, ok := next.ops[key]; ok {
			continue
		}
		for _, k := range op.keys {
			if next.keys[k]--; next.keys[k] <= 0 {
				delete(next.keys, k)
			}
		}
	}
	if unresolved != 0 {
		a.noteUnresolved(prev, next, unresolved)
	}
	a.stateHold = next
	return nil
}

// noteUnresolved marks next unresolved, with n operations unresolved, and
// logs once the run that began in prev has lasted unresolvedLogAfter, then at
// most once each unresolvedLogAfter. The line names no operation, key or
// config.
func (a *Adapter) noteUnresolved(prev, next *payloadState, n int) {
	// Carry the run's start and last line from prev.
	now := time.Now()
	next.unresolved, next.unresolvedSince, next.loggedAt = true, prev.unresolvedSince, prev.loggedAt
	if next.unresolvedSince.IsZero() {
		next.unresolvedSince = now
	}

	// Log once the run and the time since the last line are long enough.
	if now.Sub(next.unresolvedSince) < unresolvedLogAfter || (!next.loggedAt.IsZero() && now.Sub(next.loggedAt) < unresolvedLogAfter) {
		return
	}
	next.loggedAt = now
	a.le.WithField("unresolved-ops", n).
		WithField("unresolved-for", now.Sub(next.unresolvedSince).Round(time.Second).String()).
		Warn("operations stay unresolved, so their Space keeps every payload it holds")
}

// resetPayloads holds the payloads of the operations of the last order Hold
// saw, of the positions above the checkpoint and of the pending admissions,
// dropping the others once every operation of that order resolved and its
// checkpoint World is held: the base root holds it, or kept reports that
// Install just kept the positions up to the replay base and it is that World.
// Without a drop it appends what it does not hold yet. The driver goroutine
// calls it.
func (a *Adapter) resetPayloads(ctx context.Context, kept bool) error {
	// Read the base and the held payloads, then key the positions' payloads,
	// outside the lock.
	unlock, err := a.rootMtx.Lock(ctx)
	if err != nil {
		return err
	}
	base, snap := a.held[RootReplayBase], a.payloads.roots
	unlock()
	positions := a.replay.payloads()
	posKeys := newOpHold(positions).keys
	pending := a.pendingPayloads()

	// Plan the drop once the order resolved and its World is held.
	st := a.stateHold
	var drop *payloadDrop
	if st != nil && !st.unresolved &&
		(refEqual(base, st.world) || (kept && refEqual(a.replay.base.GetHeadRef().GetRootRef(), st.world))) {
		if drop, err = planPayloadDrop(snap, st.keys, posKeys); err != nil {
			return err
		}
	}
	if drop != nil && a.resetPlanned != nil {
		a.resetPlanned()
	}

	// Take the lock, and find the payloads not held yet.
	release, err := a.lockRoots(ctx, slices.Concat(positions, pending)...)
	if err != nil {
		return err
	}
	defer release()
	h := &a.payloads
	var missing []*block.BlockRef
	for i, ref := range positions {
		if _, ok := h.keys[posKeys[i]]; !ok {
			missing = append(missing, ref)
		}
	}
	missing = append(missing, slices.DeleteFunc(slices.Clone(pending), h.holds)...)

	// Append them, or move onto the planned set with them and every held root
	// a pending admission names, read again under the lock (invariant 3).
	if drop == nil {
		return a.appendRoots(ctx, h, missing)
	}
	admitted := slices.DeleteFunc(a.pendingPayloads(), func(ref *block.BlockRef) bool { return !h.holds(ref) })
	return a.movePayloadDrop(ctx, drop, len(snap), slices.Concat(missing, admitted))
}

// pendingPayloads returns the payloads of the pending admissions. The driver
// goroutine calls it.
func (a *Adapter) pendingPayloads() []*block.BlockRef {
	var payloads []*block.BlockRef
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, adm := range a.pending {
			payloads = append(payloads, adm.payloads...)
		}
	})
	return payloads
}

// payloadDrop is the payload set resetPayloads moves onto: the held roots it
// keeps, encoded as one block.
type payloadDrop struct {
	// roots are the kept roots, in held order.
	roots []*block.BlockRef
	// keys are the keys of roots.
	keys map[string]struct{}
	// entry is the HeldRoots block naming roots, nil when it keeps none.
	entry *block.PutBatchEntry
}

// planPayloadDrop returns the roots of held that keys or posKeys name, with
// the block that holds them, or nil when it keeps every root. It runs outside
// rootMtx.
func planPayloadDrop(held []*block.BlockRef, keys map[string]int, posKeys []string) (*payloadDrop, error) {
	// Key the positions' payloads beside the order's.
	pos := make(map[string]struct{}, len(posKeys))
	for _, k := range posKeys {
		pos[k] = struct{}{}
	}

	// Keep the held roots either names.
	d := &payloadDrop{keys: make(map[string]struct{}, len(keys)+len(pos))}
	for _, ref := range held {
		k := ref.MarshalString()
		_, atPos := pos[k]
		if keys[k] == 0 && !atPos {
			continue
		}
		d.roots = append(d.roots, ref)
		d.keys[k] = struct{}{}
	}
	if len(d.roots) == len(held) {
		return nil, nil
	}

	// Encode the block that holds them.
	if len(d.roots) != 0 {
		var err error
		if d.entry, err = refsEntry(&HeldRoots{Roots: d.roots}, d.roots); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// movePayloadDrop moves the payloads root onto the roots of d, followed by
// the roots appended since the set held n roots and missing, as at most two
// blocks. The caller holds rootMtx.
func (a *Adapter) movePayloadDrop(ctx context.Context, d *payloadDrop, n int, missing []*block.BlockRef) error {
	// Take the roots appended since the plan read the set (invariant 5).
	h := &a.payloads
	if len(h.roots) < n {
		return errors.Errorf("payload set shrank from %d to %d roots outside resetPayloads", n, len(h.roots))
	}
	extra := dedupRefs(slices.Concat(h.roots[n:], missing))
	keys := d.keys
	extra = slices.DeleteFunc(extra, func(ref *block.BlockRef) bool {
		k := ref.MarshalString()
		if _, ok := keys[k]; ok {
			return true
		}
		keys[k] = struct{}{}
		return false
	})

	// Name the kept block, then the appended roots with it as their Prev.
	var entries []*block.PutBatchEntry
	var top *block.BlockRef
	links := 0
	if d.entry != nil {
		entries, top, links = append(entries, d.entry), d.entry.Ref, 1
	}
	if len(extra) != 0 {
		refs := slices.Clone(extra)
		if top != nil {
			refs = append(refs, top)
		}
		entry, err := refsEntry(&HeldRoots{Roots: extra, Prev: top}, refs)
		if err != nil {
			return err
		}
		entries, top = append(entries, entry), entry.Ref
		links++
	}

	// Move the root, then record the roots it holds.
	if err := a.moveRootsLocked(ctx, entries, block.NamedRoot{Name: h.name, Ref: top}); err != nil {
		return err
	}
	h.roots, h.keys, h.links = slices.Concat(d.roots, extra), keys, links
	return nil
}
