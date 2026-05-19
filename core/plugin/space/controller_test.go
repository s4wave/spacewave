package plugin_space

import (
	"testing"

	"github.com/sirupsen/logrus"
)

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

func TestFilterValidPluginIDsDropsCorruptSettingsEntries(t *testing.T) {
	got := filterValidPluginIDs(
		logrus.NewEntry(logrus.New()),
		[]string{"spacewave-notes", "\b\x02\x1aBbinary-plugin-id", "spacewave-app"},
	)
	want := []string{"spacewave-notes", "spacewave-app"}
	if len(got) != len(want) {
		t.Fatalf("plugin ids = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("plugin ids = %q, want %q", got, want)
		}
	}
}
