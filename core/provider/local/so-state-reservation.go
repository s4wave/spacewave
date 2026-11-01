package provider_local

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/s4wave/spacewave/core/sobject"
)

// stateWriteReservationKey scopes a context claim to one state-functions owner.
type stateWriteReservationKey struct{ _ byte }

// soStateWriteReservation owns one retained state's write intent.
type soStateWriteReservation struct {
	// key identifies the persistence owner allowed to borrow this claim.
	key *stateWriteReservationKey
	// id identifies its retained SharedObject.
	id string
	// entry owns the admitted and durable state being prepared.
	entry *soStateEntry
	// settled records whether this intent has drained every preceding receipt.
	settled atomic.Bool
	// active refuses contexts retained after their owner returned the intent.
	active atomic.Bool
	// release returns both the intent and the retained entry reference.
	release func()
	// once makes repeated releases harmless.
	once sync.Once
}

// WithContext carries a live reservation to this provider's state writer.
func (r *soStateWriteReservation) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, r.key, r)
}

// Release returns the retained entry and write intent once.
func (r *soStateWriteReservation) Release() {
	r.once.Do(func() { r.active.Store(false); r.release() })
}

// borrowedStateWrite keeps the caller's intent until that caller releases it.
// Its synchronous state-lock user finishes before the owner calls Release.
type borrowedStateWrite struct{ *soStateWriteReservation }

// Release leaves the caller's reservation owned by that caller.
func (r borrowedStateWrite) Release() {}

// borrowStateWrite reuses a live claim and refuses an upgrade under a writer.
func borrowStateWrite(ctx context.Context, key *stateWriteReservationKey, id string, settled bool) (sobject.StateWriteReservation, error) {
	r, _ := ctx.Value(key).(*soStateWriteReservation)
	if r == nil || r.id != id || !r.active.Load() {
		return nil, nil
	}
	if settled && !r.settled.Load() {
		if _, pending := r.entry.readSettled(); pending != nil {
			return nil, sobject.ErrStateWriteUnsettled
		}
		r.settled.Store(true)
	}
	return borrowedStateWrite{r}, nil
}
