package bldr_cli_compiler

import (
	"testing"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
)

func TestMarshalConfigSetDeterministic(t *testing.T) {
	// Build two configs whose map insertion order differs from their sorted order.
	a := &configset_proto.ControllerConfig{Id: "a", Rev: 1, Config: []byte(`{"a":1}`)}
	z := &configset_proto.ControllerConfig{Id: "z", Rev: 2, Config: []byte(`{"z":2}`)}

	// Marshal a config set inserting the later id first.
	first, err := marshalConfigSetDeterministic(map[string]*configset_proto.ControllerConfig{"z": z, "a": a})
	if err != nil {
		t.Fatal(err)
	}

	// Marshal the same configs inserting the earlier id first.
	second, err := marshalConfigSetDeterministic(map[string]*configset_proto.ControllerConfig{"a": a, "z": z})
	if err != nil {
		t.Fatal(err)
	}

	// Both insertion orders must produce identical encodings.
	if string(first) != string(second) {
		t.Fatal("config set encoding changes with map insertion order")
	}

	// Decode the encoding back into a ConfigSet.
	decoded := &configset_proto.ConfigSet{}
	if err := decoded.UnmarshalVT(first); err != nil {
		t.Fatal(err)
	}

	// The decoded configs must equal the originals.
	if !decoded.GetConfigs()["a"].EqualVT(a) || !decoded.GetConfigs()["z"].EqualVT(z) {
		t.Fatalf("decoded config set does not match input: %+v", decoded.GetConfigs())
	}
}
