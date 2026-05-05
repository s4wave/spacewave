package plugin_space

import "testing"

func TestControllerLoadedPluginIDsSnapshotAndWake(t *testing.T) {
	c := &Controller{}

	c.setLoadedPluginIDs([]string{"plugin-a"})
	ids, ch := c.GetLoadedPluginIDsAndWaitCh()
	if len(ids) != 1 || ids[0] != "plugin-a" {
		t.Fatalf("unexpected loaded plugin ids: %v", ids)
	}

	ids[0] = "mutated"
	next, _ := c.GetLoadedPluginIDsAndWaitCh()
	if len(next) != 1 || next[0] != "plugin-a" {
		t.Fatalf("loaded plugin ids alias leaked: %v", next)
	}

	c.setLoadedPluginIDs([]string{"plugin-a", "plugin-b"})
	select {
	case <-ch:
	default:
		t.Fatal("expected wait channel to close after loaded plugin set changed")
	}
}
