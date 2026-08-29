package main

import (
	"testing"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
)

// TestConfigSetProviderLocalSignaling decodes the compiled configset.bin and
// asserts the provider-local entry carries the trusted cloud signaling URL
// serialized from bldr.star, so the running CLI default matches the build
// graph.
func TestConfigSetProviderLocalSignaling(t *testing.T) {
	data, err := configSetFS.ReadFile("configset.bin")
	if err != nil {
		t.Fatal(err)
	}
	set := &configset_proto.ConfigSet{}
	if err := set.UnmarshalVT(data); err != nil {
		t.Fatalf("decode configset.bin: %v", err)
	}
	entry, ok := set.GetConfigs()["provider-local"]
	if !ok {
		t.Fatalf("configset.bin has no provider-local entry: %v", set.GetConfigs())
	}
	cfg := &provider_local.Config{}
	if err := cfg.UnmarshalJSON(entry.GetConfig()); err != nil {
		t.Fatalf("decode provider-local config: %v", err)
	}
	// PRODUCTION_CLOUD_API_ENDPOINT in bldr.star.
	if got := cfg.GetSignalingUrl(); got != "https://spacewave.app" {
		t.Fatalf("provider-local signalingUrl = %q, want https://spacewave.app", got)
	}
	if _, err := cfg.ParseSignalingURL(); err != nil {
		t.Fatalf("provider-local signalingUrl invalid: %v", err)
	}
}
