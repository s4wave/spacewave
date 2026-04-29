package bldr_plugin

import "testing"

func TestPluginRpcComponentID(t *testing.T) {
	t.Parallel()

	componentID := BuildPluginRpcComponentID("spacewave-v86", "glados/transient-v86/exec-1")
	if componentID != "spacewave-v86/glados/transient-v86/exec-1" {
		t.Fatalf("unexpected component id: %q", componentID)
	}

	pluginID, instanceKey := ParsePluginRpcComponentID(componentID)
	if pluginID != "spacewave-v86" {
		t.Fatalf("unexpected plugin id: %q", pluginID)
	}
	if instanceKey != "glados/transient-v86/exec-1" {
		t.Fatalf("unexpected instance key: %q", instanceKey)
	}
}

func TestPluginRpcComponentIDShared(t *testing.T) {
	t.Parallel()

	componentID := BuildPluginRpcComponentID("spacewave-core", "")
	if componentID != "spacewave-core" {
		t.Fatalf("unexpected component id: %q", componentID)
	}

	pluginID, instanceKey := ParsePluginRpcComponentID(componentID)
	if pluginID != "spacewave-core" {
		t.Fatalf("unexpected plugin id: %q", pluginID)
	}
	if instanceKey != "" {
		t.Fatalf("unexpected instance key: %q", instanceKey)
	}
}
