package s4wave_device

import (
	"testing"

	"github.com/aperturerobotics/cayley/quad"
)

func TestDeviceToSensorQuadLinksTypedEdge(t *testing.T) {
	q := NewDeviceToSensorQuad("devices/a", "sensor-key")
	if q.GetSubject() == "" || q.GetPredicate() != PredDeviceToSensor.String() || q.GetObj() == "" {
		t.Fatalf("quad = %+v, want a complete device-to-sensor edge", q)
	}
	if PredDeviceToSensor != quad.IRI("device/sensor") {
		t.Fatalf("predicate = %v, want device/sensor", PredDeviceToSensor)
	}
}

func TestSensorValidateRequiresEndpointIdentity(t *testing.T) {
	sensor := &Sensor{
		EndpointId:      "",
		AdapterKind:     SensorAdapterKind_SENSOR_ADAPTER_KIND_ESPHOME,
		EndpointLabel:   "radar",
		ConnectionState: SensorConnectionState_SENSOR_CONNECTION_STATE_CONNECTING,
	}
	if err := sensor.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want endpoint_id rejection")
	}
	sensor.EndpointId = "radar"
	if err := sensor.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}
