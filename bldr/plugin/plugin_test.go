package bldr_plugin

import "testing"

func TestPluginRpcComponentID(t *testing.T) {
	t.Parallel()

	componentID := BuildPluginRpcComponentID("spacewave-v86", "sample/transient-v86/exec-1", "")
	if componentID != "spacewave-v86/sample%2Ftransient-v86%2Fexec-1" {
		t.Fatalf("unexpected component id: %q", componentID)
	}

	pluginID, instanceKey, manifestRoot, err := ParsePluginRpcComponentID(componentID)
	if err != nil || manifestRoot != "" {
		t.Fatalf("unexpected executable: %q, %v", manifestRoot, err)
	}
	if pluginID != "spacewave-v86" {
		t.Fatalf("unexpected plugin id: %q", pluginID)
	}
	if instanceKey != "sample/transient-v86/exec-1" {
		t.Fatalf("unexpected instance key: %q", instanceKey)
	}
}

func TestPluginRpcComponentIDShared(t *testing.T) {
	t.Parallel()

	componentID := BuildPluginRpcComponentID("spacewave-core", "", "")
	if componentID != "spacewave-core" {
		t.Fatalf("unexpected component id: %q", componentID)
	}

	pluginID, instanceKey, manifestRoot, err := ParsePluginRpcComponentID(componentID)
	if err != nil || manifestRoot != "" {
		t.Fatalf("unexpected executable: %q, %v", manifestRoot, err)
	}
	if pluginID != "spacewave-core" {
		t.Fatalf("unexpected plugin id: %q", pluginID)
	}
	if instanceKey != "" {
		t.Fatalf("unexpected instance key: %q", instanceKey)
	}
}

// TestPluginRpcComponentIDManifest keeps reserved URL characters in the instance
// from changing which immutable executable a cross-worker call addresses.
func TestPluginRpcComponentIDManifest(t *testing.T) {
	instance := "space/a?manifest=other#revision"
	componentID := BuildPluginRpcComponentID("colors", instance, "exact-root")
	pluginID, instanceKey, manifestRoot, err := ParsePluginRpcComponentID(componentID)
	if err != nil || pluginID != "colors" || instanceKey != instance || manifestRoot != "exact-root" {
		t.Fatalf("unexpected address: %q, %q, %q, %v", pluginID, instanceKey, manifestRoot, err)
	}
}
