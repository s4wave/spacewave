package plugin_host_scheduler

import (
	"context"
	"encoding/base64"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_entrypoint "github.com/s4wave/spacewave/bldr/plugin/entrypoint"
	plugin_entrypoint_controller "github.com/s4wave/spacewave/bldr/plugin/entrypoint/controller"
	plugin_forward_rpc_service "github.com/s4wave/spacewave/bldr/plugin/forward-rpc-service"
	bldr_web_plugin_controller "github.com/s4wave/spacewave/bldr/web/plugin/controller"
	web_view "github.com/s4wave/spacewave/bldr/web/view"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/db/testbed"
	volume_rpc "github.com/s4wave/spacewave/db/volume/rpc"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	bifrost_rpc_access "github.com/s4wave/spacewave/net/rpc/access"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
	"github.com/sirupsen/logrus"
)

// probeReached starts the error every probe service answers with, followed by
// the name of its bus, so a call that returns it reached that bus's service.
const probeReached = "probe service reached on "

// probeServices answers LookupRpcService on a bus for the listed service IDs
// with an invoker that only reports the call reached it.
type probeServices struct {
	bus string
	ids []string
}

func (p *probeServices) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/probe-services", controller.MustParseVersion("0.0.1"), "")
}

func (p *probeServices) Execute(context.Context) error {
	return nil
}

func (p *probeServices) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	d, ok := di.GetDirective().(bifrost_rpc.LookupRpcService)
	if !ok || !slices.Contains(p.ids, d.LookupRpcServiceID()) {
		return nil, nil
	}
	inv := srpc.InvokerFunc(func(string, string, srpc.Stream) (bool, error) {
		return true, errors.New(probeReached + p.bus)
	})
	return directive.R(directive.NewValueResolver([]bifrost_rpc.LookupRpcServiceValue{inv}), nil)
}

func (p *probeServices) Close() error {
	return nil
}

// schedulerLookups resolves the scheduler's plugin and plugin-host RPC
// lookups on the bus as its HandleDirective does, without running plugins.
type schedulerLookups struct {
	sched *Controller
}

func (s *schedulerLookups) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/scheduler-lookups", controller.MustParseVersion("0.0.1"), "")
}

func (s *schedulerLookups) Execute(context.Context) error {
	return nil
}

func (s *schedulerLookups) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	d, ok := di.GetDirective().(bifrost_rpc.LookupRpcService)
	if !ok {
		return nil, nil
	}
	return directive.R(bldr_plugin.ResolveLookupRpcService(ctx, d, s.sched))
}

func (s *schedulerLookups) Close() error {
	return nil
}

// fallbackBed is a host bus with a scheduler whose plugin muxes are probed.
type fallbackBed struct {
	sched *Controller
	loads chan string
}

// newFallbackBed builds the scheduler on b with conf and the probe services.
func newFallbackBed(ctx context.Context, t *testing.T, le *logrus.Entry, b bus.Bus, busName string, conf *Config, serviceIDs []string) *fallbackBed {
	sched := NewController(le, b, conf)
	bed := &fallbackBed{sched: sched, loads: make(chan string, 8)}
	addControllers(ctx, t, b,
		&loadSpy{sched: sched, loads: bed.loads},
		&schedulerLookups{sched: sched},
		&probeServices{bus: busName, ids: serviceIDs},
	)
	return bed
}

// addControllers adds ctrls to b until the test ends.
func addControllers(ctx context.Context, t *testing.T, b bus.Bus, ctrls ...controller.Controller) {
	for _, ctrl := range ctrls {
		rel, err := b.AddController(ctx, ctrl, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Cleanup(rel)
	}
}

// call invokes serviceID through the mux the scheduler builds for pluginID
// bound to bindingKey. It returns "reached on <bus>", "loaded at <key>" when the call
// loaded a plugin, or "refused: <reason>".
func (bed *fallbackBed) call(ctx context.Context, pluginID, bindingKey, serviceID string) string {
	mux, rel := bed.sched.buildPluginMux(ctx, pluginID, nil, nil, nil, nil, nil, nil, nil, false, false, bindingKey)
	defer rel()
	return bed.exec(ctx, srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))), serviceID)
}

// exec calls serviceID on client and reports the outcome as call does.
func (bed *fallbackBed) exec(ctx context.Context, client srpc.Client, serviceID string) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.ExecCall(ctx, serviceID, "Probe", srpc.NewRawMessage(nil, false), srpc.NewRawMessage(nil, false))
	}()
	select {
	case key := <-bed.loads:
		return "loaded at " + strconv.Quote(key)
	case err := <-errCh:
		if _, bus, ok := strings.Cut(errString(err), probeReached); ok {
			return "reached on " + bus
		}
		return "refused: " + errString(err)
	}
}

func errString(err error) string {
	if err == nil {
		return "<nil>"
	}
	return err.Error()
}

// storageVolumeServiceID is the service ID a plugin host storage volume's RPC
// server takes, which any plugin can compute.
func storageVolumeServiceID(storageID, storageVolumeID string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(storageID)) + "/" +
		base64.RawURLEncoding.EncodeToString([]byte(storageVolumeID)) + "/" +
		volume_rpc.SRPCAccessVolumesServiceID
}

// TestHostBusFallbackDefaultDeny checks a plugin outside the host set reaches
// no host bus service through its mux's fallback, and loads other plugins'
// services only at its own binding key, while a host plugin keeps the bus.
func TestHostBusFallbackDefaultDeny(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// The launcher service is a probe that restarts nothing.
	storageVol := storageVolumeServiceID("default", "victim")
	hostSvc := "probe.HostService"
	bed := newFallbackBed(ctx, t, le, tb.Bus, "app", &Config{HostExportPluginIds: []string{"spacewave-core"}}, []string{
		hostSvc, storageVol, spacewave_launcher.SRPCLauncherServiceID,
	})

	// Each plugin call names its own target, so no load is shared.
	escapes := []struct{ name, serviceID, pluginSvc string }{
		{"i1 another plugin by service ID", "", "probe.Svc"},
		{"i2 plugin-host passthrough", bldr_plugin.HostServiceIDPrefix + hostSvc, ""},
		{"i3 storage volume by service ID", storageVol, ""},
		{"i4 launcher", spacewave_launcher.SRPCLauncherServiceID, ""},
		{"b web view of another plugin", "", web_view.SRPCAccessWebViewsServiceID},
	}
	callers := []struct {
		name, pluginID, binding string
		host                    bool
	}{
		{"Space-keyed plugin", "attacker", "space-a", false},
		{"unkeyed plugin outside the host set", "attacker", "", false},
		{"host plugin", "spacewave-core", "", true},
	}
	for i, caller := range callers {
		for j, esc := range escapes {
			serviceID := esc.serviceID
			if esc.pluginSvc != "" {
				serviceID = bldr_plugin.PluginServiceID("other-"+strconv.Itoa(i*len(escapes)+j), esc.pluginSvc)
			}
			got := bed.call(ctx, caller.pluginID, caller.binding, serviceID)
			t.Logf("%s, %s: %s", caller.name, esc.name, got)

			// A plugin load stays at the caller's binding; anything else on
			// the bus is reached only by a host plugin.
			var ok bool
			switch {
			case esc.pluginSvc != "":
				ok = got == "loaded at "+strconv.Quote(caller.binding)
			case caller.host:
				ok = got == "reached on app"
			default:
				ok = strings.HasPrefix(got, "refused")
			}
			if !ok {
				t.Errorf("%s, %s: escaped: %s", caller.name, esc.name, got)
			}
		}
	}
}

// TestHostBusFallbackAllowsOwnSpaceWorld checks the Space scheduler's allowed
// host service reaches its own Space's World only, by exact service ID, and
// that the registered PluginHost service still answers a Space plugin. Each
// Space runs its scheduler on its own child bus, which serves its World and
// bridges no RPC service lookup to the parent or to another Space.
func TestHostBusFallbackAllowsOwnSpaceWorld(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	worldSvc := sdk_world_engine.SpaceWorldServiceID
	beds := make(map[string]*fallbackBed)
	for _, space := range []string{"space-a", "space-b"} {
		tb, err := testbed.NewTestbed(ctx, le)
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Cleanup(tb.Release)
		beds[space] = newFallbackBed(ctx, t, le, tb.Bus, space, &Config{AllowedHostServiceIds: []string{worldSvc}}, []string{
			worldSvc, "s4wave.world", worldSvc + ".Evil", "probe.Other",
		})
	}

	// A Space A plugin reaches Space A's World and nothing near its ID.
	bed := beds["space-a"]
	for _, tc := range []struct{ name, serviceID, want string }{
		{"own Space World", worldSvc, "reached on space-a"},
		{"prefix of the allowed ID", "s4wave.world", "refused"},
		{"suffix on the allowed ID", worldSvc + ".Evil", "refused"},
		{"plugin-host passthrough of the allowed ID", bldr_plugin.HostServiceIDPrefix + worldSvc, "refused"},
		{"plugin-host passthrough of another service", bldr_plugin.HostServiceIDPrefix + "probe.Other", "refused"},
	} {
		got := bed.call(ctx, "space-plugin", "space-a", tc.serviceID)
		t.Logf("Space A plugin, %s: %s", tc.name, got)
		if !strings.HasPrefix(got, tc.want) {
			t.Errorf("Space A plugin, %s: %s, want %s", tc.name, got, tc.want)
		}
	}

	// The registered PluginHost answers it with its own binding.
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	mux, rel := bed.sched.buildPluginMux(ctx, "space-plugin", nil, nil, nil, nil, nil, nil, nil, false, false, "space-a")
	defer rel()
	host := bldr_plugin.NewSRPCPluginHostClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
	info, err := host.GetPluginInfo(ctx, &bldr_plugin.GetPluginInfoRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("Space A plugin, PluginHost.GetPluginInfo: instance %q", info.GetInstanceKey())
	if info.GetInstanceKey() != "space-a" {
		t.Fatalf("GetPluginInfo: instance %q, want space-a", info.GetInstanceKey())
	}
}

// relayLoads resolves LoadPlugin on the host bus for the in-process plugins
// with their running clients and reports the binding key of any other load, as
// loadSpy.
type relayLoads struct {
	loadSpy
	plugins map[string]bldr_plugin.RunningPlugin
}

func (s *relayLoads) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	d, ok := di.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok || s.plugins[d.LoadPluginID()] == nil {
		return s.loadSpy.HandleDirective(ctx, di)
	}
	return directive.R(directive.NewValueResolver([]bldr_plugin.RunningPlugin{s.plugins[d.LoadPluginID()]}), nil)
}

// inProcessPlugin runs pluginID on its own bus with the real entrypoint
// controller, its host client bound to the scheduler's mux for it, and returns
// the bus and a client to its root mux.
func inProcessPlugin(ctx context.Context, t *testing.T, le *logrus.Entry, sched *Controller, pluginID string, ctrls ...controller.Controller) (bus.Bus, srpc.Client) {
	b, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	mux, rel := sched.buildPluginMux(ctx, pluginID, nil, nil, nil, nil, nil, nil, nil, false, false, "")
	t.Cleanup(rel)
	host := bldr_plugin.NewSRPCPluginHostClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
	ctrls = append(ctrls, plugin_entrypoint_controller.NewController(b, le, &bldr_plugin.PluginMeta{PluginId: pluginID}, host))
	addControllers(ctx, t, b, ctrls...)
	return b, srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(plugin_entrypoint.NewPluginRpcMux(le, b))))
}

// accessClient returns a client whose every call goes through the
// AccessRpcService at accessServiceID on client, asking for serviceID under
// serverID.
func accessClient(client srpc.Client, accessServiceID, serviceID, serverID string) srpc.Client {
	access := bifrost_rpc_access.NewSRPCAccessRpcServiceClientWithServiceID(client, accessServiceID)
	req := bifrost_rpc_access.NewLookupRpcServiceRequest(serviceID, serverID)
	return srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(srpc.NewMux(bifrost_rpc_access.NewProxyInvoker(access, req, true)))))
}

// TestHostBusFallbackHostPluginRelaysNothing checks a plugin outside the host
// set cannot use another plugin as a relay. A call it sends to spacewave-core
// is refused at once and reaches no host bus service and loads no plugin as
// core, while a service core itself serves still answers. The web plugin's
// forwarder carries the old unanchored "web-view/.*" pattern a target built
// before the anchored one sends; a "web-view/" server ID another plugin or the
// host names through it is refused at once, while the web plugin's own web
// view still reaches core. Core and the web plugin
// run in process with the real entrypoint controller and root mux, each on its
// own bus, with its host client bound to the scheduler's mux for it.
func TestHostBusFallbackHostPluginRelaysNothing(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// The app bus serves a host-only probe; core's bus serves its own. The web
	// plugin forwards its web views' calls to core, as HandleRpcViaPlugin does
	// for the Go compiler's handle-rpc config.
	const corePluginID, webPluginID = "spacewave-core", "web"
	hostSvc, coreSvc := "probe.HostService", "probe.CoreService"
	sched := NewController(le, tb.Bus, &Config{HostExportPluginIds: []string{corePluginID, webPluginID}})
	bed := &fallbackBed{sched: sched, loads: make(chan string, 8)}
	_, coreRoot := inProcessPlugin(ctx, t, le, sched, corePluginID, &probeServices{bus: "core", ids: []string{coreSvc}})
	webBus, webRoot := inProcessPlugin(ctx, t, le, sched, webPluginID)
	addControllers(ctx, t, webBus, bldr_web_plugin_controller.NewRpcForwarder(le, webBus, &plugin_forward_rpc_service.Config{
		PluginId:   corePluginID,
		ServerIdRe: "web-view/.*",
	}))
	addControllers(ctx, t, tb.Bus,
		&relayLoads{
			loadSpy: loadSpy{sched: sched, loads: bed.loads},
			plugins: map[string]bldr_plugin.RunningPlugin{
				corePluginID: bldr_plugin.NewRunningPlugin(coreRoot),
				webPluginID:  bldr_plugin.NewRunningPlugin(webRoot),
			},
		},
		&schedulerLookups{sched: sched},
		&probeServices{bus: "app", ids: []string{hostSvc}},
	)

	// accessVia asks pluginID's AccessRpcService, as the caller, for
	// serviceID under serverID.
	via := bldr_plugin.PluginServiceID
	accessVia := func(pluginID, serviceID, serverID string) func(srpc.Client) srpc.Client {
		return func(client srpc.Client) srpc.Client {
			return accessClient(client, via(pluginID, bifrost_rpc_access.SRPCAccessRpcServiceServiceID), serviceID, serverID)
		}
	}
	const relayed, shipped = "relayed", "shipped"
	hostCall := bldr_plugin.HostServiceIDPrefix + hostSvc
	cases := []struct {
		name, serviceID string
		wrap            func(srpc.Client) srpc.Client
		want            string
	}{
		{"plugin-host service through core", via(corePluginID, hostCall), nil, relayed},
		{"another plugin through core", "", nil, relayed},
		{"AccessRpcService body, empty server", hostCall, accessVia(corePluginID, hostCall, ""), relayed},
		{"AccessRpcService body, default server", hostCall, accessVia(corePluginID, hostCall, "default"), relayed},
		{"web view server through the web plugin", coreSvc, accessVia(webPluginID, coreSvc, "web-view/x"), relayed},
		{"core's own service", via(corePluginID, coreSvc), nil, shipped},
	}
	for i, caller := range []struct{ name, binding string }{
		{"unkeyed plugin outside the host set", ""},
		{"Space-keyed plugin", "space-a"},
	} {
		mux, rel := sched.buildPluginMux(ctx, "attacker", nil, nil, nil, nil, nil, nil, nil, false, false, caller.binding)
		client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
		for _, tc := range cases {
			c := client
			if tc.wrap != nil {
				c = tc.wrap(client)
			}
			// Each caller names its own plugin, so no load is shared.
			serviceID := tc.serviceID
			if serviceID == "" {
				serviceID = via(corePluginID, via("other-"+strconv.Itoa(i), "probe.Svc"))
			}
			bed.expect(t, caller.name+", "+tc.name, bed.timedExec(ctx, c, serviceID), tc.want)
		}
		rel()
	}

	// The host's own calls: a web view of the web plugin still reaches core,
	// and a root mux names a "web-view/" server ID as the host's, so neither
	// core nor the web plugin's forwarder relays it.
	webView := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(srpc.NewMux(bifrost_rpc.NewInvoker(webBus, web_view.WebViewServerID("1"), true)))))
	bed.expect(t, "web plugin's own web view", bed.timedExec(ctx, webView, coreSvc), shipped)
	rootBody := accessClient(coreRoot, bifrost_rpc_access.SRPCAccessRpcServiceServiceID, hostCall, "web-view/x")
	bed.expect(t, "root AccessRpcService body, web view server", bed.timedExec(ctx, rootBody, hostCall), relayed)
	webRootBody := accessClient(webRoot, bifrost_rpc_access.SRPCAccessRpcServiceServiceID, coreSvc, "web-view/x")
	bed.expect(t, "web plugin root AccessRpcService body, web view server", bed.timedExec(ctx, webRootBody, coreSvc), relayed)
}

// timedResult is one call's outcome, as exec reports it, and how long it took.
type timedResult struct {
	got  string
	took time.Duration
}

// timedExec runs exec and measures it.
func (bed *fallbackBed) timedExec(ctx context.Context, client srpc.Client, serviceID string) timedResult {
	start := time.Now()
	got := bed.exec(ctx, client, serviceID)
	return timedResult{got: got, took: time.Since(start)}
}

// expect checks one relay test outcome. A relayed call must be refused with
// ErrRelayRefused well before exec's deadline; a shipped call must reach core.
func (bed *fallbackBed) expect(t *testing.T, name string, r timedResult, want string) {
	t.Logf("%s: %s (%s)", name, r.got, r.took.Round(time.Millisecond))
	switch want {
	case "shipped":
		if r.got != "reached on core" {
			t.Errorf("%s: shipped flow broke: %s", name, r.got)
		}
	case "relayed":
		if !strings.Contains(r.got, bldr_plugin.ErrRelayRefused.Error()) || r.took > 500*time.Millisecond {
			t.Errorf("%s: want a refusal at once, got %s after %s", name, r.got, r.took)
		}
	}
}
