package plugin_host_scheduler

import (
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/sirupsen/logrus"
)

func TestPluginInstanceWaitsForInitialCapabilityRegistration(t *testing.T) {
	le := logrus.NewEntry(logrus.New())
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainer(&PluginStatusSnapshot{}),
		pluginStatus:    make(map[string]*bldr_plugin.PluginStatus),
	}
	instance := &pluginInstance{
		c:                ctrl,
		le:               le,
		pluginID:         "test-plugin",
		runningPluginCtr: ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
		pluginLoadStateCtr: ccontainer.NewCContainer(
			bldr_plugin.NewPluginLoadState(
				nil,
				bldr_plugin.InitialCapabilityRegistrationPending,
			),
		),
	}

	instance.beginInitialCapabilityRegistration()
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(srpc.NewMux())))
	instance.updateRpcClient(client)
	if running := instance.runningPluginCtr.GetValue(); running != nil {
		t.Fatal("RPC-connected plugin reported running before initial registration")
	}
	state := instance.pluginLoadStateCtr.GetValue()
	if state.GetRpcClient() == nil {
		t.Fatal("expected RPC client during initial registration")
	}
	if state.GetInitialCapabilityRegistrationState() != bldr_plugin.InitialCapabilityRegistrationPending {
		t.Fatalf("registration state = %v, want pending", state.GetInitialCapabilityRegistrationState())
	}

	instance.finishInitialCapabilityRegistration(true)
	if running := instance.runningPluginCtr.GetValue(); running == nil {
		t.Fatal("plugin did not report running after initial registration")
	}
	state = instance.pluginLoadStateCtr.GetValue()
	if state.GetInitialCapabilityRegistrationState() != bldr_plugin.InitialCapabilityRegistrationComplete {
		t.Fatalf("registration state = %v, want complete", state.GetInitialCapabilityRegistrationState())
	}
}
