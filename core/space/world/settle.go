package space_world

import (
	"bytes"
	"cmp"
	"context"
	"slices"
	"sync"

	"github.com/aperturerobotics/util/broadcast"
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
// An adapter registers each write before it passes it to
// QueueOperationWithReceipt, so the entry exists before storage can hold the
// operation and before any replay can see it. The entry is in flight until
// the adapter decides the write: it drops it when the write is decided, and
// tells it when the write ends unknown. Nothing orders that tell before the
// next mount's adapter opens, since the restart may run inside the reload the
// unknown called, so a live adapter settles an entry in two ways:
//
//   - Its replay applies the entry's operation: the mount's state holds it,
//     so the replay's decision answers it, told or not yet told.
//   - The entry is told, and the adapter's mount loaded its state after the
//     write's mount ended its writes: an operation the loaded state lacks
//     was never stored.
//
// Invariant: an entry resolves only from the replay of an adapter whose
// mount's state shows whether the write was stored. A mount other than the
// one the write went unknown on loaded the state storage held after every
// write the old mount sent before that load (the provider's reload barrier).
// With the reload fence on, the old mount sends nothing after the unknown, so
// an entry sent before this adapter opened is decided by the state it loaded.
// With sobject.ReloadFenceOff the old mount keeps writing until the reload
// ends its run, so the head an entry settles at may hold writes that followed
// the unknown. The mount the write went unknown on shows it only once the
// write's QueueOperation returned on it without error (stored). A write still
// inside QueueOperation, or one whose outcome storage could not show, waits
// for another mount, since a mount fenced for a reload may lack a write
// storage holds.
type settleSet struct {
	// bcast guards the fields below and every entry's fields, and broadcasts
	// when an entry is told, stored or resolved. Its holder takes no adapter
	// lock: an adapter takes its own bcast first, then this one.
	bcast broadcast.Broadcast
	// entries are the writes sent and not yet decided, and the unknown ones
	// until a WaitSettled reads their answer, by send order. There is one per
	// write in flight, and one per unknown write, which is rare.
	entries map[uint64]*settleEntry
	// sent is the send order of the last entry registered.
	sent uint64
	// rev counts the tells and stores, which can make an entry adoptable.
	rev uint64
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

// settleEntry is one write sent to a mount, from its send until its decision,
// or until a WaitSettled reads its answer when its outcome went unknown.
type settleEntry struct {
	// seq is the entry's send order.
	seq uint64
	// receipt is the sending admission's receipt, which names the operation
	// once the signer published it, or nil once told or resolved.
	receipt *sobject.SignedOperationReceipt
	// pos is the operation's position in this device's chain once told.
	pos *sobject.SOOperationPosition
	// so is the mount the write was sent on, or nil once resolved.
	so sobject.SharedObject
	// told reports that the write's adapter decided it unknown.
	told bool
	// stored reports that the write's QueueOperation returned on so without
	// error, so the next adapter on so may settle it too.
	stored bool
	// resolved reports that the fields below are set.
	resolved bool
	// head is the installed head of the adapter that settled the write.
	head world.Head
	// applied reports that the operation applied.
	applied bool
	// err is why the outcome cannot be settled, such as world.ErrOutcomeHidden.
	err error
}

// position returns the entry's operation position, or nil before the signer
// published it. The caller holds the set's bcast.
func (e *settleEntry) position() *sobject.SOOperationPosition {
	if e.told {
		return e.pos
	}
	return e.receipt.Position()
}

// newSettleSet returns an empty settle set.
func newSettleSet() *settleSet {
	return &settleSet{entries: make(map[uint64]*settleEntry)}
}

// register adds the write about to be sent on so, whose position receipt
// will hold, and returns its entry.
func (s *settleSet) register(so sobject.SharedObject, receipt *sobject.SignedOperationReceipt) *settleEntry {
	var ent *settleEntry
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		s.sent++
		ent = &settleEntry{seq: s.sent, receipt: receipt, so: so}
		s.entries[ent.seq] = ent
	})
	return ent
}

// last returns the send order of the last entry registered.
func (s *settleSet) last() uint64 {
	var seq uint64
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		seq = s.sent
	})
	return seq
}

// tell records that the write of ent ended unknown at pos. stored reports
// that its QueueOperation returned without error. An entry another adapter's
// replay already resolved keeps its answer.
func (s *settleSet) tell(ent *settleEntry, pos *sobject.SOOperationPosition, stored bool) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		ent.pos, ent.receipt, ent.told = pos.CloneVT(), nil, true
		ent.stored = stored
		s.rev++
		broadcast()
	})
}

// drop removes the entry of a write its adapter decided.
func (s *settleSet) drop(ent *settleEntry) {
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		delete(s.entries, ent.seq)
	})
}

// markStored records that the QueueOperation of ent's write returned without
// error after the write was told unknown, as when the adapter ended while the
// call ran.
func (s *settleSet) markStored(ent *settleEntry) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if ent.told && !ent.resolved {
			ent.stored = true
			s.rev++
			broadcast()
		}
	})
}

// replayed returns the unresolved entry of the operation of hash, which an
// adapter's replay applied, and its position, or nil. A nil set has none.
func (s *settleSet) replayed(hash []byte) (*settleEntry, *sobject.SOOperationPosition) {
	// A nil set or an empty hash matches nothing.
	if s == nil || len(hash) == 0 {
		return nil, nil
	}

	// Find the unresolved entry with that hash.
	var ent *settleEntry
	var pos *sobject.SOOperationPosition
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, e := range s.entries {
			if e.resolved {
				continue
			}
			if p := e.position(); bytes.Equal(p.GetOpHash(), hash) {
				ent, pos = e, p
				return
			}
		}
	})
	return ent, pos
}

// waitTold waits until the count of tells and stores differs from rev, and
// returns the entries an adapter on so that opened after send order upto may
// settle by absence, in nonce order, with the new count. Those are the told,
// unresolved entries sent by then, whose write went unknown on another mount
// or that so stored.
func (s *settleSet) waitTold(ctx context.Context, so sobject.SharedObject, upto, rev uint64) ([]*settleEntry, uint64, error) {
	var out []*settleEntry
	err := s.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		if s.rev == rev {
			return false, nil
		}
		rev = s.rev
		for _, ent := range s.entries {
			if ent.told && !ent.resolved && ent.seq <= upto && (ent.so != so || ent.stored) {
				out = append(out, ent)
			}
		}
		return true, nil
	})
	slices.SortFunc(out, func(a, b *settleEntry) int {
		return cmp.Compare(a.pos.GetNonce(), b.pos.GetNonce())
	})
	return out, rev, err
}

// resolve sets the answer of ent once, and drops the mount and receipt it
// pins. A second call does nothing.
func (s *settleSet) resolve(ent *settleEntry, head world.Head, applied bool, err error) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if ent.resolved {
			return
		}
		ent.head, ent.applied, ent.err = head, applied, err
		ent.so, ent.receipt, ent.resolved = nil, nil, true
		broadcast()
	})
}

// wait waits for the answer of the write of opHash that ended unknown, and
// removes its entry once it returns the answer. A WaitSettled already waiting
// reads it too, and a later one returns ErrNoSettle.
func (s *settleSet) wait(ctx context.Context, opHash []byte) (world.Head, bool, error) {
	// Find the told entry of opHash, then wait for its answer.
	var ent *settleEntry
	err := s.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		if ent == nil && len(opHash) != 0 {
			for _, e := range s.entries {
				if e.told && bytes.Equal(e.pos.GetOpHash(), opHash) {
					ent = e
					break
				}
			}
		}
		if ent == nil {
			return false, ErrNoSettle
		}
		return ent.resolved, nil
	})
	if err != nil && ctx.Err() != nil {
		return world.Head{}, false, context.Cause(ctx)
	}
	if err != nil {
		return world.Head{}, false, err
	}

	// Remove the answered entry.
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		delete(s.entries, ent.seq)
	})
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

// adoptLocked makes ent's write, at pos, a pending admission of this
// adapter, with no operation or candidate of its own, so replay's match, the
// checkpoint cover and installHead decide it as they decide a live write.
// decide resolves its entry. The caller holds bcast.
func (a *Adapter) adoptLocked(ent *settleEntry, pos *sobject.SOOperationPosition) *spaceAdmission {
	// Admit a pending admission already at pos.
	adm := a.newAdmission(world.Candidate{}, nil, nil, nil)
	adm.receipt.Publish(pos)
	adm.pos, adm.hash, adm.settle = pos, slices.Clone(pos.GetOpHash()), ent
	a.admitLocked(adm)
	return adm
}

// adoptReplayedLocked adopts the entry of op, an operation of this device
// that replay applied and no pending admission matched, so replay decides it:
// a write in flight on another adapter, or one an adapter left unknown. It
// returns nil when no unresolved entry names op. The caller holds bcast.
func (a *Adapter) adoptReplayedLocked(op *sobject.OrderedOperation) *spaceAdmission {
	ent, pos := a.settles.replayed(op.Hash)
	if ent == nil {
		return nil
	}
	if adm := a.adoptedLocked(ent); adm != nil {
		return adm
	}
	return a.adoptLocked(ent, pos)
}

// adoptedLocked returns the pending admission of this adapter that settles
// ent, or nil. An adopted admission leaves pending only once decide resolved
// its entry, or at the run's end. The caller holds bcast.
func (a *Adapter) adoptedLocked(ent *settleEntry) *spaceAdmission {
	for _, adm := range a.pending {
		if adm.settle == ent {
			return adm
		}
	}
	return nil
}

// adoptSettles adopts each told write this adapter may settle by absence,
// as it is told, until ctx ends: those sent before the adapter opened, whose
// mount ended its writes before this one loaded (see settleSet). It applies
// the installed checkpoint's cover to each, as queued does for a late live
// write, then settles each the loaded state lacks.
func (a *Adapter) adoptSettles(ctx context.Context) error {
	var rev uint64
	for {
		// Wait for a tell or a store.
		ents, next, err := a.settles.waitTold(ctx, a.so, a.settleSeq, rev)
		if err != nil {
			return context.Canceled
		}
		rev = next

		// Adopt the ones not adopted yet, and decide those the cover answers.
		var adopted, refused, unknown, accepted []*spaceAdmission
		a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			for _, ent := range ents {
				if a.adoptedLocked(ent) != nil {
					continue
				}
				adm := a.adoptLocked(ent, ent.pos)
				adopted = append(adopted, adm)
				switch a.coverLocked(adm) {
				case coverRefused:
					refused = append(refused, adm)
				case coverUnknown:
					unknown = append(unknown, adm)
				case coverApplied:
					if a.acceptHeight >= a.cover.height {
						accepted = append(accepted, adm)
					}
				}
			}
		})
		a.decideCovered(refused, unknown)
		if len(accepted) != 0 {
			a.decide(accepted, world.Accepted, "", nil)
		}
		if len(adopted) != 0 {
			if err := a.settleAdopted(ctx, adopted); err != nil {
				return err
			}
		}
	}
}

// registerSent registers adm's write in the settle set before the queue
// passes it to QueueOperationWithReceipt, so the entry exists before storage
// can hold the operation. An admission already decided, as at the run's end,
// sends without one, as before the set.
func (a *Adapter) registerSent(adm *spaceAdmission) {
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if adm.state != admissionDecided {
			adm.sent = a.settles.register(a.so, &adm.receipt)
		}
	})
}

// settleAdopted decides each adopted write the mount's state lacks: once
// replay installed a head at or past that state's order, a write neither held
// in its operation set nor decided by the installed checkpoint's cover was
// never stored, so it did not apply. The mount loaded after the write's mount
// ended its writes, so a later read of its state lacks the write only when
// the loaded one did.
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

// settleDecidedLocked tells, drops or resolves the settle entry of adm, which
// decide just decided. An adopted write resolves its entry: accepted applied
// at its head, refused did not, and one a checkpoint covers without its
// outcome resolves with that error. Any other unknown ending, as at unmount,
// leaves it for the next adapter. A sent write that ended unknown under its
// own hash tells its entry, stored when queued recorded its position, and
// resolves it at once when a checkpoint hides its outcome. Any other ending
// drops it, since no WaitSettled asks for it. The caller holds bcast.
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

	// Tell a sent write that ended unknown under its own hash, and drop any
	// other.
	ent := adm.sent
	if ent == nil {
		return
	}
	var unknown *world.DurabilityUnknownError
	pos := adm.receipt.Position()
	if adm.decision != world.Undecided || !errors.As(adm.err, &unknown) || pos == nil || !bytes.Equal(unknown.OpHash, pos.GetOpHash()) {
		a.settles.drop(ent)
		return
	}
	a.settles.tell(ent, pos, adm.pos != nil)
	if errors.Is(adm.err, world.ErrOutcomeHidden) {
		a.settles.resolve(ent, adm.head, false, adm.err)
	}
}

// WaitSettled waits until an adapter whose mount shows the write of opHash,
// whose commit ended with world.DurabilityUnknownError, settles it, and
// returns that adapter's installed head and whether the operation applied. It
// returns the answer once: a later call for the hash returns ErrNoSettle. A
// write the installed checkpoint covers without recording its outcome
// returns world.ErrOutcomeHidden, and an operation hash with no such write
// returns ErrNoSettle.
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
