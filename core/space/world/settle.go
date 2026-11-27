package space_world

import (
	"bytes"
	"cmp"
	"context"
	"slices"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/world"
)

// ErrNoSettle is returned by WaitSettled for an operation hash no write of
// this process ended with an unknown outcome.
var ErrNoSettle = errors.New("no write with an unknown outcome has this operation hash")

// reasonNotStored is the reason of an adopted write the reloaded state lacks.
const reasonNotStored = "not stored by the write whose outcome was unknown"

// settleSet settles the writes of this device whose outcome went unknown,
// across the successive adapters of one Space World. The factory shares one
// set with each controller it constructs for the engine, and the controller
// with each adapter it runs.
//
// Invariant: an entry resolves only from the replay of an adapter whose
// mount's state shows whether the write was stored. A mount other than the
// one the write went unknown on loaded the state storage held after the old
// mount wrote its last write, and the old mount writes nothing after the
// unknown (the provider's reload fence and the adapter's Reloading gate), so
// the settled head is the state at the unknown, with the unknown write when
// it applied, plus what other devices wrote. The mount the write went
// unknown on shows it only once the write's QueueOperation returned on it
// without error (stored), as when the adapter ended before replay decided
// the write: its state then holds the write, as it would for a live adapter.
// A write still inside QueueOperation, or one whose outcome storage could
// not show, waits for another mount, since a mount fenced for a reload may
// lack a write storage holds.
type settleSet struct {
	// mtx guards entries.
	mtx sync.Mutex
	// entries are the unknown writes by operation hash. A resolved entry is
	// kept until a WaitSettled reads its answer, and keeps only that answer.
	// There is one per unknown write, which is rare.
	entries map[string]*settleEntry
}

// settleSets holds one settleSet per engine ID. A set stays for the life of
// the factory: it is a map header and the rare unknown writes, one per engine
// this process ran.
type settleSets struct {
	// mtx guards sets.
	mtx sync.Mutex
	// sets are the settle sets by engine ID, made on first use.
	sets map[string]*settleSet
}

// get returns the settle set of engineID, making it on first use.
func (s *settleSets) get(engineID string) *settleSet {
	// Take the set, making it on first use.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.sets == nil {
		s.sets = make(map[string]*settleSet)
	}
	set, ok := s.sets[engineID]
	if !ok {
		set = newSettleSet()
		s.sets[engineID] = set
	}
	return set
}

// settleEntry is one write whose outcome went unknown.
type settleEntry struct {
	// pos is the operation's position in this device's chain.
	pos *sobject.SOOperationPosition
	// so is the mount the write went unknown on, or nil once resolved.
	so sobject.SharedObject
	// stored reports that the write's QueueOperation returned on so without
	// error, so the next adapter on so may settle it too.
	stored bool
	// done closes once the fields below are set.
	done chan struct{}
	// head is the installed head of the adapter that settled the write.
	head world.Head
	// applied reports that the operation applied.
	applied bool
	// err is why the outcome cannot be settled, such as world.ErrOutcomeHidden.
	err error
}

// newSettleSet returns an empty settle set.
func newSettleSet() *settleSet {
	return &settleSet{entries: make(map[string]*settleEntry)}
}

// add registers the write at pos, which went unknown on so, and returns its
// entry. stored reports that its QueueOperation returned on so without error.
// A write already registered keeps its entry.
func (s *settleSet) add(so sobject.SharedObject, pos *sobject.SOOperationPosition, stored bool) *settleEntry {
	// Keep the entry of a write already registered.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	key := string(pos.GetOpHash())
	if ent, ok := s.entries[key]; ok {
		return ent
	}
	ent := &settleEntry{pos: pos.CloneVT(), so: so, stored: stored, done: make(chan struct{})}
	s.entries[key] = ent
	return ent
}

// markStored records that the QueueOperation of the write of opHash returned
// on so without error after its entry was registered, as when the adapter
// ended while the call ran.
func (s *settleSet) markStored(so sobject.SharedObject, opHash []byte) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if ent, ok := s.entries[string(opHash)]; ok && ent.so == so {
		ent.stored = true
	}
}

// unsettled returns the entries still unresolved that an adapter on so may
// settle, in nonce order: those whose write went unknown on another mount,
// and those so stored.
func (s *settleSet) unsettled(so sobject.SharedObject) []*settleEntry {
	// Collect the open entries so's state shows.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	var out []*settleEntry
	for _, ent := range s.entries {
		if ent.so == so && !ent.stored {
			continue
		}
		select {
		case <-ent.done:
		default:
			out = append(out, ent)
		}
	}
	slices.SortFunc(out, func(a, b *settleEntry) int {
		return cmp.Compare(a.pos.GetNonce(), b.pos.GetNonce())
	})
	return out
}

// resolve sets the answer of ent once, and drops the mount it pins. A second
// call does nothing.
func (s *settleSet) resolve(ent *settleEntry, head world.Head, applied bool, err error) {
	// Skip an entry already resolved.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	select {
	case <-ent.done:
		return
	default:
	}
	ent.head, ent.applied, ent.err = head, applied, err
	ent.so = nil
	close(ent.done)
}

// wait waits for the answer of the write of opHash, and removes its entry once
// it returns the answer. A WaitSettled already waiting reads it too, and a
// later one returns ErrNoSettle.
func (s *settleSet) wait(ctx context.Context, opHash []byte) (world.Head, bool, error) {
	// Find the entry of opHash.
	key := string(opHash)
	s.mtx.Lock()
	ent, ok := s.entries[key]
	s.mtx.Unlock()
	if !ok || len(opHash) == 0 {
		return world.Head{}, false, ErrNoSettle
	}
	select {
	case <-ctx.Done():
		return world.Head{}, false, context.Cause(ctx)
	case <-ent.done:
	}

	// Remove the answered entry.
	s.mtx.Lock()
	if s.entries[key] == ent {
		delete(s.entries, key)
	}
	s.mtx.Unlock()
	return ent.head, ent.applied, ent.err
}

// durabilityUnknown returns the unknown outcome of a write with cause, naming
// its operation by opHash when the signer published one.
func durabilityUnknown(opHash []byte, cause error) error {
	if len(opHash) != 0 {
		return &world.DurabilityUnknownError{OpHash: opHash, Err: cause}
	}
	return errors.Wrap(world.ErrDurabilityUnknown, cause.Error())
}

// adoptSettles makes each write another mount left unknown a pending
// admission of this adapter, with no operation or candidate of its own, so
// replay's match, the checkpoint cover and installHead decide it as they
// decide a live write. decide resolves its entry. It runs before the driver's
// first step and returns the adopted admissions.
func (a *Adapter) adoptSettles() []*spaceAdmission {
	var adopted []*spaceAdmission
	for _, ent := range a.settles.unsettled(a.so) {
		adm := a.newAdmission(world.Candidate{}, nil, nil, nil)
		adm.receipt.Publish(ent.pos)
		adm.pos, adm.hash, adm.settle = ent.pos, slices.Clone(ent.pos.GetOpHash()), ent
		adopted = append(adopted, adm)
	}
	if len(adopted) != 0 {
		a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			for _, adm := range adopted {
				a.admitLocked(adm)
			}
		})
	}
	return adopted
}

// settleAdopted decides each adopted write the state this mount loaded lacks:
// once replay installed a head at or past that state's order, a write neither
// held in its operation set nor decided by the installed checkpoint's cover
// was never stored, so it did not apply.
func (a *Adapter) settleAdopted(ctx context.Context, adopted []*spaceAdmission) error {
	// Read which adopted writes the loaded state holds.
	snap, err := a.so.GetSharedObjectState(ctx)
	if err != nil {
		return err
	}
	target, err := sobject.ReadOrder(ctx, snap)
	if err != nil {
		return err
	}
	stored := make(map[*spaceAdmission]bool, len(adopted))
	for _, adm := range adopted {
		stored[adm] = target.Set.Get(adm.hash) != nil
	}

	// Wait for replay to reach that order, then take the ones still queued
	// that neither the order nor the cover holds.
	var lost []*spaceAdmission
	err = a.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		if !a.installed || !a.reachedLocked(target) {
			return false, nil
		}
		for _, adm := range adopted {
			if adm.state == admissionQueued && !stored[adm] && a.cover.outcome(adm.pos) == coverNone {
				lost = append(lost, adm)
			}
		}
		return true, nil
	})
	if err != nil {
		return context.Canceled
	}
	if len(lost) != 0 {
		a.decide(lost, world.Rejected, reasonNotStored, nil)
	}
	return nil
}

// settleDecidedLocked registers or resolves the settle of adm, which decide
// just decided. An adopted write resolves its entry: accepted applied at its
// head, refused did not, and one a checkpoint covers without its outcome
// resolves with that error. Any other unknown ending, as at unmount, leaves
// it for the next adapter. A live write that ended unknown registers an entry,
// stored when queued recorded its position, and resolved at once when a
// checkpoint hides its outcome. The caller holds bcast.
func (a *Adapter) settleDecidedLocked(adm *spaceAdmission) {
	// Resolve an adopted write from its decision.
	if ent := adm.settle; ent != nil {
		switch {
		case adm.decision == world.Accepted:
			a.settles.resolve(ent, adm.head, true, nil)
		case adm.decision != world.Undecided:
			a.settles.resolve(ent, adm.head, false, nil)
		case errors.Is(adm.err, world.ErrOutcomeHidden):
			a.settles.resolve(ent, adm.head, false, adm.err)
		}
		return
	}

	// Register a live write that ended unknown under its own hash.
	var unknown *world.DurabilityUnknownError
	pos := adm.receipt.Position()
	if adm.decision != world.Undecided || !errors.As(adm.err, &unknown) || pos == nil || !bytes.Equal(unknown.OpHash, pos.GetOpHash()) {
		return
	}
	ent := a.settles.add(a.so, pos, adm.pos != nil)
	if errors.Is(adm.err, world.ErrOutcomeHidden) {
		a.settles.resolve(ent, adm.head, false, adm.err)
	}
}

// WaitSettled waits until a successor adapter settles the write of opHash
// whose commit ended with world.DurabilityUnknownError, and returns that
// adapter's installed head and whether the operation applied. A write the
// installed checkpoint covers without recording its outcome returns
// world.ErrOutcomeHidden, and an operation hash with no such write returns
// ErrNoSettle.
func (e *Engine) WaitSettled(ctx context.Context, opHash []byte) (world.Head, bool, error) {
	return e.a.settles.wait(ctx, opHash)
}

// WaitSettled waits as Engine.WaitSettled does, for a write on any engine
// of this controller's engine ID.
func (c *Controller) WaitSettled(ctx context.Context, opHash []byte) (world.Head, bool, error) {
	return c.settles.wait(ctx, opHash)
}

// _ is a type assertion
var (
	_ world.SettleWaiter = (*Engine)(nil)
	_ world.SettleWaiter = (*Controller)(nil)
)
