package plugin_entrypoint_controller

import (
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

type testStartupPluginRef struct {
	stateCtr *ccontainer.CContainer[bldr_plugin.PluginLoadState]
}

func newTestStartupPluginRef() *testStartupPluginRef {
	return &testStartupPluginRef{
		stateCtr: ccontainer.NewCContainer(bldr_plugin.NewPluginLoadState(
			nil,
			bldr_plugin.InitialCapabilityRegistrationPending,
		)),
	}
}

func (r *testStartupPluginRef) GetRunningPluginCtr() ccontainer.Watchable[bldr_plugin.RunningPlugin] {
	return ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil)
}

func (r *testStartupPluginRef) GetPluginLoadStateCtr() ccontainer.Watchable[bldr_plugin.PluginLoadState] {
	return r.stateCtr
}

type testStartupPluginSource struct {
	refs       map[string]*testStartupPluginRef
	releasedCh map[string]chan struct{}
}

func newTestStartupPluginSource(pluginIDs ...string) *testStartupPluginSource {
	source := &testStartupPluginSource{
		refs:       make(map[string]*testStartupPluginRef, len(pluginIDs)),
		releasedCh: make(map[string]chan struct{}, len(pluginIDs)),
	}
	for _, pluginID := range pluginIDs {
		source.refs[pluginID] = newTestStartupPluginRef()
		source.releasedCh[pluginID] = make(chan struct{})
	}
	return source
}

func (s *testStartupPluginSource) AddPluginReference(
	pluginID, _ string,
) (bldr_plugin.RunningPluginRef, func()) {
	return s.refs[pluginID], func() { close(s.releasedCh[pluginID]) }
}

func TestStartupGroupCoordinatorTransitionsOnceAfterAllPluginsTerminal(t *testing.T) {
	// Start a coordinator for a duplicated, unordered plugin list.
	ctx := t.Context()
	source := newTestStartupPluginSource("plugin/a", "plugin/b")
	coordinator := NewStartupGroupCoordinator([]string{"plugin/b", "plugin/a", "plugin/a"}, source)
	if err := coordinator.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// Assert the group is not ready before any plugin completes.
	if coordinator.IsReady() {
		t.Fatal("startup group was ready before any plugin completed")
	}

	// Complete plugin/a and wait for its reference release.
	source.refs["plugin/a"].stateCtr.SetValue(bldr_plugin.NewPluginLoadState(
		nil,
		bldr_plugin.InitialCapabilityRegistrationComplete,
	))
	<-source.releasedCh["plugin/a"]

	// Assert the group is still not ready while plugin/b is pending.
	if coordinator.IsReady() {
		t.Fatal("startup group was ready before the last plugin completed")
	}

	// Complete plugin/b and wait for the group to become ready.
	source.refs["plugin/b"].stateCtr.SetValue(bldr_plugin.NewPluginLoadState(
		nil,
		bldr_plugin.InitialCapabilityRegistrationComplete,
	))
	if err := coordinator.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	<-source.releasedCh["plugin/b"]

	// Push plugin/a back to pending and assert readiness stays terminal.
	source.refs["plugin/a"].stateCtr.SetValue(bldr_plugin.NewPluginLoadState(
		nil,
		bldr_plugin.InitialCapabilityRegistrationPending,
	))
	if !coordinator.IsReady() || !coordinator.GetReadyCtr().GetValue() {
		t.Fatal("startup group readiness reverted after its terminal transition")
	}
}

func TestStartupGroupCoordinatorReleasesAfterTerminalFailure(t *testing.T) {
	// Start a coordinator watching a single failing plugin.
	ctx := t.Context()
	source := newTestStartupPluginSource("plugin/broken")
	coordinator := NewStartupGroupCoordinator([]string{"plugin/broken"}, source)
	if err := coordinator.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// Fail the plugin and wait for the group to become ready and release it.
	source.refs["plugin/broken"].stateCtr.SetValue(bldr_plugin.NewPluginLoadState(
		nil,
		bldr_plugin.InitialCapabilityRegistrationFailed,
	))
	if err := coordinator.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	<-source.releasedCh["plugin/broken"]
}

func TestStartupGroupCoordinatorReleasesAfterStartupBudgetExhaustion(t *testing.T) {
	// Start a coordinator watching a single stuck plugin.
	ctx := t.Context()
	source := newTestStartupPluginSource("plugin/stuck")
	coordinator := NewStartupGroupCoordinator([]string{"plugin/stuck"}, source)
	if err := coordinator.Start(ctx); err != nil {
		t.Fatal(err)
	}

	// Exhaust the plugin's startup budget and wait for readiness and release.
	source.refs["plugin/stuck"].stateCtr.SetValue(
		bldr_plugin.NewPluginLoadState(
			nil,
			bldr_plugin.InitialCapabilityRegistrationPending,
		).WithStartupBudgetExhausted(),
	)
	if err := coordinator.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
	<-source.releasedCh["plugin/stuck"]

	// Clear the budget exhaustion and assert readiness stays terminal.
	source.refs["plugin/stuck"].stateCtr.SetValue(bldr_plugin.NewPluginLoadState(
		nil,
		bldr_plugin.InitialCapabilityRegistrationPending,
	))
	if !coordinator.IsReady() || !coordinator.GetReadyCtr().GetValue() {
		t.Fatal("startup group readiness reverted after budget exhaustion")
	}
}
