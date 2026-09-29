package plugin_entrypoint_controller

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/bus/inmem"
	cdc "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/sirupsen/logrus"
)

// testHostScheduler is a host scheduler with a settable status snapshot.
type testHostScheduler struct {
	statusCtr *ccontainer.CContainer[*bldr_plugin.PluginStatusSnapshot]
}

func (s *testHostScheduler) GetInstanceKey() string {
	return ""
}

func (s *testHostScheduler) GetPluginStatusCtr() ccontainer.Watchable[*bldr_plugin.PluginStatusSnapshot] {
	return s.statusCtr
}

// TestLookupPluginSchedulerStreamsHostStatus checks that a plugin sees its
// host scheduler's plugin status, including later changes.
func TestLookupPluginSchedulerStreamsHostStatus(t *testing.T) {
	// Bound the test and build a logger for the plugin host and controller.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Run a plugin host server with a settable status snapshot over an in-memory SRPC pipe.
	hostBus := inmem.NewBus(cdc.NewController(ctx, le))
	scheduler := &testHostScheduler{
		statusCtr: ccontainer.NewCContainer(&bldr_plugin.PluginStatusSnapshot{}),
	}
	host := plugin_host.NewPluginHostServer(ctx, hostBus, le, "spacewave-core", "", nil, nil, "", false)
	host.SetPluginScheduler(scheduler)
	mux := srpc.NewMux()
	if err := bldr_plugin.SRPCRegisterPluginHost(mux, host); err != nil {
		t.Fatal(err)
	}
	hostClient := bldr_plugin.NewSRPCPluginHostClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))

	// Add the plugin entrypoint controller against the host client.
	pluginBus := inmem.NewBus(cdc.NewController(ctx, le))
	meta := &bldr_plugin.PluginMeta{PluginId: "spacewave-core"}
	rel, err := pluginBus.AddController(ctx, NewController(pluginBus, le, meta, hostClient), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	// Resolve LookupPluginScheduler and expect the root host scheduler.
	vals, _, ref, err := bus.ExecCollectValues[bldr_plugin.LookupPluginSchedulerValue](
		ctx, pluginBus, bldr_plugin.NewLookupPluginScheduler(), false, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()
	if len(vals) != 1 || vals[0].GetInstanceKey() != "" {
		t.Fatalf("schedulers = %v, want the root host scheduler", vals)
	}

	// Publish a running plugin snapshot through the host scheduler.
	scheduler.statusCtr.SetValue(&bldr_plugin.PluginStatusSnapshot{
		Plugins: []*bldr_plugin.PluginStatus{{
			PluginId:    "spacewave-notes",
			InstanceKey: "space/local/account/space",
			State:       bldr_plugin.PluginState_PluginState_RUNNING,
		}},
	})

	// Wait for the scheduler to stream the running plugin status.
	got, err := vals[0].GetPluginStatusCtr().WaitValueWithValidator(
		ctx,
		func(status *bldr_plugin.PluginStatusSnapshot) (bool, error) {
			return len(status.GetPlugins()) != 0, nil
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	// Assert the streamed status reports the running spacewave-notes plugin.
	if plugin := got.GetPlugins()[0]; plugin.GetPluginId() != "spacewave-notes" ||
		plugin.GetState() != bldr_plugin.PluginState_PluginState_RUNNING {
		t.Fatalf("plugin status = %v, want running spacewave-notes", plugin)
	}
}
