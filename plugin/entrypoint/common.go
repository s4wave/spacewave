package plugin_entrypoint

import (
	"context"
	"io/fs"

	"github.com/aperturerobotics/bldr/core"
	"github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	bldr_rpc "github.com/aperturerobotics/bldr/rpc"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// AddFactoryFunc is a callback to add a factory.
type AddFactoryFunc func(b bus.Bus) []controller.Factory

// BuildConfigSetFunc is a function to build a list of ConfigSet to apply.
type BuildConfigSetFunc func(ctx context.Context, b bus.Bus, le *logrus.Entry) ([]configset.ConfigSet, error)

// ExecutePlugin builds the bus & starts common controllers.
func ExecutePlugin(
	ctx context.Context,
	le *logrus.Entry,
	addFactoryFuncs []AddFactoryFunc,
	configSetFuncs []BuildConfigSetFunc,
	muxedConn network.MuxedConn,
) error {
	var rels []func()
	rel := func() {
		for _, rel := range rels {
			rel()
		}
	}

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}
	for _, fn := range addFactoryFuncs {
		if fn != nil {
			for _, factory := range fn(b) {
				sr.AddFactory(factory)
			}
		}
	}

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
	rels = append(rels, csRef.Release)

	// load root config sets
	var configSets []configset.ConfigSet
	for _, configSetFn := range configSetFuncs {
		confSets, err := configSetFn(ctx, b, le)
		if err != nil {
			rel()
			return err
		}
		configSets = append(configSets, confSets...)
	}

	// apply config sets
	mergedConfigSet := configset.MergeConfigSets(configSets...)
	if len(mergedConfigSet) != 0 {
		_, csetRef, err := b.AddDirective(configset.NewApplyConfigSet(mergedConfigSet), nil)
		if err != nil {
			rel()
			return err
		}
		rels = append(rels, csetRef.Release)
	}

	// construct plugin host
	pluginHostClient := srpc.NewClientWithMuxedConn(muxedConn)
	pluginHostClientCtrl := bldr_rpc.NewClientController(
		le,
		b,
		controller.NewInfo("plugin/entrypoint/client", Version, "plugin entrypoint rpc client"),
		pluginHostClient,
		[]string{plugin.HostServiceIDPrefix},
	)
	pluginHostRel, err := b.AddController(ctx, pluginHostClientCtrl, nil)
	if err != nil {
		rel()
		return err
	}
	rels = append(rels, pluginHostRel)

	// lookup the plugin information
	pluginHost := plugin.NewSRPCPluginHostClient(pluginHostClient)
	pluginInfo, err := pluginHost.GetPluginInfo(ctx, &plugin.GetPluginInfoRequest{})
	if err != nil {
		rel()
		return err
	}
	le.Infof(
		"plugin information received from host w/ manifest: %s",
		pluginInfo.GetPluginManifest().MarshalString(),
	)

	// handle PluginFetch requests via bus PluginFetch.
	fetchViaBus := plugin_host.NewPluginFetchViaBusController(le, b)
	fetchViaBusRel, err := b.AddController(ctx, fetchViaBus, nil)
	if err != nil {
		rel()
		return err
	}
	rels = append(rels, fetchViaBusRel)

	// construct the rpc client controller
	// listen for incoming requests
	errCh := make(chan error, 1)
	go func() {
		// use bus to invoke services
		srv := srpc.NewServer(bldr_rpc.NewInvoker(b, plugin.HostClientID))
		errCh <- srv.AcceptMuxedConn(ctx, muxedConn)
	}()

	select {
	case <-ctx.Done():
		rel()
		return context.Canceled
	case err := <-errCh:
		rel()
		return err
	}
}

// ConfigSetFuncFromFS builds a ConfigSetFunc which parses a file in a FS as a ConfigSet.
func ConfigSetFuncFromFS(ifs fs.FS, fileName string) BuildConfigSetFunc {
	return func(ctx context.Context, b bus.Bus, le *logrus.Entry) ([]configset.ConfigSet, error) {
		data, err := fs.ReadFile(ifs, fileName)
		if err != nil {
			return nil, err
		}
		set := &configset_proto.ConfigSet{}
		if err := set.UnmarshalVT(data); err != nil {
			return nil, err
		}
		cset, err := set.Resolve(ctx, b)
		if err != nil {
			return nil, err
		}
		return []configset.ConfigSet{cset}, nil
	}
}
