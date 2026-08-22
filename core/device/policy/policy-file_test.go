package device_policy

import (
	"strings"
	"testing"

	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

func TestValidateAcceptsWellFormedSensorEndpoints(t *testing.T) {
	policy := &DevicePolicy{
		SensorEndpoint: []*SensorEndpointPolicy{
			{Id: "radar", Enabled: true, AdapterKind: s4wave_device.SensorAdapterKind_SENSOR_ADAPTER_KIND_ESPHOME, Endpoint: "127.0.0.1:6053"},
			// Disabled endpoints may keep an unset adapter kind.
			{Id: "spare", Enabled: false, Endpoint: "10.0.0.9:6053"},
		},
	}
	if err := Validate(policy); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsBadSensorEndpoints(t *testing.T) {
	esphome := s4wave_device.SensorAdapterKind_SENSOR_ADAPTER_KIND_ESPHOME
	cases := map[string]*DevicePolicy{
		"missing id": {
			SensorEndpoint: []*SensorEndpointPolicy{{Enabled: true, AdapterKind: esphome, Endpoint: "h:1"}},
		},
		"duplicate id": {
			SensorEndpoint: []*SensorEndpointPolicy{
				{Id: "radar", Enabled: true, AdapterKind: esphome, Endpoint: "a:1"},
				{Id: "radar", Enabled: true, AdapterKind: esphome, Endpoint: "b:2"},
			},
		},
		"missing endpoint address": {
			SensorEndpoint: []*SensorEndpointPolicy{{Id: "radar", Enabled: true, AdapterKind: esphome}},
		},
		"unknown adapter kind while enabled": {
			SensorEndpoint: []*SensorEndpointPolicy{{Id: "radar", Enabled: true, Endpoint: "h:1"}},
		},
	}
	for name, policy := range cases {
		err := Validate(policy)
		if err == nil {
			t.Fatalf("%s: Validate() error = nil, want rejection", name)
		}
		if !strings.Contains(err.Error(), "sensor-endpoint") {
			t.Fatalf("%s: error = %v, want a sensor-endpoint failure", name, err)
		}
	}
}
