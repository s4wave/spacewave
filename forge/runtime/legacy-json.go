package forge_runtime

import (
	"context"

	"github.com/aperturerobotics/fastjson"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/world"
)

// Releases before the protobuf records stored WorkerCapacity and Reservation
// bodies as JSON objects. A stored JSON body still decodes, and the next write
// of the record stores it as protobuf, so a Worker that ran an older release
// keeps its claim and its reservations. A protobuf body never starts with '{':
// that byte is the tag of a group field, which these messages do not use.
//
// Delete this file once no Space holds a JSON capacity or reservation body.

// lookupRecord loads one capacity or reservation body into a new value,
// decoding a legacy JSON body with legacy. It returns world.ErrObjectNotFound
// when the object does not exist.
func lookupRecord[T any, PT interface {
	*T
	UnmarshalVT([]byte) error
}](
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	legacy func(PT, *fastjson.Value) error,
) (PT, error) {
	// Read the stored body and report a missing object.
	data, found, err := world.LookupObjectBodyBytes(ctx, ws, objKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
	}

	// Decode protobuf, or the legacy JSON object an older release wrote.
	out := PT(new(T))
	if len(data) == 0 || data[0] != '{' {
		return out, out.UnmarshalVT(data)
	}
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		return nil, errors.Wrap(err, "legacy json record")
	}
	return out, legacy(out, value)
}

// unmarshalLegacyWorkerCapacity restores a capacity record from its JSON form.
func unmarshalLegacyWorkerCapacity(w *WorkerCapacity, value *fastjson.Value) error {
	// Restore the observed and reserved totals.
	w.WorkerObjectKey = string(value.GetStringBytes("workerObjectKey"))
	w.MilliCpuTotal = value.GetUint64("milliCpuTotal")
	w.MilliCpuReserved = value.GetUint64("milliCpuReserved")
	w.MemoryBytesTotal = value.GetUint64("memoryBytesTotal")
	w.MemoryBytesReserved = value.GetUint64("memoryBytesReserved")

	// Restore the owner claim and the supported backends.
	w.OwnerDeviceObjectKey = string(value.GetStringBytes("ownerDeviceObjectKey"))
	w.ClaimId = string(value.GetStringBytes("claimId"))
	w.OwnerEpoch = value.GetUint64("ownerEpoch")
	w.OwnerState = CapacityOwnerState(value.GetInt("ownerState")) //nolint:gosec // Validate rejects an undefined state.
	w.Generation = value.GetUint64("generation")
	for _, b := range value.GetArray("backends") {
		w.Backends = append(w.Backends, string(b.GetStringBytes()))
	}

	// Restore the lease and observation timestamps.
	var err error
	if w.OwnerLeaseExpiresAt, err = legacyTimestamp(value, "ownerLeaseExpiresAt"); err != nil {
		return err
	}
	w.ObservedAt, err = legacyTimestamp(value, "observedAt")
	return err
}

// unmarshalLegacyReservation restores a reservation from its JSON form.
func unmarshalLegacyReservation(r *Reservation, value *fastjson.Value) error {
	// Restore the identity, request, generation and state.
	r.WorkerObjectKey = string(value.GetStringBytes("workerObjectKey"))
	r.ExecutionObjectKey = string(value.GetStringBytes("executionObjectKey"))
	r.Request = &ResourceRequest{
		MilliCpu:    value.GetUint64("request", "milliCpu"),
		MemoryBytes: value.GetUint64("request", "memoryBytes"),
		Backend:     string(value.GetStringBytes("request", "backend")),
	}
	r.Generation = value.GetUint64("generation")
	r.State = ReservationState(value.GetInt("state")) //nolint:gosec // Validate rejects an undefined state.
	var err error
	if r.LeaseExpiresAt, err = legacyTimestamp(value, "leaseExpiresAt"); err != nil {
		return err
	}

	// Restore the standing grant's placement edge.
	if placement := value.Get("placement"); placement != nil {
		r.Placement = &quad.Quad{}
		if err := r.Placement.UnmarshalJSON(placement.MarshalTo(nil)); err != nil {
			return errors.Wrap(err, "placement")
		}
	}

	// Restore the backend runtime custody.
	if rt := value.Get("runtime"); rt != nil && rt.Type() == fastjson.TypeObject {
		r.Runtime = &BackendRuntimeIdentity{
			Backend:            string(rt.GetStringBytes("backend")),
			Id:                 string(rt.GetStringBytes("id")),
			StopCommand:        string(rt.GetStringBytes("stopCommand")),
			StopTimeoutSeconds: uint32(rt.GetUint("stopTimeoutSeconds")), //nolint:gosec // a stop timeout never nears 2^32 seconds.
		}
		for _, entry := range rt.GetArray("stopEnv") {
			r.Runtime.StopEnv = append(r.Runtime.StopEnv, string(entry.GetStringBytes()))
		}
	}

	// Restore the cleanup receipt, whose reason was a string.
	if c := value.Get("cleanup"); c != nil && c.Type() == fastjson.TypeObject {
		r.Cleanup = &CleanupReceipt{
			ReservationObjectKey: string(c.GetStringBytes("reservationObjectKey")),
			ExecutionObjectKey:   string(c.GetStringBytes("executionObjectKey")),
			RuntimeIdentity:      string(c.GetStringBytes("runtimeIdentity")),
			Generation:           c.GetUint64("generation"),
			RuntimeStopped:       c.GetBool("runtimeStopped"),
			CapacityReleased:     c.GetBool("capacityReleased"),
			Reason:               CleanupReason_CLEANUP_REASON_STOPPED,
		}
		if string(c.GetStringBytes("reason")) == "expired" {
			r.Cleanup.Reason = CleanupReason_CLEANUP_REASON_EXPIRED
		}
	}
	return nil
}

// legacyTimestamp decodes an optional JSON timestamp field.
func legacyTimestamp(value *fastjson.Value, key string) (*timestamp.Timestamp, error) {
	// An absent or null field is no timestamp.
	tsValue := value.Get(key)
	if tsValue == nil || tsValue.Type() == fastjson.TypeNull {
		return nil, nil
	}

	// Decode the RFC 3339 string the protobuf JSON form uses.
	ts := &timestamp.Timestamp{}
	if err := ts.UnmarshalJSON(tsValue.MarshalTo(nil)); err != nil {
		return nil, errors.Wrap(err, key)
	}
	return ts, nil
}
