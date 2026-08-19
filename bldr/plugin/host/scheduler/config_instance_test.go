package plugin_host_scheduler

import "testing"

func TestConfigInstanceKeyIsolatedFromDist(t *testing.T) {
	base := NewConfig("", "engine", "plugin-host", "volume", "peer", true, false, false)
	space := NewConfig("space-a", "engine", "plugin-host", "volume", "peer", true, false, false)
	if base.GetInstanceKey() != "" {
		t.Fatalf("Dist instance key = %q", base.GetInstanceKey())
	}
	if space.GetInstanceKey() != "space-a" {
		t.Fatalf("Space instance key = %q", space.GetInstanceKey())
	}
	if base.EqualsConfig(space) {
		t.Fatal("Dist and Space scheduler configs compare equal")
	}
}
