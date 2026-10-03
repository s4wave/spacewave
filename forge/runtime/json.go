package forge_runtime

import (
	"strconv"

	"github.com/aperturerobotics/fastjson"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
)

func setBoolJSONField(arena *fastjson.Arena, obj *fastjson.Value, key string, value bool) {
	if value {
		obj.Set(key, arena.NewTrue())
	} else {
		obj.Set(key, arena.NewFalse())
	}
}

func setStringJSONField(arena *fastjson.Arena, obj *fastjson.Value, key, value string) {
	if value != "" {
		obj.Set(key, arena.NewString(value))
	}
}

func marshalTimestampField(arena *fastjson.Arena, ts *timestamp.Timestamp) (*fastjson.Value, error) {
	// Encode the timestamp before copying its parsed JSON into the caller arena.
	if ts == nil {
		return arena.NewNull(), nil
	}
	tsJSON, err := ts.MarshalJSON()
	if err != nil {
		return nil, errors.Wrap(err, "marshal timestamp")
	}
	var parser fastjson.Parser

	// Parse the timestamp JSON and report malformed timestamp output.
	value, err := parser.ParseBytes(tsJSON)
	if err != nil {
		return nil, errors.Wrap(err, "parse timestamp")
	}
	return arena.DeepCopyValue(value), nil
}

func unmarshalTimestampField(value *fastjson.Value, key string) (*timestamp.Timestamp, error) {
	// Decode the optional timestamp field into a typed timestamp.
	tsValue := value.Get(key)
	if tsValue == nil || tsValue.Type() == fastjson.TypeNull {
		return nil, nil
	}
	ts := &timestamp.Timestamp{}
	if err := ts.UnmarshalJSON(tsValue.MarshalTo(nil)); err != nil {
		return nil, errors.Wrap(err, "unmarshal "+key)
	}
	return ts, nil
}

// marshalJSONValue marshals the receipt into the arena.
func (r *CleanupReceipt) marshalJSONValue(arena *fastjson.Arena) (*fastjson.Value, error) {
	// Encode cleanup identities and generation into a JSON receipt.
	obj := arena.NewObject()
	setStringJSONField(arena, obj, "reservationObjectKey", r.ReservationObjectKey)
	setStringJSONField(arena, obj, "executionObjectKey", r.ExecutionObjectKey)
	setStringJSONField(arena, obj, "runtimeIdentity", r.RuntimeIdentity)
	obj.Set("generation", arena.NewNumberString(strconv.FormatUint(r.Generation, 10)))

	// Encode cleanup completion facts and reason.
	setBoolJSONField(arena, obj, "runtimeStopped", r.RuntimeStopped)
	setBoolJSONField(arena, obj, "capacityReleased", r.CapacityReleased)
	setStringJSONField(arena, obj, "reason", r.Reason)
	return obj, nil
}

// unmarshalJSONValue unmarshals the receipt from a parsed object value.
func (r *CleanupReceipt) unmarshalJSONValue(value *fastjson.Value) error {
	// Restore cleanup identities and generation from the JSON receipt.
	r.ReservationObjectKey = string(value.GetStringBytes("reservationObjectKey"))
	r.ExecutionObjectKey = string(value.GetStringBytes("executionObjectKey"))
	r.RuntimeIdentity = string(value.GetStringBytes("runtimeIdentity"))
	r.Generation = value.GetUint64("generation")

	// Restore cleanup completion facts and reason.
	r.RuntimeStopped = value.GetBool("runtimeStopped")
	r.CapacityReleased = value.GetBool("capacityReleased")
	r.Reason = string(value.GetStringBytes("reason"))
	return nil
}

// MarshalJSON marshals the CleanupReceipt to JSON without reflection.
func (r *CleanupReceipt) MarshalJSON() ([]byte, error) {
	// Preserve a nil cleanup receipt as JSON null.
	if r == nil {
		return []byte("null"), nil
	}

	// Encode the cleanup receipt in a fresh JSON arena.
	var arena fastjson.Arena
	value, err := r.marshalJSONValue(&arena)
	if err != nil {
		return nil, err
	}
	return value.MarshalTo(nil), nil
}

// UnmarshalJSON unmarshals the CleanupReceipt from JSON without reflection.
func (r *CleanupReceipt) UnmarshalJSON(data []byte) error {
	// Parse the cleanup receipt JSON before restoring its fields.
	var parser fastjson.Parser
	value, err := parser.ParseBytes(data)
	if err != nil {
		return err
	}
	if value.Type() == fastjson.TypeNull {
		// Accept a null receipt or require an object for decoding.
		*r = CleanupReceipt{}
		return nil
	}
	if value.Type() != fastjson.TypeObject {
		return errors.New("cleanup receipt must be object")
	}
	return r.unmarshalJSONValue(value)
}
