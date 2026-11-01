package sobject

import (
	"context"
	"errors"
)

// ErrStateWriteUnsettled asks a caller to release its writers and reservation,
// reserve the durable state, then retry a publication the store declined.
var ErrStateWriteUnsettled = errors.New("state write needs settled preparation")

// StateWriteReservation orders state-write intent before engine/provider writers.
// A settled reservation also drains prior receipts before returning. Join every
// operation using WithContext before Release; callers must not upgrade an active
// admitted-state reservation while holding an engine writer.
type StateWriteReservation interface {
	// WithContext carries this reservation into the provider state lock.
	WithContext(context.Context) context.Context
	// Release returns write intent once; repeated calls are harmless.
	Release()
}

// StateWriteReserver orders optional state preparation independently of block publication.
type StateWriteReserver interface {
	// ReserveStateWrite waits for intent and optionally for prior receipts.
	ReserveStateWrite(context.Context, bool) (StateWriteReservation, error)
	// TryReserveStateWrite claims admitted-state intent without waiting.
	TryReserveStateWrite(context.Context, bool) (StateWriteReservation, error)
}

// TryReserveStateWrite claims intent without waiting, requiring settled receipts
// when asked. A nil claim
// asks a reservation-enabled caller to retry outside its engine writer.
func (s *SOHost) TryReserveStateWrite(ctx context.Context, settled bool) (StateWriteReservation, error) {
	if s.syncFuncs.TryReserveWrite == nil {
		return nil, nil
	}
	return s.syncFuncs.TryReserveWrite(ctx, s.sharedObjectID, settled)
}

// ReserveStateWrite claims the provider's optional write intent. Call it before
// taking an engine writer; settled waits for every preceding state receipt.
func (s *SOHost) ReserveStateWrite(ctx context.Context, settled bool) (StateWriteReservation, error) {
	if s.syncFuncs.ReserveWrite == nil {
		return nil, nil
	}
	return s.syncFuncs.ReserveWrite(ctx, s.sharedObjectID, settled)
}
