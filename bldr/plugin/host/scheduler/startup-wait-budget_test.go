package plugin_host_scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/sirupsen/logrus"
)

func newStartupWaitBudgetTestInstance() *pluginInstance {
	ctrl := &Controller{
		pluginStatusCtr: ccontainer.NewCContainer(&PluginStatusSnapshot{}),
		pluginStatus:    make(map[string]*bldr_plugin.PluginStatus),
	}
	return &pluginInstance{
		c:                ctrl,
		le:               logrus.NewEntry(logrus.New()),
		pluginID:         "test-plugin",
		runningPluginCtr: ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
		pluginLoadStateCtr: ccontainer.NewCContainer(
			bldr_plugin.NewPluginLoadState(
				nil,
				bldr_plugin.InitialCapabilityRegistrationPending,
			),
		),
	}
}

// waitForStartupBudgetExhausted waits until the instance publishes the
// exhausted startup wait budget dimension.
func waitForStartupBudgetExhausted(
	t *testing.T,
	inst *pluginInstance,
) bldr_plugin.PluginLoadState {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	current := inst.pluginLoadStateCtr.GetValue()
	for !current.GetStartupBudgetExhausted() {
		next, err := inst.pluginLoadStateCtr.WaitValueChange(ctx, current, nil)
		if err != nil {
			t.Fatalf("startup wait budget never reported exhausted: %v", err)
		}
		current = next
	}
	return current
}

func TestStartupWaitBudgetPublishesExhaustionWhilePending(t *testing.T) {
	instance := newStartupWaitBudgetTestInstance()
	defer instance.stopStartupWaitBudget()

	instance.armStartupWaitBudget(5 * time.Millisecond)
	state := waitForStartupBudgetExhausted(t, instance)
	if state.GetInitialCapabilityRegistrationState() != bldr_plugin.InitialCapabilityRegistrationPending {
		t.Fatalf("registration state = %v, want pending", state.GetInitialCapabilityRegistrationState())
	}
}

func TestStartupWaitBudgetDeadlineNoOpsAfterCompletion(t *testing.T) {
	instance := newStartupWaitBudgetTestInstance()

	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(srpc.NewMux())))
	instance.updateRpcClient(client)
	instance.finishInitialCapabilityRegistration(true)

	// A deadline firing after completion must leave the terminal state and
	// the published projection untouched.
	instance.markStartupWaitBudgetExhausted(time.Minute)

	state := instance.pluginLoadStateCtr.GetValue()
	if state.GetStartupBudgetExhausted() {
		t.Fatal("deadline marked a completed registration as exhausted")
	}
	if state.GetInitialCapabilityRegistrationState() != bldr_plugin.InitialCapabilityRegistrationComplete {
		t.Fatalf("registration state = %v, want complete", state.GetInitialCapabilityRegistrationState())
	}
	if running := instance.runningPluginCtr.GetValue(); running == nil {
		t.Fatal("deadline callback regressed the running plugin projection")
	}
}

func TestExecutionRetriesContinueAfterBudgetExhaustion(t *testing.T) {
	instance := newStartupWaitBudgetTestInstance()
	defer instance.stopStartupWaitBudget()

	instance.armStartupWaitBudget(5 * time.Millisecond)
	waitForStartupBudgetExhausted(t, instance)

	// A retry attempt still delivers an RPC client and completes
	// registration after the budget was exhausted.
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(srpc.NewMux())))
	instance.updateRpcClient(client)
	instance.finishInitialCapabilityRegistration(true)

	state := instance.pluginLoadStateCtr.GetValue()
	if state.GetInitialCapabilityRegistrationState() != bldr_plugin.InitialCapabilityRegistrationComplete {
		t.Fatalf("registration state = %v, want complete after exhaustion", state.GetInitialCapabilityRegistrationState())
	}
	if running := instance.runningPluginCtr.GetValue(); running == nil {
		t.Fatal("plugin did not report running after completing a retry post-exhaustion")
	}
}

func TestStartupBudgetExhaustionDoesNotRevert(t *testing.T) {
	instance := newStartupWaitBudgetTestInstance()
	defer instance.stopStartupWaitBudget()

	instance.armStartupWaitBudget(5 * time.Millisecond)
	waitForStartupBudgetExhausted(t, instance)

	// A subsequent execution attempt for the same instance boot keeps the
	// exhausted fact published.
	instance.beginInitialCapabilityRegistration()
	state := instance.pluginLoadStateCtr.GetValue()
	if !state.GetStartupBudgetExhausted() {
		t.Fatal("retry execution attempt reverted the exhausted startup wait budget")
	}
	if state.GetInitialCapabilityRegistrationState() != bldr_plugin.InitialCapabilityRegistrationPending {
		t.Fatalf("registration state = %v, want pending", state.GetInitialCapabilityRegistrationState())
	}
}
