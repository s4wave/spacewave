package s4wave_vm_world

import "testing"

func TestDefaultVmRuntimePluginID(t *testing.T) {
	if defaultVmPluginID != "spacewave-v86" {
		t.Fatalf("defaultVmPluginID = %q, want %q", defaultVmPluginID, "spacewave-v86")
	}
}
