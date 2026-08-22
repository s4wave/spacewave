package forge_runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/aperturerobotics/fastjson"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

const reservationObjectKeyPrefix = "forge/runtime/reservation/"

// BuildReservationObjectKey builds the deterministic object key for the one
// reservation of an Execution attempt. A retry after release is a new attempt
// with a new Execution object key.
func BuildReservationObjectKey(executionObjectKey string) string {
	hash := sha256.Sum256([]byte(executionObjectKey))
	return reservationObjectKeyPrefix + hex.EncodeToString(hash[:12])
}

// NewReservationBlock constructs an empty Reservation block.
func NewReservationBlock() block.Block {
	return &Reservation{}
}

// Reservation records one generation-fenced and lease-fenced capacity grant.
// The record is the durable truth: a daemon restart reconciles by reading it.
type Reservation struct {
	// WorkerObjectKey is the Forge Worker object key holding the capacity.
	WorkerObjectKey string `json:"workerObjectKey,omitempty"`
	// ExecutionObjectKey is the owning Execution attempt object key.
	ExecutionObjectKey string `json:"executionObjectKey,omitempty"`
	// Request is the reserved capacity request.
	Request ResourceRequest `json:"request"`
	// Generation fences runtime custody. Resume after uncertainty increments
	// the generation; calls fenced against an older generation are stale.
	Generation uint64
	// LeaseExpiresAt is when unobserved custody expires. Expiry releases the
	// debited capacity exactly once.
	LeaseExpiresAt *timestamp.Timestamp
	// State is the reservation lifecycle state.
	State ReservationState
	// Runtime identifies the claimed backend runtime, set on activation.
	Runtime BackendRuntimeIdentity
	// Cleanup records the terminal cleanup receipt, set on release.
	Cleanup *CleanupReceipt
}

// ObjectKey returns the deterministic reservation object key.
func (r *Reservation) ObjectKey() string {
	return BuildReservationObjectKey(r.ExecutionObjectKey)
}

// Validate validates the reservation.
func (r *Reservation) Validate() error {
	switch {
	case r.WorkerObjectKey == "":
		return errors.New("worker_object_key cannot be empty")
	case r.ExecutionObjectKey == "":
		return errors.New("execution_object_key cannot be empty")
	case r.Generation == 0:
		return errors.New("generation must be set")
	case !r.State.Valid():
		return errors.Errorf("invalid reservation state %d", r.State)
	case r.LeaseExpiresAt == nil:
		return errors.New("lease_expires_at must be set")
	}
	if err := r.Request.Validate(); err != nil {
		return errors.Wrap(err, "request")
	}
	if err := r.LeaseExpiresAt.Validate(false); err != nil {
		return errors.Wrap(err, "lease_expires_at")
	}
	if r.Cleanup != nil {
		if err := r.Cleanup.Validate(); err != nil {
			return errors.Wrap(err, "cleanup")
		}
		switch r.State {
		case ReservationStatePendingStop:
			if r.Cleanup.Complete() {
				return errors.New("pending-stop receipt must stay partial until the stop confirms")
			}
		case ReservationStateReleased:
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
	case r.State == ReservationStateUncertain:
		return OutcomeUncertain
	case r.LeaseExpired(now):
		return OutcomeUncertain
	default:
		return OutcomeActive
	}
}

// LeaseExpired reports whether the lease is past due at the given time.
func (r *Reservation) LeaseExpired(now time.Time) bool {
	return r.LeaseExpiresAt != nil && now.After(r.LeaseExpiresAt.AsTime())
}

// Reset resets the block.
func (r *Reservation) Reset() {
	*r = Reservation{}
}

// MarshalBlock marshals the block to binary.
func (r *Reservation) MarshalBlock() ([]byte, error) {
	return r.MarshalJSON()
}

// UnmarshalBlock unmarshals the block from binary.
func (r *Reservation) UnmarshalBlock(data []byte) error {
	return r.UnmarshalJSON(data)
}

// MarshalJSON marshals the Reservation to JSON without reflection.
func (r *Reservation) MarshalJSON() ([]byte, error) {
	var arena fastjson.Arena
	obj := arena.NewObject()
	setStringJSONField(&arena, obj, "workerObjectKey", r.WorkerObjectKey)
	setStringJSONField(&arena, obj, "executionObjectKey", r.ExecutionObjectKey)
	req := arena.NewObject()
	req.Set("milliCpu", arena.NewNumberString(strconv.FormatUint(r.Request.MilliCPU, 10)))
	req.Set("memoryBytes", arena.NewNumberString(strconv.FormatUint(r.Request.MemoryBytes, 10)))
	req.Set("backend", arena.NewString(r.Request.Backend))
	obj.Set("request", req)
	obj.Set("generation", arena.NewNumberString(strconv.FormatUint(r.Generation, 10)))
	tsValue, err := marshalTimestampField(&arena, r.LeaseExpiresAt)
	if err != nil {
		return nil, err
	}
	obj.Set("leaseExpiresAt", tsValue)
	obj.Set("state", arena.NewNumberInt(int(r.State)))
	if !r.Runtime.IsZero() {
		rt := arena.NewObject()
		rt.Set("backend", arena.NewString(r.Runtime.Backend))
		rt.Set("id", arena.NewString(r.Runtime.ID))
		obj.Set("runtime", rt)
	}
	if r.Cleanup != nil {
		cleanupValue, err := r.Cleanup.marshalJSONValue(&arena)
		if err != nil {
			return nil, err
		}
		obj.Set("cleanup", cleanupValue)
	}
	return obj.MarshalTo(nil), nil
}

// UnmarshalJSON unmarshals the Reservation from JSON without reflection.
func (r *Reservation) UnmarshalJSON(data []byte) error {
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		return err
	}
	if value.Type() == fastjson.TypeNull {
		*r = Reservation{}
		return nil
	}
	if value.Type() != fastjson.TypeObject {
		return errors.New("reservation must be object")
	}
	r.WorkerObjectKey = string(value.GetStringBytes("workerObjectKey"))
	r.ExecutionObjectKey = string(value.GetStringBytes("executionObjectKey"))
	r.Request = ResourceRequest{
		MilliCPU:    value.GetUint64("request", "milliCpu"),
		MemoryBytes: value.GetUint64("request", "memoryBytes"),
		Backend:     string(value.GetStringBytes("request", "backend")),
	}
	r.Generation = value.GetUint64("generation")
	r.State = ReservationState(value.GetInt("state"))
	if tsValue := value.Get("leaseExpiresAt"); tsValue != nil && tsValue.Type() != fastjson.TypeNull {
		ts := &timestamp.Timestamp{}
		if err := ts.UnmarshalJSON(tsValue.MarshalTo(nil)); err != nil {
			return errors.Wrap(err, "unmarshal lease_expires_at")
		}
		r.LeaseExpiresAt = ts
	}
	if rt := value.Get("runtime"); rt != nil && rt.Type() == fastjson.TypeObject {
		r.Runtime = BackendRuntimeIdentity{
			Backend: string(rt.GetStringBytes("backend")),
			ID:      string(rt.GetStringBytes("id")),
		}
	}
	if cleanupValue := value.Get("cleanup"); cleanupValue != nil && cleanupValue.Type() == fastjson.TypeObject {
		cleanup := &CleanupReceipt{}
		if err := cleanup.unmarshalJSONValue(cleanupValue); err != nil {
			return errors.Wrap(err, "unmarshal cleanup")
		}
		r.Cleanup = cleanup
	}
	return nil
}

// LookupReservation loads one persisted Reservation or ErrReservationNotFound.
// The loaded record is validated before use.
func LookupReservation(ctx context.Context, ws world.WorldState, objKey string) (*Reservation, error) {
	res, err := world.LookupObjectBody[*Reservation](ctx, ws, objKey, NewReservationBlock)
	if errors.Is(err, world.ErrObjectNotFound) {
		return nil, ErrReservationNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := res.Validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid reservation %s", objKey)
	}
	return res, nil
}

// _ is a type assertion
var _ block.Block = (*Reservation)(nil)
