//go:build !js
// +build !js

package plugin_entrypoint

import (
	"context"
	"os"
	"os/signal"

	"github.com/aperturerobotics/bifrost/util/rwc"
	"github.com/aperturerobotics/bldr/plugin"
	bldr_rpc "github.com/aperturerobotics/bldr/rpc"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// Version is the entrypoint version
var Version = semver.MustParse("0.0.1")

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

	// construct mplex
	inOutRwc := rwc.NewReadWriteCloser(os.Stdin, os.Stdout)
	muxedConn, err := srpc.NewMuxedConnWithRwc(ctx, inOutRwc, false)
	if err != nil {
		return err
	}
	defer muxedConn.Close()

	// lookup the plugin information
	pluginHostClient := srpc.NewClientWithMuxedConn(muxedConn)
	pluginHost := plugin.NewSRPCPluginHostClient(pluginHostClient)
	pluginInfo, err := pluginHost.GetPluginInfo(ctx, &plugin.GetPluginInfoRequest{})
	if err != nil {
		return err
	}
	le.Infof(
		"plugin information received from host w/ manifest: %s",
		pluginInfo.GetPluginManifest().MarshalString(),
	)

	pluginHostClientCtrl := bldr_rpc.NewClientController(
		le,
		b,
		controller.NewInfo("plugin/entrypoint/client", Version, "plugin entrypoint rpc client"),
		pluginHostClient,
		[]string{"plugin-host/"},
	)
	pluginHostRel, err := b.AddController(ctx, pluginHostClientCtrl, nil)
	if err != nil {
		return err
	}
	defer pluginHostRel()

	// load demo-plugin
	// TODO: remove
	/*
		go func() {
			_, err := pluginHostClient.LoadPlugin(ctx, &plugin.LoadPluginRequest{
				PluginId: "sandbox-demo-plugin",
			})
			if err != nil && err != context.Canceled {
				os.Stderr.WriteString(err.Error() + "\n")
			}
		}()
	*/

	// configure rpc mux
	// TODO: implement as a controller
	// mux := srpc.NewMux()
	// _ = plugin.SRPCRegisterPluginFetch(mux, plugin_host.NewPluginFetchViaBus(le, b))

	// listen for incoming requests
	errCh := make(chan error, 1)
	go func() {
		// use bus to invoke services
		srv := srpc.NewServer(bldr_rpc.NewInvoker(b, "plugin-host"))
		errCh <- srv.AcceptMuxedConn(ctx, muxedConn)
	}()

	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-errCh:
		return err
	}
}
