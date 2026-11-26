package forge_runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

const reservationObjectKeyPrefix = "forge/runtime/reservation/"

// BuildReservationObjectKey builds the deterministic reservation key for a
// capacity holder. Execution retries and standing node claims reuse the key
// at the next reservation generation.
func BuildReservationObjectKey(executionObjectKey string) string {
	hash := sha256.Sum256([]byte(executionObjectKey))
	return reservationObjectKeyPrefix + hex.EncodeToString(hash[:12])
}

// ObjectKey returns the deterministic reservation object key.
func (r *Reservation) ObjectKey() string {
	return BuildReservationObjectKey(r.ExecutionObjectKey)
}

// Validate validates the reservation.
func (r *Reservation) Validate() error {
	// Validate the reservation identity, generation and lifecycle fields.
	switch {
	case r.WorkerObjectKey == "":
		return errors.New("worker_object_key cannot be empty")
	case r.ExecutionObjectKey == "":
		return errors.New("execution_object_key cannot be empty")
	case r.Generation == 0:
		return errors.New("generation must be set")
	case r.State == ReservationState_RESERVATION_STATE_UNKNOWN || !r.State.Valid():
		return errors.Errorf("invalid reservation state %d", r.State)
	case r.LeaseExpiresAt == nil:
		return errors.New("lease_expires_at must be set")
	}
	if err := r.GetRequest().Validate(); err != nil && r.Placement == nil {
		// Validate the reserved resources and custody lease.
		return errors.Wrap(err, "request")
	}
	if r.Placement != nil && (r.GetRequest().GetBackend() == "" || !r.GetRuntime().IsZero()) {
		return errors.New("placement grants require a backend and cannot hold runtime custody")
	}
	if err := r.LeaseExpiresAt.Validate(false); err != nil {
		return errors.Wrap(err, "lease_expires_at")
	}

	// Require one distinct card id for each card the request asks for.
	if len(r.GetCardIds()) != int(r.GetRequest().GetCards().GetCount()) {
		return errors.Errorf("holds %d cards for a request of %d", len(r.GetCardIds()), r.GetRequest().GetCards().GetCount())
	}
	seen := make(map[string]struct{}, len(r.GetCardIds()))
	for _, id := range r.GetCardIds() {
		if _, dup := seen[id]; dup || id == "" {
			return errors.Errorf("invalid or repeated card id %q", id)
		}
		seen[id] = struct{}{}
	}
	if r.Cleanup != nil {
		// Require the cleanup receipt to agree with the reservation state.
		if err := r.Cleanup.Validate(); err != nil {
			return errors.Wrap(err, "cleanup")
		}
		switch r.State {
		case ReservationState_RESERVATION_STATE_PENDING_STOP:
			if r.Cleanup.Complete() {
				return errors.New("pending-stop receipt must stay partial until the stop confirms")
			}
		case ReservationState_RESERVATION_STATE_RELEASED:
			if !r.Cleanup.Complete() {
				return errors.New("released reservation requires the completed receipt")
			}
		default:
			return errors.New("cleanup receipt requires pending-stop or released state")
		}
	}
	return nil
}

// Outcome classifies the reservation for reconcile at the given time.
func (r *Reservation) Outcome(now time.Time) ReservationOutcome {
	switch {
	case r.State.Terminal():
		return OutcomeTerminal
	case r.State == ReservationState_RESERVATION_STATE_UNCERTAIN:
		return OutcomeUncertain
	case r.LeaseExpired(now):
		return OutcomeUncertain
	default:
		return OutcomeActive
	}
}

// LeaseExpired reports whether runtime custody is past due. Standing grants
// follow their placement edge rather than a runtime lease.
func (r *Reservation) LeaseExpired(now time.Time) bool {
	return r.Placement == nil && r.LeaseExpiresAt != nil && now.After(r.LeaseExpiresAt.AsTime())
}

// LookupReservation loads one persisted Reservation or ErrReservationNotFound.
// The loaded record is validated before use.
func LookupReservation(ctx context.Context, ws world.WorldState, objKey string) (*Reservation, error) {
	// Load the reservation and translate a missing World object.
	res, err := lookupRecord(ctx, ws, objKey, unmarshalLegacyReservation)
	if errors.Is(err, world.ErrObjectNotFound) {
		return nil, ErrReservationNotFound
	}
	if err != nil {
		return nil, err
	}

	// Validate the persisted reservation before returning it.
	if err := res.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid reservation %s", objKey)
	}
	return res, nil
}

// _ is a type assertion
var _ block.Block = (*Reservation)(nil)
