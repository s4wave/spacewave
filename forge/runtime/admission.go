package forge_runtime

import (
	"context"
	"time"

	"github.com/pkg/errors"
)

// DefaultLeaseDuration is the reservation lease applied when the admission
// owner does not configure a duration.
const DefaultLeaseDuration = 10 * time.Minute

// ReservationState describes whether capacity remains held and whether the
// runtime outcome is known.
type ReservationState uint8

const (
	// ReservationStateReserved holds debited capacity before a runtime claims it.
	ReservationStateReserved ReservationState = iota + 1
	// ReservationStateActive holds debited capacity for one claimed runtime generation.
	ReservationStateActive
	// ReservationStateUncertain holds debited capacity while the runtime outcome
	// is unknown, for example after a Device disconnect. Capacity stays debited
	// until the same fenced runtime reconnects or the lease expires.
	ReservationStateUncertain
	// ReservationStatePendingStop holds a fenced runtime whose stop has not
	// been confirmed yet. Capacity release follows the expiry rule while the
	// runtime outcome stays unknown until the stop is confirmed.
	ReservationStatePendingStop
	// ReservationStateReleased is terminal: cleanup is recorded and no
	// capacity is held.
	ReservationStateReleased
)

// Valid reports whether the state is a defined value.
func (s ReservationState) Valid() bool {
	return s >= ReservationStateReserved && s <= ReservationStateReleased
}

// Live reports whether the state may still transition before release.
func (s ReservationState) Live() bool {
	return !s.Terminal()
}

// Terminal reports whether the state releases capacity permanently.
func (s ReservationState) Terminal() bool {
	return s == ReservationStateReleased
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

// ResourceRequest declares the host capacity and backend required by one execution attempt.
type ResourceRequest struct {
	// MilliCPU is the requested CPU in milli-cores.
	MilliCPU uint64
	// MemoryBytes is the requested memory in bytes.
	MemoryBytes uint64
	// Backend names the runtime backend required by the attempt.
	Backend string
}

// Validate validates the request.
func (r ResourceRequest) Validate() error {
	switch {
	case r.MilliCPU == 0:
		return errors.New("milli_cpu must be set")
	case r.MemoryBytes == 0:
		return errors.New("memory_bytes must be set")
	case r.Backend == "":
		return errors.New("backend must be set")
	}
	return nil
}

// BackendRuntimeIdentity identifies one backend runtime instance for one
// reservation generation. The identity is stable across daemon restarts so
// reconcile can resume observation without another launch.
type BackendRuntimeIdentity struct {
	// Backend names the runtime backend that owns the runtime.
	Backend string
	// ID is the backend-scoped runtime identifier, for example a container id.
	ID string
}

// IsZero reports whether the identity is unset.
func (i BackendRuntimeIdentity) IsZero() bool {
	return i.Backend == "" && i.ID == ""
}

// CleanupReceipt records the terminal cleanup facts for one reservation generation.
type CleanupReceipt struct {
	// ReservationObjectKey is the released reservation object key.
	ReservationObjectKey string
	// ExecutionObjectKey is the owning Execution object key.
	ExecutionObjectKey string
	// RuntimeIdentity is the stopped runtime identity, empty when no runtime launched.
	RuntimeIdentity string
	// Generation is the fenced generation the receipt applies to.
	Generation uint64
	// RuntimeStopped records that the backend runtime was confirmed stopped,
	// or that no runtime ever launched. It stays false while a stop is pending;
	// the receipt never fabricates this fact.
	RuntimeStopped bool
	// CapacityReleased records that reserved capacity was credited back exactly once.
	CapacityReleased bool
	// Reason records why the reservation released: "stopped" or "expired".
	Reason string
}

// Complete reports whether every cleanup fact is recorded.
func (r *CleanupReceipt) Complete() bool {
	return r != nil && r.RuntimeStopped && r.CapacityReleased
}

// Validate validates the receipt.
func (r *CleanupReceipt) Validate() error {
	if r == nil {
		return errors.New("cleanup receipt cannot be nil")
	}
	switch {
	case r.ReservationObjectKey == "":
		return errors.New("reservation_object_key cannot be empty")
	case r.ExecutionObjectKey == "":
		return errors.New("execution_object_key cannot be empty")
	case r.Generation == 0:
		return errors.New("generation must be set")
	case r.Reason == "":
		return errors.New("reason must be set")
	case r.CapacityReleased != r.RuntimeStopped:
		return errors.New("receipt must be partial (nothing released, stop unknown) or complete (stop confirmed and capacity credited)")
	}
	return nil
}

// Errors returned by runtime admission.
var (
	// ErrReservationNotFound is returned when a reservation object key is unknown.
	ErrReservationNotFound = errors.New("reservation not found")
	// ErrWorkerNotObserved is returned when a Worker has no observed capacity record.
	ErrWorkerNotObserved = errors.New("worker capacity not observed")
	// ErrStaleGeneration is returned when a call fences against an older
	// generation, for example a late return from a replaced runtime.
	ErrStaleGeneration = errors.New("stale reservation generation")
	// ErrCapacityExhausted is returned when a worker cannot satisfy a request.
	ErrCapacityExhausted = errors.New("worker capacity exhausted")
	// ErrWorkerClaimHeld is returned when another live admission owner
	// instance holds the single-writer claim on one Worker's capacity record.
	ErrWorkerClaimHeld = errors.New("worker capacity claimed by another instance")
	// ErrWorkerNotClaimed is returned when the calling instance holds no live
	// claim on one Worker's capacity record.
	ErrWorkerNotClaimed = errors.New("worker capacity not claimed by this instance")
	// ErrWorkerDraining is returned when a Worker's capacity record is
	// draining and cannot accept new reservations.
	ErrWorkerDraining = errors.New("worker capacity is draining")
	// ErrDrainIncomplete is returned when a drain cannot complete because
	// live reservations still debit the Worker's capacity.
	ErrDrainIncomplete = errors.New("drain incomplete: reservations remain")
	// ErrBackendUnsupported is returned when a worker does not declare the backend.
	ErrBackendUnsupported = errors.New("backend unsupported by worker")
	// ErrReservationTerminal is returned when an idempotent retry hits a
	// released reservation; a retry after release is a new attempt with a new
	// Execution object key.
	ErrReservationTerminal = errors.New("reservation already released")
	// ErrRequestMismatch is returned when an existing reservation conflicts with the request.
	ErrRequestMismatch = errors.New("reservation request mismatch")
	// ErrReservationExpired is returned when an idempotent retry hits a live
	// reservation whose lease already expired but is not swept yet. Run the
	// expiry sweep; the retry then requires a new attempt.
	ErrReservationExpired = errors.New("reservation lease expired")
)

// Cleanup reasons recorded on receipts.
const (
	// CleanupReasonStop records a caller-requested stop of a live runtime.
	CleanupReasonStop = "stopped"
	// CleanupReasonExpired records a lease-expiry release.
	CleanupReasonExpired = "expired"
)

// RuntimeAdmission atomically reserves Worker capacity and reconciles fenced
// backend runtimes. Forge owns this boundary; callers never keep their own
// capacity ledger.
type RuntimeAdmission interface {
	// Reserve atomically debits Worker capacity for one Execution attempt.
	// Reserve is idempotent per Execution object key while the reservation is
	// live; a released reservation requires a new attempt.
	Reserve(ctx context.Context, workerObjectKey, executionObjectKey string, request ResourceRequest) (*Reservation, error)
	// LookupReservation loads one persisted reservation. Reconcile after a
	// restart reads the same object and resumes observation without relaunch.
	LookupReservation(ctx context.Context, reservationObjectKey string) (*Reservation, error)
	// StopAndRelease stops the fenced runtime, credits capacity exactly once,
	// and returns the persisted cleanup facts. A stale generation is rejected
	// without touching the current runtime or capacity. Until the stop is
	// confirmed the reservation sits in the durable pending-stop state.
	StopAndRelease(ctx context.Context, reservationObjectKey string, generation uint64) (*CleanupReceipt, error)
}
