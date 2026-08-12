package plugin_host_configset

import (
	"context"
	"strings"
	"testing"
	"time"

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
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
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
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

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

	serverExited := make(chan error, 1)
	server := &observingPluginHostServer{
		SRPCPluginHostServer: bldr_plugin_host.NewPluginHostServer(ctx, hostBus, le, "spacewave-launcher", nil, nil),
		exited:               serverExited,
	}
	mux := srpc.NewMux()
	if err := bldr_plugin.SRPCRegisterPluginHost(mux, server); err != nil {
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

	conf := &Config{ConfigSet: map[string]*configset_proto.ControllerConfig{
		"release-world-fetch": {Id: (&config.Placeholder{}).GetConfigID(), Rev: 1},
	}}
	baseFactory := NewFactory(pluginBus)
	loaded, err := baseFactory.Construct(ctx, conf, controller.ConstructOpts{Logger: le})
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.(*Controller).Execute(ctx)
	if got == nil || !strings.Contains(got.Error(), "injected controller failure") {
		t.Fatalf("Execute error = %v, want injected controller failure", got)
	}
	select {
	case <-serverExited:
	case <-time.After(time.Second):
		t.Fatal("host ExecController remained active after member error return")
	}
}

func TestPluginHostExecControllerReportsMemberError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
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
	mux := srpc.NewMux()
	if err := bldr_plugin.SRPCRegisterPluginHost(
		mux,
		bldr_plugin_host.NewPluginHostServer(ctx, hostBus, le, "spacewave-launcher", nil, nil),
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
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := web_runtime.IsWebRuntimeClientClosed(test.err); got != test.want {
				t.Fatalf("isWebRuntimeClientClosed() = %v, want %v", got, test.want)
			}
		})
	}
}
