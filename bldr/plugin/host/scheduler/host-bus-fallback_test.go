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
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	web_view "github.com/s4wave/spacewave/bldr/web/view"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/db/testbed"
	volume_rpc "github.com/s4wave/spacewave/db/volume/rpc"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
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
	for _, ctrl := range []controller.Controller{
		&loadSpy{sched: sched, loads: bed.loads},
		&schedulerLookups{sched: sched},
		&probeServices{bus: busName, ids: serviceIDs},
	} {
		rel, err := b.AddController(ctx, ctrl, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		t.Cleanup(rel)
	}
	return bed
}

// call invokes serviceID through the mux the scheduler builds for pluginID
// bound to bindingKey. It returns "reached on <bus>", "loaded at <key>" when the call
// loaded a plugin, or "refused: <reason>".
func (bed *fallbackBed) call(ctx context.Context, pluginID, bindingKey, serviceID string) string {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	mux, rel := bed.sched.buildPluginMux(ctx, pluginID, nil, nil, nil, nil, nil, nil, nil, false, false, bindingKey)
	defer rel()
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
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
