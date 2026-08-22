package forge_runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strconv"
	"time"

	"github.com/aperturerobotics/fastjson"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

const workerCapacityObjectKeyPrefix = "forge/runtime/capacity/"

// BuildWorkerCapacityObjectKey builds the deterministic capacity object key
// for one Forge Worker. The Worker daemon owns this record; Forge validates
// and debits it, callers never keep a parallel ledger.
func BuildWorkerCapacityObjectKey(workerObjectKey string) string {
	hash := sha256.Sum256([]byte(workerObjectKey))
	return workerCapacityObjectKeyPrefix + hex.EncodeToString(hash[:12])
}

// NewWorkerCapacityBlock constructs an empty WorkerCapacity block.
func NewWorkerCapacityBlock() block.Block {
	return &WorkerCapacity{}
}

// CapacityOwnerState describes the lifecycle state of a capacity record's
// durable owner claim.
type CapacityOwnerState uint8

const (
	// CapacityOwnerStateUnspecified is the zero value: no live owner claim.
	// Legacy records written before owner claims decode with this state and
	// stay unavailable to every gated operation.
	CapacityOwnerStateUnspecified CapacityOwnerState = iota
	// CapacityOwnerStateActive means the claim is live and admits new work.
	CapacityOwnerStateActive
	// CapacityOwnerStateDraining means the claim is live but blocks new work
	// until reserved debits clear or declared totals grow to fit them.
	CapacityOwnerStateDraining
)

// Valid reports whether the state is a defined value.
func (s CapacityOwnerState) Valid() bool {
	return s <= CapacityOwnerStateDraining
}

// WorkerCapacity is the observed and reserved capacity for one Forge Worker.
type WorkerCapacity struct {
	// WorkerObjectKey is the Forge Worker object key this record describes.
	// The record's own object key is a one-way hash of this value, so scans
	// and reclaim need it stored to name the Worker.
	WorkerObjectKey string
	// MilliCPUTotal is the observed total CPU in milli-cores.
	MilliCPUTotal uint64
	// MilliCPUReserved is the CPU currently debited by live reservations.
	MilliCPUReserved uint64
	// MemoryBytesTotal is the observed total memory in bytes.
	MemoryBytesTotal uint64
	// MemoryBytesReserved is the memory currently debited by live reservations.
	MemoryBytesReserved uint64
	// Backends lists the runtime backends the Worker supports.
	Backends []string
	// ObservedAt is when the Worker reported the totals.
	ObservedAt *timestamp.Timestamp
	// Generation increments on every mutation of this record so observers can
	// fence stale capacity views.
	Generation uint64
	// OwnerDeviceObjectKey is the enrolled Device object key of the owning
	// daemon instance. Empty together with every other owner field marks a
	// legacy ownerless record.
	OwnerDeviceObjectKey string
	// ClaimID identifies one owning daemon instance claim.
	ClaimID string
	// OwnerEpoch increments on every claim transition; calls fencing against
	// an older epoch are stale.
	OwnerEpoch uint64
	// OwnerLeaseExpiresAt is when an unrenewed owner claim expires.
	OwnerLeaseExpiresAt *timestamp.Timestamp
	// OwnerState is the claim lifecycle state.
	OwnerState CapacityOwnerState
}

// SupportsBackend reports whether the Worker declares the backend.
func (w *WorkerCapacity) SupportsBackend(backend string) bool {
	return slices.Contains(w.Backends, backend)
}

// owned reports whether the record carries any owner-claim field.
func (w *WorkerCapacity) owned() bool {
	return w.OwnerDeviceObjectKey != "" || w.ClaimID != "" ||
		w.WorkerObjectKey != "" || w.OwnerEpoch != 0 ||
		w.OwnerLeaseExpiresAt != nil || w.OwnerState != CapacityOwnerStateUnspecified
}

// Validate validates the capacity record. A record carries either no owner
// fields at all (legacy ownerless shape) or all six; partial owner shapes are
// invalid so a half-written claim can never decode as available.
func (w *WorkerCapacity) Validate() error {
	// An ACTIVE record never over-commits; a DRAINING record may hold debits
	// above shrunk declared totals until credits land or the claim deletes it.
	if w.OwnerState == CapacityOwnerStateActive {
		switch {
		case w.MilliCPUReserved > w.MilliCPUTotal:
			return errors.New("reserved milli_cpu exceeds total")
		case w.MemoryBytesReserved > w.MemoryBytesTotal:
			return errors.New("reserved memory_bytes exceeds total")
		}
	}
	if w.Generation == 0 {
		return errors.New("generation must be set")
	}
	if err := w.ObservedAt.Validate(false); err != nil {
		return errors.Wrap(err, "observed_at")
	}
	if !w.owned() {
		return nil
	}
	switch {
	case w.WorkerObjectKey == "":
		return errors.New("worker_object_key must be set when owned")
	case w.OwnerDeviceObjectKey == "":
		return errors.New("owner_device_object_key must be set when owned")
	case w.ClaimID == "":
		return errors.New("claim_id must be set when owned")
	case w.OwnerEpoch == 0:
		return errors.New("owner_epoch must be set when owned")
	case w.OwnerState == CapacityOwnerStateUnspecified || !w.OwnerState.Valid():
		return errors.New("invalid owner_state")
	}
	if err := w.OwnerLeaseExpiresAt.Validate(false); err != nil {
		return errors.Wrap(err, "owner_lease_expires_at")
	}
	return nil
}

// OwnerClaimActive reports whether the record carries a live owner claim at
// the given time: owned, lease unexpired, and state ACTIVE. It returns typed
// sentinel errors so callers can distinguish unavailable, expired, and
// draining records without string matching.
func (w *WorkerCapacity) OwnerClaimActive(now time.Time) error {
	switch {
	case !w.owned():
		return ErrCapacityUnowned
	case w.OwnerLeaseExpiresAt == nil || !now.Before(w.OwnerLeaseExpiresAt.AsTime()):
		return ErrCapacityOwnerExpired
	case w.OwnerState != CapacityOwnerStateActive:
		return ErrCapacityDraining
	}
	return nil
}

// Reset resets the block.
func (w *WorkerCapacity) Reset() {
	*w = WorkerCapacity{}
}

// MarshalBlock marshals the block to binary.
func (w *WorkerCapacity) MarshalBlock() ([]byte, error) {
	return w.MarshalJSON()
}

// UnmarshalBlock unmarshals the block from binary.
func (w *WorkerCapacity) UnmarshalBlock(data []byte) error {
	return w.UnmarshalJSON(data)
}

// MarshalJSON marshals the WorkerCapacity to JSON without reflection.
func (w *WorkerCapacity) MarshalJSON() ([]byte, error) {
	var arena fastjson.Arena
	obj := arena.NewObject()
	setStringJSONField(&arena, obj, "workerObjectKey", w.WorkerObjectKey)
	setStringJSONField(&arena, obj, "ownerDeviceObjectKey", w.OwnerDeviceObjectKey)
	setStringJSONField(&arena, obj, "claimId", w.ClaimID)
	obj.Set("ownerEpoch", arena.NewNumberString(strconv.FormatUint(w.OwnerEpoch, 10)))
	ownerLease, err := marshalTimestampField(&arena, w.OwnerLeaseExpiresAt)
	if err != nil {
		return nil, err
	}
	obj.Set("ownerLeaseExpiresAt", ownerLease)
	obj.Set("ownerState", arena.NewNumberInt(int(w.OwnerState)))
	obj.Set("milliCpuTotal", arena.NewNumberString(strconv.FormatUint(w.MilliCPUTotal, 10)))
	obj.Set("milliCpuReserved", arena.NewNumberString(strconv.FormatUint(w.MilliCPUReserved, 10)))
	obj.Set("memoryBytesTotal", arena.NewNumberString(strconv.FormatUint(w.MemoryBytesTotal, 10)))
	obj.Set("memoryBytesReserved", arena.NewNumberString(strconv.FormatUint(w.MemoryBytesReserved, 10)))
	backends := arena.NewArray()
	for i, b := range w.Backends {
		backends.SetArrayItem(i, arena.NewString(b))
	}
	obj.Set("backends", backends)
	tsValue, err := marshalTimestampField(&arena, w.ObservedAt)
	if err != nil {
		return nil, err
	}
	obj.Set("observedAt", tsValue)
	obj.Set("generation", arena.NewNumberString(strconv.FormatUint(w.Generation, 10)))
	return obj.MarshalTo(nil), nil
}

// UnmarshalJSON unmarshals the WorkerCapacity from JSON without reflection.
func (w *WorkerCapacity) UnmarshalJSON(data []byte) error {
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		return err
	}
	if value.Type() == fastjson.TypeNull {
		*w = WorkerCapacity{}
		return nil
	}
	if value.Type() != fastjson.TypeObject {
		return errors.New("worker capacity must be object")
	}
	w.MilliCPUTotal = value.GetUint64("milliCpuTotal")
	w.MilliCPUReserved = value.GetUint64("milliCpuReserved")
	w.MemoryBytesTotal = value.GetUint64("memoryBytesTotal")
	w.MemoryBytesReserved = value.GetUint64("memoryBytesReserved")
	w.WorkerObjectKey = string(value.GetStringBytes("workerObjectKey"))
	w.OwnerDeviceObjectKey = string(value.GetStringBytes("ownerDeviceObjectKey"))
	w.ClaimID = string(value.GetStringBytes("claimId"))
	w.OwnerEpoch = value.GetUint64("ownerEpoch")
	w.OwnerLeaseExpiresAt, err = unmarshalTimestampField(value, "ownerLeaseExpiresAt")
	if err != nil {
		return err
	}
	w.OwnerState = CapacityOwnerState(value.GetInt("ownerState"))
	w.Backends = nil
	for _, b := range value.GetArray("backends") {
		w.Backends = append(w.Backends, string(b.GetStringBytes()))
	}
	w.ObservedAt, err = unmarshalTimestampField(value, "observedAt")
	if err != nil {
		return err
	}
	w.Generation = value.GetUint64("generation")
	return nil
}

// LookupWorkerCapacity loads one Worker capacity record or ErrWorkerNotObserved.
func LookupWorkerCapacity(ctx context.Context, ws world.WorldState, workerObjectKey string) (*WorkerCapacity, error) {
	capacity, err := world.LookupObjectBody[*WorkerCapacity](
		ctx,
		ws,
		BuildWorkerCapacityObjectKey(workerObjectKey),
		NewWorkerCapacityBlock,
	)
	if errors.Is(err, world.ErrObjectNotFound) {
		return nil, ErrWorkerNotObserved
	}
	if err != nil {
		return nil, err
	}
	return capacity, nil
}

var _ block.Block = (*WorkerCapacity)(nil)
