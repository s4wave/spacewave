//go:build !js
// +build !js

package plugin_entrypoint

import (
	"context"
	"os"
	"os/signal"

	"github.com/aperturerobotics/bifrost/util/rwc"
	"github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

// AddFactoryFunc is a callback to add a factory.
type AddFactoryFunc func(b bus.Bus) []controller.Factory

// BuildConfigSetFunc is a function to build a list of ConfigSet to apply.
type BuildConfigSetFunc func(ctx context.Context, b bus.Bus, le *logrus.Entry) ([]configset.ConfigSet, error)

// Main runs the default main entrypoint for a program.
func Main(addFactoryFuncs []AddFactoryFunc, configSetFuncs []BuildConfigSetFunc) {
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		DisableColors:    true,
		DisableTimestamp: true,
	})
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	ctx, ctxCancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer ctxCancel()

	if err := Run(ctx, le, addFactoryFuncs, configSetFuncs); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

// Run runs the plugin entrypoint.
func Run(
	ctx context.Context,
	le *logrus.Entry,
	addFactoryFuncs []AddFactoryFunc,
	configSetFuncs []BuildConfigSetFunc,
) error {
	b, _, rel, err := StartCoreBus(ctx, le, addFactoryFuncs, configSetFuncs)
	if err != nil {
		return err
	}
	defer rel()

	// TODO: remove
	os.Stderr.WriteString("hello from plugin\n")

	// construct mplex
	inOutRwc := rwc.NewReadWriteCloser(os.Stdin, os.Stdout)
	muxedConn, err := srpc.NewMuxedConn(inOutRwc, false)
	if err != nil {
		return err
	}
	defer muxedConn.Close()

	// construct plugin host client
	client := srpc.NewClientWithMuxedConn(muxedConn)
	pluginHostClient := plugin.NewSRPCPluginHostClient(client)

	// load demo-plugin
	// TODO: remove
	go func() {
		_, err := pluginHostClient.LoadPlugin(ctx, &plugin.LoadPluginRequest{
			PluginId: "sandbox-demo-plugin",
		})
		if err != nil && err != context.Canceled {
			os.Stderr.WriteString(err.Error() + "\n")
		}
	}()

	// configure rpc mux
	mux := srpc.NewMux()
	_ = plugin.SRPCRegisterPluginFetch(mux, plugin_host.NewPluginFetchViaBus(le, b))

	// listen for incoming requests
	errCh := make(chan error, 1)
	go func() {
		srv := srpc.NewServer(mux)
		errCh <- srv.AcceptMuxedConn(ctx, muxedConn)
	}()

	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-errCh:
		return err
	}
}
