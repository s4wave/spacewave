package s4wave_device

import (
	"context"
	"strings"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// SensorTypeID is the type identifier for Spacewave-managed Sensor objects.
const SensorTypeID = "spacewave/sensor"

// DeviceCapabilityKindSensor identifies sensor endpoints exposed by a Device.
const DeviceCapabilityKindSensor = "sensor"

// PredDeviceToSensor is the typed graph edge linking a Device object to the
// Sensor endpoint it owns.
var PredDeviceToSensor = quad.IRI("device/sensor")

// NewDeviceToSensorQuad creates the graph edge from a Device object to one of
// its Sensor endpoint objects.
func NewDeviceToSensorQuad(deviceObjKey, sensorObjKey string) world.GraphQuad {
	return world.NewGraphQuadWithKeys(
		deviceObjKey,
		PredDeviceToSensor.String(),
		sensorObjKey,
		"",
	)
}

// NewSensorBlock constructs a new Sensor block.
func NewSensorBlock() block.Block {
	return &Sensor{}
}

// UnmarshalSensor unmarshals a Sensor from a cursor.
func UnmarshalSensor(ctx context.Context, bcs *block.Cursor) (*Sensor, error) {
	return block.UnmarshalBlock[*Sensor](ctx, bcs, NewSensorBlock)
}

// MarshalBlock marshals the Sensor to bytes.
func (s *Sensor) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// UnmarshalBlock unmarshals the Sensor from bytes.
func (s *Sensor) UnmarshalBlock(data []byte) error {
	return s.UnmarshalVT(data)
}

// Validate performs cursory checks on the Sensor block.
func (s *Sensor) Validate() error {
	if strings.TrimSpace(s.GetEndpointId()) == "" {
		return errors.New("sensor endpoint_id is required")
	}
	if s.GetAdapterKind() == SensorAdapterKind_SENSOR_ADAPTER_KIND_UNKNOWN {
		return errors.New("sensor adapter_kind is required")
	}
	if strings.TrimSpace(s.GetEndpointLabel()) == "" {
		return errors.New("sensor endpoint_label is required")
	}
	if s.GetConnectionState() == SensorConnectionState_SENSOR_CONNECTION_STATE_UNKNOWN {
		return errors.New("sensor connection_state is required")
	}
	seenEntities := make(map[string]struct{}, len(s.GetEntities()))
	for _, entity := range s.GetEntities() {
		if entity == nil {
			continue
		}
		objectID := strings.TrimSpace(entity.GetObjectId())
		if objectID == "" {
			return errors.New("sensor entity object_id is required")
		}
		if _, ok := seenEntities[objectID]; ok {
			return errors.Errorf("duplicate sensor entity %q", objectID)
		}
		seenEntities[objectID] = struct{}{}
		if entity.GetValueKind() == SensorEntityValueKind_SENSOR_ENTITY_VALUE_KIND_UNKNOWN {
			return errors.Errorf("sensor entity %q value_kind is required", objectID)
		}
	}
	return nil
}

// FindEntity returns the enumerated entity with the requested object ID.
func (s *Sensor) FindEntity(objectID string) *SensorEntity {
	objectID = strings.TrimSpace(objectID)
	if s == nil || objectID == "" {
		return nil
	}
	for _, entity := range s.GetEntities() {
		if entity != nil && strings.TrimSpace(entity.GetObjectId()) == objectID {
			return entity
		}
	}
	return nil
}

// TouchUpdatedAt stamps the projected change time when it advances.
func (s *Sensor) TouchUpdatedAt(now *timestamppb.Timestamp) {
	if now == nil {
		return
	}
	if current := s.GetUpdatedAt(); current != nil && current.AsTime().After(now.AsTime()) {
		return
	}
	s.UpdatedAt = now.CloneVT()
}

// _ is a type assertion
var _ block.Block = (*Sensor)(nil)
