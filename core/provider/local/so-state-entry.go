package provider_local

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/promise"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/db/world"
)

// soStateAdmission is a state write admitted to a publication whose receipt
// has not settled.
type soStateAdmission struct {
	// state is the state after the admitted write.
	state *sobject.SOState
	// receipt resolves with the publication's durable result. The entry reads
	// it and never releases it; the publisher's caller owns it.
	receipt *block.PublicationReceipt
	// dependent records that an earlier admission failed. The publication's
	// head expects that admission's state, so it fails too.
	dependent bool
	// release drops the entry reference the admission holds until it settles.
	release func()
}

// soStateEntry is the mounted state of one shared object. Its durable state
// is what storage holds, or will hold once every successful receipt so far
// is applied; its admitted state adds the writes admitted to publications
// whose receipts are pending. Consumers off the World engine read only the
// durable state, so nothing admitted leaves the device before its receipt.
type soStateEntry struct {
	// key addresses the serialized SOState in the object store.
	key []byte
	// stateProm completes after loading the persisted state, once stateCtr
	// holds it.
	stateProm *promise.PromiseContainer[*sobject.SOState]
	// stateCtr publishes the durable state.
	stateCtr *ccontainer.CContainer[*sobject.SOState]
	// admittedCtr publishes the admitted state: the durable state followed by
	// every pending admission that does not depend on a failed one.
	admittedCtr *ccontainer.CContainer[*sobject.SOState]
	// writeMtx serializes admissions and direct writes while the entry is
	// retained. An admission holds it until the publisher admits its write.
	writeMtx csync.Mutex
	// intent serializes writers before either an engine or provider writer is
	// acquired. A direct writer drains receipts here without blocking readers
	// or an engine writer, and prevents new admissions from starving its drain.
	intent csync.Mutex

	// bcast guards pending and the settling of stateCtr and admittedCtr, and
	// wakes the settling routine when an admission arrives.
	bcast broadcast.Broadcast
	// pending are the admissions whose receipts have not settled, in
	// admission order, which is the order the publisher resolves them.
	pending []*soStateAdmission
}

// newSOStateEntry constructs the entry of the state at key.
func newSOStateEntry(key []byte) *soStateEntry {
	return &soStateEntry{
		key:         key,
		stateProm:   promise.NewPromiseContainer[*sobject.SOState](),
		stateCtr:    ccontainer.NewCContainer[*sobject.SOState](nil),
		admittedCtr: ccontainer.NewCContainer[*sobject.SOState](nil),
	}
}

// execute loads the durable state of sharedObjectID from objStore, then
// settles receipts in admission order until ctx ends. Each pending admission
// retains the entry, so ctx outlives every admission.
func (e *soStateEntry) execute(ctx context.Context, objStore object.ObjectStore, sharedObjectID string) error {
	// Read the durable state through the store transaction boundary.
	var data []byte
	var found bool
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			var err error
			data, found, err = tx.Get(ctx, e.key)
			return err
		},
	)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}

	// Validate the persisted state before exposing it to consumers.
	val := &sobject.SOState{}
	if err := val.UnmarshalVT(data); err != nil {
		return err
	}
	if err := val.Validate(sharedObjectID); err != nil {
		return err
	}

	// Publish the validated initial state to waiters and watches.
	e.stateCtr.SetValue(val)
	e.admittedCtr.SetValue(val)
	e.stateProm.SetResult(val, nil)

	// Settle each receipt as it resolves.
	for {
		var released []func()
		var done, waitCh <-chan struct{}
		e.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			released = e.settleLocked()
			if len(e.pending) != 0 {
				done = e.pending[0].receipt.Done()
			}
			waitCh = getWaitCh()
		})
		releaseAll(released)
		select {
		case <-ctx.Done():
			return nil
		case <-done:
		case <-waitCh:
		}
	}
}

// readAdmitted returns the admitted state after settling the resolved receipts,
// so a write never builds on an admission already known to have failed. The
// caller holds writeMtx.
func (e *soStateEntry) readAdmitted() *sobject.SOState {
	var state *sobject.SOState
	var released []func()
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		released = e.settleLocked()
		state = e.admittedCtr.GetValue()
	})
	releaseAll(released)
	return state
}

// waitSettled waits until every pending receipt settled and returns the durable
// state, which then equals the admitted state. Reservation callers hold only
// intent, preventing new admissions while keeping both writers free. The legacy
// lane retains its original provider-writer wait.
func (e *soStateEntry) waitSettled(ctx context.Context) (*sobject.SOState, error) {
	for {
		// Settle completed receipts and subscribe to the next pending one.
		state, done := e.readSettled()
		if done == nil {
			return state, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-done:
		}
	}
}

// readSettled returns the durable state when nothing is pending, or the next
// receipt's wait channel. It performs no wait and requires no writer lock.
func (e *soStateEntry) readSettled() (*sobject.SOState, <-chan struct{}) {
	// Settle the resolved prefix and retain its release callbacks outside bcast.
	var state *sobject.SOState
	var released []func()
	var done <-chan struct{}
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		released = e.settleLocked()
		if len(e.pending) != 0 {
			done = e.pending[0].receipt.Done()
			return
		}
		state = e.stateCtr.GetValue()
	})
	releaseAll(released)
	return state, done
}

// admit records state, built on base, as admitted to the publication of
// receipt. release drops the entry reference the admission holds until its
// receipt settles. The caller holds writeMtx, so only a failed receipt
// settling since the caller read base as the admitted state can have changed
// it. The publication's head then expects a state that never became durable:
// admit tracks the admission as dependent, leaves the admitted state alone,
// and returns ErrPublicationDependency.
func (e *soStateEntry) admit(base, state *sobject.SOState, receipt *block.PublicationReceipt, release func()) error {
	var err error
	e.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Track the admission, dependent when the admitted state was reset.
		dependent := e.admittedCtr.GetValue() != base
		e.pending = append(e.pending, &soStateAdmission{state: state, receipt: receipt, dependent: dependent, release: release})

		// Publish the admitted state unless the admission is dependent.
		if dependent {
			err = errors.Wrap(block.ErrPublicationDependency, "shared object state write built on a failed publication")
		} else {
			e.admittedCtr.SetValue(state)
		}
		broadcast()
	})
	return err
}

// commit records state as written directly to storage. The caller holds
// writeMtx and wrote state after every pending receipt settled.
func (e *soStateEntry) commit(state *sobject.SOState) {
	e.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		e.stateCtr.SetValue(state)
		e.admittedCtr.SetValue(state)
		e.stateProm.SetResult(state, nil)
	})
}

// settleLocked moves the resolved prefix of pending into the durable state
// and returns the entry references to release outside bcast. A failed
// admission resets the admitted state to the durable state and marks every
// later admission dependent, since each was built on it. The caller holds
// bcast.
func (e *soStateEntry) settleLocked() []func() {
	var released []func()
	for len(e.pending) != 0 {
		// Stop at the first unresolved receipt.
		adm := e.pending[0]
		select {
		case <-adm.receipt.Done():
		default:
			return released
		}
		e.pending = e.pending[1:]
		released = append(released, adm.release)

		// A successful receipt makes its state durable.
		if adm.receipt.Err() == nil {
			e.stateCtr.SetValue(adm.state)
			e.stateProm.SetResult(adm.state, nil)
			continue
		}

		// A failed receipt takes its successors with it.
		if adm.dependent {
			continue
		}
		for _, later := range e.pending {
			later.dependent = true
		}
		e.admittedCtr.SetValue(e.stateCtr.GetValue())
	}
	return released
}

// releaseAll calls each release function.
func releaseAll(released []func()) {
	for _, release := range released {
		release()
	}
}
