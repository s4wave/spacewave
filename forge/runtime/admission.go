// Package forge_runtime admits Forge Worker capacity on a Device and tracks the
// reservations that Executions hold against it.
//
// A Device advertises a forge-worker capability whose availability
// LookupDeviceAvailability reads and WatchDeviceAvailability watches. Before a
// Worker runs an Execution, admission reserves capacity on the Device. A
// reservation is:
//
//   - Reserved: capacity is debited and no runtime has claimed it.
//   - Active: one claimed runtime generation holds it.
//   - Uncertain: the outcome is unknown, as after a Device disconnect.
//   - PendingStop: the runtime is fenced and its stop is not yet confirmed.
//   - Released: cleanup is recorded and no capacity is held.
//
// Capacity stays debited until the same fenced runtime reconnects, its stop is
// confirmed, or the lease expires (DefaultLeaseDuration for a reservation,
// DefaultOwnerLeaseDuration for the owner's claim), so a lost Device neither
// leaks capacity forever nor lets it be spent twice.
package forge_runtime

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

// DefaultLeaseDuration is the reservation lease applied when the admission
// owner does not configure a duration.
const DefaultLeaseDuration = 10 * time.Minute

// DefaultOwnerLeaseDuration is the owner claim lease applied when the
// admission owner does not configure a duration.
const DefaultOwnerLeaseDuration = time.Minute

// WorkerClaimRef identifies one Worker execution's durable owner claim on a
// capacity record. The reference is presented by the caller and checked
// against the record's stored claim inside each write transaction.
type WorkerClaimRef struct {
	// DeviceObjectKey is the enrolled Device object key hosting the Worker.
	DeviceObjectKey string
	// ClaimID is the per-instance claim identifier. A new claim id on the same
	// Device replaces a previous instance after reclaim.
	ClaimID string
}

// Valid reports whether the state is a defined value.
func (s ReservationState) Valid() bool {
	_, ok := ReservationState_name[int32(s)]
	return ok
}

// Live reports whether the state may still transition before release.
func (s ReservationState) Live() bool {
	return !s.Terminal()
}

// Terminal reports whether the state releases capacity permanently.
func (s ReservationState) Terminal() bool {
	return s == ReservationState_RESERVATION_STATE_RELEASED
}

// ReservationOutcome classifies a reservation for reconciliation after a
// daemon restart or a Device reconnect.
type ReservationOutcome uint8

const (
	// OutcomeActive means the reservation holds capacity and custody is fenced.
	OutcomeActive ReservationOutcome = iota + 1
	// OutcomeUncertain means the runtime outcome is unknown: custody went
	// unreachable or a confirmed stop is still pending reconciliation.
	OutcomeUncertain
	// OutcomeTerminal means cleanup is recorded and no capacity is held.
	OutcomeTerminal
)

// Errors returned by runtime admission.
var (
	// ErrReservationNotFound is returned when a reservation object key is unknown.
	ErrReservationNotFound = errors.New("reservation not found")
	// ErrWorkerNotObserved is returned when a Worker has no observed capacity record.
	ErrWorkerNotObserved = errors.New("worker capacity not observed")
	// ErrStaleGeneration is returned when a call fences against an older
	// generation, for example a late return from a replaced runtime.
	ErrStaleGeneration = errors.New("stale reservation generation")
	// ErrCapacityExhausted is returned when a request fits a Worker's declared
	// totals but live reservations hold what it needs. A credit can admit it,
	// so the caller waits for one.
	ErrCapacityExhausted = errors.New("worker capacity exhausted")
	// ErrCapacityNeverFits is returned when a Worker's declared totals could
	// never hold a request, whatever is reserved, such as two cards on a
	// one-card Worker. Waiting would never end, so the caller fails at once.
	ErrCapacityNeverFits = errors.New("request exceeds worker capacity")
	// ErrBackendUnsupported is returned when a worker does not declare the backend.
	ErrBackendUnsupported = errors.New("backend unsupported by worker")
	// ErrReservationTerminal is returned when a lease renewal hits a released
	// reservation.
	ErrReservationTerminal = errors.New("reservation already released")
	// ErrRequestMismatch is returned when an existing reservation conflicts with the request.
	ErrRequestMismatch = errors.New("reservation request mismatch")
	// ErrReservationExpired is returned when an idempotent retry hits a
	// live reservation whose lease already expired but is not swept yet.
	// Run the expiry sweep. The retry then reserves the next generation.
	ErrReservationExpired = errors.New("reservation lease expired")
	// ErrCapacityOwned is returned when a different Device holds the live
	// owner claim on a capacity record.
	ErrCapacityOwned = errors.New("worker capacity claimed by another device")
	// ErrCapacityOwnerExpired is returned when the live claim's lease expired
	// without renewal or reclaim.
	ErrCapacityOwnerExpired = errors.New("worker capacity owner claim expired")
	// ErrCapacityDraining is returned when the record's claim is live but in
	// the draining state and cannot accept new work.
	ErrCapacityDraining = errors.New("worker capacity is draining")
)

// refusals are the errors that refuse a request without changing the ledger.
var refusals = [...]error{
	ErrCapacityExhausted,
	ErrCapacityNeverFits,
	ErrBackendUnsupported,
	ErrCapacityDraining,
	ErrCapacityOwnerExpired,
	ErrCapacityOwned,
}

// IsRefusal reports whether admission refused a request because the Worker
// cannot take it now: its capacity or backends do not fit, it is draining, or
// its owner claim expired or belongs to another Device. The ledger is
// unchanged, and a change to the capacity record the caller read wakes a retry.
func IsRefusal(err error) bool {
	for _, refusal := range refusals {
		if errors.Is(err, refusal) {
			return true
		}
	}
	return false
}

// RuntimeAdmission atomically reserves Worker capacity and reconciles fenced
// backend runtimes. Forge owns this boundary. Callers never keep their own
// capacity ledger.
type RuntimeAdmission interface {
	// Reserve atomically debits Worker capacity for one Execution attempt.
	// Reserve is idempotent per Execution object key while the reservation is
	// live. After release it reserves the next generation for the resumed
	// attempt, fencing calls from the released runtime.
	Reserve(ctx context.Context, workerObjectKey, executionObjectKey string, request *ResourceRequest) (*Reservation, error)
	// LookupReservation loads one persisted reservation. Reconcile after a
	// restart reads the same object and resumes observation without relaunch.
	LookupReservation(ctx context.Context, reservationObjectKey string) (*Reservation, error)
	// StopAndRelease stops the fenced runtime, credits capacity exactly
	// once, and returns the persisted cleanup facts. The caller must
	// present the live owner claim (ref and current owner epoch) of the
	// Worker's capacity record. A deposed or stale instance is rejected
	// before the stopper runs. A stale reservation generation is rejected
	// without touching the current runtime or capacity. Until the stop is
	// confirmed the reservation sits in the durable pending-stop state.
	StopAndRelease(ctx context.Context, ref WorkerClaimRef, ownerEpoch uint64, reservationObjectKey string, generation uint64) (*CleanupReceipt, error)
}
