package plugin_host_scheduler

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/sirupsen/logrus"
)

// TestStartupProtocolFailureStopsSelection checks the scheduler's retry decision
// after process disconnect, then allows a newly selected manifest to start.
func TestStartupProtocolFailureStopsSelection(t *testing.T) {
	// Exercise the production selection lifecycle with a failing execution boundary.
	le := logrus.NewEntry(logrus.New())
	c := &Controller{le: le, conf: &Config{},
		pluginStatusCtr: ccontainer.NewCContainer(&plugin.PluginStatusSnapshot{}),
		pluginStatus:    make(map[string]*plugin.PluginStatus),
	}
	_, instance := c.newPluginInstance(pluginReference{pluginID: "sample"})
	var starts atomic.Int32
	instance.executions = keyed.NewKeyedRefCount(func(key executionReference) (keyed.Routine, *pluginInstance) {
		_, worker := instance.newExecution(key)
		return func(context.Context) error {
			starts.Add(1)
			worker.beginInitialCapabilityRegistration()
			worker.updateRpcClient(nil)
			if got := worker.pluginLoadStateCtr.GetValue().GetInitialCapabilityRegistrationState(); got != plugin.InitialCapabilityRegistrationPending {
				t.Error("disconnect discarded pending process failure")
			}
			err := plugin_host.NewStartupProtocolError("host-a", "plugin-b", errors.New("proto: wrong wireType"))
			worker.finishExecution(err)
			return err
		}, worker
	})
	instance.executions.SetContext(t.Context(), true)
	t.Cleanup(instance.executions.ClearContext)
	t.Cleanup(func() { instance.clearExecution(nil) })

	// A nil routine result suppresses retry; another manifest remains selectable.
	for rev := uint64(1); rev <= 2; rev++ {
		args := &executePluginArgs{pluginHost: &testPluginHost{id: "test"}, manifestSnapshot: &manifest.ManifestSnapshot{
			ManifestRef: newTestManifestRef("sample", "js", rev, "candidate").GetManifestRef(),
			Manifest:    &manifest.Manifest{Meta: manifest.NewManifestMeta("sample", manifest.BuildType_RELEASE, "js", rev)},
		}}
		if err := instance.execSelectedPlugin(t.Context(), args); err != nil {
			t.Fatalf("selection returned retryable error: %v", err)
		}
		if got := starts.Load(); got != int32(rev) {
			t.Fatalf("executions = %d, want %d", got, rev)
		}
		state := instance.pluginLoadStateCtr.GetValue()
		if state.GetInitialCapabilityRegistrationState() != plugin.InitialCapabilityRegistrationFailed {
			t.Fatal("terminal startup failure remained pending")
		}
		if err := state.GetStartupError(); err == nil || !strings.Contains(err.Error(), "host-a; plugin build plugin-b") {
			t.Fatalf("startup diagnostic = %v", err)
		}
	}
}
