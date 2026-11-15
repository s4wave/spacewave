package plugin_host_configset

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	bldr_plugin_load "github.com/s4wave/spacewave/bldr/plugin/load"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

type observingPluginHostServer struct {
	bldr_plugin.SRPCPluginHostServer
	exited chan error
}

func (s *observingPluginHostServer) ExecController(
	req *controller_exec.ExecControllerRequest,
	stream bldr_plugin.SRPCPluginHost_ExecControllerStream,
) error {
	err := s.SRPCPluginHostServer.ExecController(req, stream)
	s.exited <- err
	return err
}

type failingFactory struct{}

func (*failingFactory) GetConfigID() string {
	return (&config.Placeholder{}).GetConfigID()
}

func (*failingFactory) ConstructConfig() config.Config {
	return &config.Placeholder{}
}

func (*failingFactory) GetVersion() controller.Version {
	return controller.MustParseVersion("0.0.1")
}

func (*failingFactory) Construct(context.Context, config.Config, controller.ConstructOpts) (controller.Controller, error) {
	return &failingController{}, nil
}

type failingController struct{}

func (*failingController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/failing", controller.MustParseVersion("0.0.1"), "")
}

func (*failingController) Execute(context.Context) error {
	return errors.New("injected controller failure")
}

func (*failingController) HandleDirective(context.Context, directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}
func (*failingController) Close() error { return nil }

func TestControllerReturnsConfigSetMemberError(t *testing.T) {
	// Bound the test with a three second timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Start the host bus with the failing factory and config set controller.
	hostBus, staticResolver, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	staticResolver.AddFactory(&failingFactory{})
	configSetController, err := configset_controller.NewController(le, hostBus)
	if err != nil {
		t.Fatal(err)
	}
	releaseConfigSet, err := hostBus.AddController(ctx, configSetController, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseConfigSet()

	// Serve the plugin host over an in-memory pipe.
	serverExited := make(chan error, 1)
	server := &observingPluginHostServer{
		SRPCPluginHostServer: newHostServer(ctx, hostBus, le, "spacewave-core", ""),
		exited:               serverExited,
	}
	mux := srpc.NewMux()
	if err := bldr_plugin.SRPCRegisterPluginHost(mux, server); err != nil {
		t.Fatal(err)
	}
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))

	// Bridge the plugin bus to the host through the client controller.
	pluginBus, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	clientController := bifrost_rpc.NewClientController(
		le,
		pluginBus,
		controller.NewInfo("test/plugin-host-client", controller.MustParseVersion("0.0.1"), ""),
		client,
		[]string{bldr_plugin.HostServiceIDPrefix},
	)
	releaseClient, err := pluginBus.AddController(ctx, clientController, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseClient()

	// Configure the controller with one failing member config.
	conf := &Config{ConfigSet: map[string]*configset_proto.ControllerConfig{
		"release-world-fetch": {Id: (&config.Placeholder{}).GetConfigID(), Rev: 1},
	}}

	// The controller must surface the member controller's failure.
	baseFactory := NewFactory(pluginBus)
	loaded, err := baseFactory.Construct(ctx, conf, controller.ConstructOpts{Logger: le})
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.(*Controller).Execute(ctx)
	if got == nil || !strings.Contains(got.Error(), "injected controller failure") {
		t.Fatalf("Execute error = %v, want injected controller failure", got)
	}

	// The host ExecController must have exited.
	select {
	case <-serverExited:
	case <-time.After(time.Second):
		t.Fatal("host ExecController remained active after member error return")
	}
}

func TestPluginHostExecControllerReportsMemberError(t *testing.T) {
	// Bound the test with a three second timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Start the host bus with the failing factory and config set controller.
	hostBus, staticResolver, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	staticResolver.AddFactory(&failingFactory{})
	configSetController, err := configset_controller.NewController(le, hostBus)
	if err != nil {
		t.Fatal(err)
	}
	releaseConfigSet, err := hostBus.AddController(ctx, configSetController, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseConfigSet()

	// Serve the plugin host and run the failing config set.
	mux := srpc.NewMux()
	if err := bldr_plugin.SRPCRegisterPluginHost(
		mux,
		newHostServer(ctx, hostBus, le, "spacewave-core", ""),
	); err != nil {
		t.Fatal(err)
	}
	client := bldr_plugin.NewSRPCPluginHostClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
	status, err := client.ExecController(ctx, &controller_exec.ExecControllerRequest{ConfigSet: &configset_proto.ConfigSet{
		Configs: map[string]*configset_proto.ControllerConfig{
			"release-world-fetch": {Id: (&config.Placeholder{}).GetConfigID(), Rev: 1},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer status.Close()

	// The stream must report the injected member error.
	for {
		response, err := status.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if response.GetError() == nil {
			continue
		}
		if response.GetId() != "release-world-fetch" || !strings.Contains(response.GetError().Error(), "injected controller failure") {
			t.Fatalf("ExecController response = %#v, want release-world-fetch injected error", response)
		}
		break
	}
}

// testHostExports is the host's own plugin set in these tests.
var testHostExports = []string{"spacewave-core"}

// newHostServer serves pluginID, bound to binding, on a host whose own plugin
// set is testHostExports.
func newHostServer(ctx context.Context, hostBus bus.Bus, le *logrus.Entry, pluginID, binding string) *bldr_plugin_host.PluginHostServer {
	server := bldr_plugin_host.NewPluginHostServer(ctx, hostBus, le, pluginID, "", nil, nil, "", false)
	server.SetRegistrationInstanceKey(binding)
	server.SetHostExportPluginIDs(testHostExports)
	return server
}

// TestExecControllerRefusesNonHostPlugin runs the config sets that escaped a
// plugin's scope through the host's ExecController: every one is refused for
// a plugin the host did not start, keyed or not, and the host's own plugin
// still runs them.
func TestExecControllerRefusesNonHostPlugin(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// The host bus can run each escaping config.
	hostBus, staticResolver, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	staticResolver.AddFactory(volume_rpc_server.NewFactory(hostBus))
	configSetController, err := configset_controller.NewController(le, hostBus)
	if err != nil {
		t.Fatal(err)
	}
	releaseConfigSet, err := hostBus.AddController(ctx, configSetController, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseConfigSet()

	// exec runs one config as pluginID bound to binding and returns the first
	// error the stream reports, or nil once the config reports a status.
	exec := func(pluginID, binding, id, conf string) error {
		mux := srpc.NewMux()
		if err := bldr_plugin.SRPCRegisterPluginHost(mux, newHostServer(ctx, hostBus, le, pluginID, binding)); err != nil {
			t.Fatal(err)
		}
		client := bldr_plugin.NewSRPCPluginHostClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux))))
		status, err := client.ExecController(ctx, &controller_exec.ExecControllerRequest{ConfigSet: &configset_proto.ConfigSet{
			Configs: map[string]*configset_proto.ControllerConfig{"escape": {Id: id, Rev: 1, Config: []byte(conf)}},
		}})
		if err != nil {
			return err
		}
		defer status.Close()
		resp, err := status.Recv()
		if err != nil {
			return err
		}
		return resp.GetError()
	}

	escapes := []struct{ name, id, conf string }{
		{"every host volume with its private key", volume_rpc_server.ConfigID, `{"serviceId":"evil/rpc.volume.AccessVolumes","volumeIdRe":".*","exposePrivateKey":true}`},
		{"the approvals volume", volume_rpc_server.ConfigID, `{"serviceId":"evil2/rpc.volume.AccessVolumes","volumeIdList":["platform-account"],"exposePrivateKey":true}`},
		{"an unkeyed instance of itself", bldr_plugin_load.ConfigID, `{"pluginId":"attacker"}`},
		{"another plugin's storage volume", storage_volume.ConfigID, `{"storageId":"default","storageVolumeId":"other-plugin/state"}`},
	}
	callers := []struct{ name, pluginID, binding string }{
		{"Space-keyed plugin", "attacker", "space-a"},
		{"unkeyed plugin outside the host set", "attacker", ""},
	}
	for _, caller := range callers {
		for _, escape := range escapes {
			err := exec(caller.pluginID, caller.binding, escape.id, escape.conf)
			if !bldr_plugin.IsNotHostPlugin(err) {
				t.Fatalf("%s, %s: escaped (err = %v)", caller.name, escape.name, err)
			}
			t.Logf("%s, %s: refused", caller.name, escape.name)
		}
	}

	// The host's own plugin still runs a host config.
	if err := exec("spacewave-core", "", volume_rpc_server.ConfigID, `{"serviceId":"core/rpc.volume.AccessVolumes","volumeIdList":["none"]}`); err != nil {
		t.Fatalf("spacewave-core host config: %v", err)
	}
	t.Log("spacewave-core host config: applied")
}

// TestControllerRefusesSpacePluginHostConfigSet runs a Space-keyed plugin's
// hostConfigSet: the controller ends at once with the typed refusal, which the
// plugin entrypoint reports as the plugin's failure, and no member runs.
func TestControllerRefusesSpacePluginHostConfigSet(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// The host could run the member if it were asked to.
	hostBus, staticResolver, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	staticResolver.AddFactory(&failingFactory{})
	configSetController, err := configset_controller.NewController(le, hostBus)
	if err != nil {
		t.Fatal(err)
	}
	releaseConfigSet, err := hostBus.AddController(ctx, configSetController, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseConfigSet()

	// Bridge the Space plugin's bus to the host.
	mux := srpc.NewMux()
	if err := bldr_plugin.SRPCRegisterPluginHost(mux, newHostServer(ctx, hostBus, le, "space-plugin", "space-a")); err != nil {
		t.Fatal(err)
	}
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))
	pluginBus, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	clientController := bifrost_rpc.NewClientController(
		le,
		pluginBus,
		controller.NewInfo("test/plugin-host-client", controller.MustParseVersion("0.0.1"), ""),
		client,
		[]string{bldr_plugin.HostServiceIDPrefix},
	)
	releaseClient, err := pluginBus.AddController(ctx, clientController, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseClient()

	// The plugin's hostConfigSet is refused with the typed reason.
	conf := &Config{ConfigSet: map[string]*configset_proto.ControllerConfig{
		"release-world-fetch": {Id: (&config.Placeholder{}).GetConfigID(), Rev: 1},
	}}
	loaded, err := NewFactory(pluginBus).Construct(ctx, conf, controller.ConstructOpts{Logger: le})
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.(*Controller).Execute(ctx)
	if !bldr_plugin.IsNotHostPlugin(got) {
		t.Fatalf("Execute error = %v, want %v", got, bldr_plugin.ErrNotHostPlugin)
	}
	if err := loaded.(*Controller).WaitAccepted(ctx); !bldr_plugin.IsNotHostPlugin(err) {
		t.Fatalf("WaitAccepted = %v, want %v", err, bldr_plugin.ErrNotHostPlugin)
	}
	t.Logf("Space-keyed plugin hostConfigSet: refused: %v", got)
}

func TestIsWebRuntimeClientClosed(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "web runtime instance closed",
			err:  errors.New("WebRuntimeClientInstance closed: plugin/spacewave-core"),
			want: true,
		},
		{
			name: "runtime client normal close",
			err:  errors.New("RuntimeClientClosedError: WebRuntimeClient: plugin/spacewave-core: runtime client generation 1 closed: normal-close"),
			want: true,
		},
		{
			name: "runtime client normal close without error name",
			err:  errors.New("WebRuntimeClient: plugin/spacewave-core: runtime client generation 12 closed: normal-close"),
			want: true,
		},
		{
			name: "runtime client normal close wrapped",
			err:  errors.New("apply configset: RuntimeClientClosedError: WebRuntimeClient: plugin/spacewave-core: runtime client generation 1 closed: normal-close"),
			want: true,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "generic instance closed",
			err:  errors.New("WebRuntimeClientInstance is closed"),
			want: false,
		},
		{
			name: "runtime client runtime error",
			err:  errors.New("RuntimeClientClosedError: WebRuntimeClient: plugin/spacewave-core: runtime client generation 1 closed: runtime-error"),
			want: false,
		},
		{
			name: "runtime client malformed generation",
			err:  errors.New("RuntimeClientClosedError: WebRuntimeClient: plugin/spacewave-core: runtime client generation first closed: normal-close"),
			want: false,
		},
		{
			name: "runtime client missing close reason",
			err:  errors.New("RuntimeClientClosedError: WebRuntimeClient: plugin/spacewave-core: runtime client generation 1 closed"),
			want: false,
		},
	}
	// Each error must match its expected close classification.
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := web_runtime.IsWebRuntimeClientClosed(test.err); got != test.want {
				t.Fatalf("isWebRuntimeClientClosed() = %v, want %v", got, test.want)
			}
		})
	}
}
