//go:build !js
// +build !js

package plugin_entrypoint

import (
	"context"
	"os"
	"os/signal"

	"github.com/aperturerobotics/bifrost/util/rwc"
	"github.com/aperturerobotics/bldr/core"
	"github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// AddFactoryFunc is a callback to add a factory.
type AddFactoryFunc func(b bus.Bus, sr *static.Resolver)

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

	ctx, ctxCancel := signal.NotifyContext(context.Background(), os.Interrupt)
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
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}
	for _, fn := range addFactoryFuncs {
		if fn != nil {
			fn(b, sr)
		}
	}

	// TODO: remove
	os.Stderr.WriteString("hello from plugin\n")

	// construct mplex
	inOutRwc := rwc.NewReadWriteCloser(os.Stdin, os.Stdout)
	muxedConn, err := srpc.NewMuxedConn(inOutRwc, true)
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

	// load configset controller
	_, _, csRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&configset_controller.Config{}),
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "construct configset controller")
	}
	defer csRef.Release()

	// load root config sets
	var configSets []configset.ConfigSet
	for _, configSetFn := range configSetFuncs {
		confSets, err := configSetFn(ctx, b, le)
		if err != nil {
			return err
		}
		configSets = append(configSets, confSets...)
	}

	// apply config sets
	mergedConfigSet := configset.MergeConfigSets(configSets...)
	if len(mergedConfigSet) != 0 {
		_, csetRef, err := b.AddDirective(configset.NewApplyConfigSet(mergedConfigSet), nil)
		if err != nil {
			return err
		}
		defer csetRef.Release()
	}

	// listen for incoming requests
	mux := srpc.NewMux()
	_ = plugin.SRPCRegisterPluginFetch(mux, plugin_host.NewPluginFetchViaBus(le, b))

	srv := srpc.NewServer(mux)
	return srv.AcceptMuxedConn(ctx, muxedConn)
}
