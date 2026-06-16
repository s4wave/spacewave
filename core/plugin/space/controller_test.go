package plugin_space

import (
	"testing"

	"github.com/aperturerobotics/util/keyed"
	"github.com/sirupsen/logrus"
)

func TestReconcileProcessConfigsClearsWhenStoreUnavailable(t *testing.T) {
	c := &Controller{
		processConfigs: map[string]processConfig{
			"stale-process": {typeID: "test/process"},
		},
	}
	c.processes = keyed.NewKeyed(func(key string) (keyed.Routine, processConfig) {
		return nil, c.processConfigs[key]
	})
	c.processes.SyncKeys([]string{"stale-process"}, false)

	c.reconcileProcessConfigs(logrus.NewEntry(logrus.New()), nil)

	if got := c.processes.GetKeys(); len(got) != 0 {
		t.Fatalf("process keys after unavailable store = %v, want none", got)
	}
	if got := len(c.processConfigs); got != 0 {
		t.Fatalf("process config count after unavailable store = %d, want 0", got)
	}
}
