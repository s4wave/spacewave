package plugin_entrypoint

import (
	"context"
	"io/fs"
	"strings"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	bifrost_rpc_access "github.com/aperturerobotics/bifrost/rpc/access"
	"github.com/aperturerobotics/bldr/core"
	manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_assets_http "github.com/aperturerobotics/bldr/plugin/assets/http"
	plugin_entrypoint_controller "github.com/aperturerobotics/bldr/plugin/entrypoint/controller"
	plugin_host_configset "github.com/aperturerobotics/bldr/plugin/host/configset"
	plugin_host_storage "github.com/aperturerobotics/bldr/plugin/host/storage"
	plugin_host_storage_volume "github.com/aperturerobotics/bldr/plugin/host/storage/volume"
	vardef "github.com/aperturerobotics/bldr/plugin/vardef"
	"github.com/aperturerobotics/bldr/storage"
	storage_controller "github.com/aperturerobotics/bldr/storage/controller"
	web_fetch_service "github.com/aperturerobotics/bldr/web/fetch/service"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	unixfs_access "github.com/aperturerobotics/hydra/unixfs/access"
	unixfs_rpc "github.com/aperturerobotics/hydra/unixfs/rpc"
	unixfs_rpc_client "github.com/aperturerobotics/hydra/unixfs/rpc/client"
	volume_rpc_client "github.com/aperturerobotics/hydra/volume/rpc/client"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

// AddFactoryFunc is a callback to add a factory.
type AddFactoryFunc func(b bus.Bus) []controller.Factory

// BuildConfigSetFunc is a function to build a list of ConfigSet to apply.
type BuildConfigSetFunc func(ctx context.Context, b bus.Bus, le *logrus.Entry) ([]configset.ConfigSet, error)

// ExecutePlugin builds the bus & starts common controllers.
func ExecutePlugin(
	rctx context.Context,
	le *logrus.Entry,
	meta *bldr_plugin.PluginMeta,
	addFactoryFuncs []AddFactoryFunc,
	configSetFuncs []BuildConfigSetFunc,
	pluginHostClient srpc.Client,
	acceptPluginHostStreams func(ctx context.Context, srv *srpc.Server) error,
) error {
	var rels []func()
	rel := func() {
		for _, rel := range rels {
			rel()
		}
	}

	// cancel the root context when exiting
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	// attach the plugin info to the context
	ctx = bldr_plugin.WithPluginContextInfo(
		ctx,
		bldr_plugin.NewPluginContextInfo(meta.CloneVT()),
	)

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}

	// add built-in factories
	sr.AddFactory(plugin_assets_http.NewFactory(b))
	sr.AddFactory(plugin_host_configset.NewFactory(b))
	sr.AddFactory(plugin_host_storage_volume.NewFactory(b))

	// add provided factories
	for _, fn := range addFactoryFuncs {
		if fn != nil {
			for _, factory := range fn(b) {
				sr.AddFactory(factory)
			}
		}
	}

	// start the node controller.
	nodeCtrl := node_controller.NewController(nil, le, b)
	nodeCtrlRel, err := b.AddController(ctx, nodeCtrl, nil)
	if err != nil {
		rel()
		return err
	}
	rels = append(rels, nodeCtrlRel)

	// load configset controller
	csCtrl, err := configset_controller.NewController(le, b)
	if err != nil {
		return err
	}
	csRel, err := b.AddController(
		ctx,
		csCtrl,
		nil,
	)
	if err != nil {
		return err
	}
	rels = append(rels, csRel)

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

	// start the plugin entrypoint controller
	pluginHost := bldr_plugin.NewSRPCPluginHostClient(pluginHostClient)
	pluginEntryCtrl := plugin_entrypoint_controller.NewController(b, le, meta, pluginHost)
	pluginEntryCtrlRel, err := b.AddController(ctx, pluginEntryCtrl, nil)
	if err != nil {
		rel()
		return err
	}
	rels = append(rels, pluginEntryCtrlRel)

	// handle Fetch requests via bus Fetch
	webFetchViaBus := web_fetch_service.NewController(le, b, &web_fetch_service.Config{
		// NotFoundIfIdle: true,
	})
	webFetchViaBusRel, err := b.AddController(ctx, webFetchViaBus, nil)
	if err != nil {
		rel()
		return err
	}
	rels = append(rels, webFetchViaBusRel)

	// lookup the plugin information
	pluginInfo, err := pluginHost.GetPluginInfo(ctx, &bldr_plugin.GetPluginInfoRequest{})
	if err != nil {
		rel()
		return err
	}
	pluginManifestRef := pluginInfo.GetManifestRef()
	le.Infof(
		"plugin information received from host w/ manifest: %s",
		pluginManifestRef.GetManifestRef().MarshalString(),
	)

	// errCh will interrupt the program
	errCh := make(chan error, 5)
	handleErr := func(err error) {
		select {
		case errCh <- err:
		default:
		}
	}

	// serve the host volume proxy controller
	hostVolumeInfo := pluginInfo.GetHostVolumeInfo()
	hostVolumeController := volume_rpc_client.NewProxyVolumeControllerWithClient(
		b,
		le,
		hostVolumeInfo,
		[]string{bldr_plugin.PluginVolumeID},
		pluginHostClient,
		bldr_plugin.HostVolumeServiceIDPrefix,
	)
	relHostVolumeController, err := b.AddController(ctx, hostVolumeController, handleErr)
	if err != nil {
		rel()
		return err
	}
	rels = append(rels, relHostVolumeController)

	// serve the plugin assets filesystem
	pluginAssetsFsCtrl := BuildPluginAssetsFSController(le, b, pluginHostClient)
	relPluginAssetsFsCtrl, err := b.AddController(ctx, pluginAssetsFsCtrl, handleErr)
	if err != nil {
		rel()
		return err
	}
	rels = append(rels, relPluginAssetsFsCtrl)

	// serve the plugin dist filesystem
	pluginDistFsCtrl := BuildPluginDistFSController(le, b, pluginHostClient)
	relPluginDistFsCtrl, err := b.AddController(ctx, pluginDistFsCtrl, handleErr)
	if err != nil {
		rel()
		return err
	}
	rels = append(rels, relPluginDistFsCtrl)

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

	// construct the rpc mux
	rpcMux := srpc.NewMux(bifrost_rpc.NewInvoker(b, bldr_plugin.HostServerIDPrefix+"default", true))

	// handle ManifestFetch requests via bus ManifestFetch.
	pluginFetchViaBus := manifest.NewManifestFetchViaBus(le, b)
	_ = manifest.SRPCRegisterManifestFetch(rpcMux, pluginFetchViaBus)

	// handle AccessRpcService requests via bus LookupRpcService.
	accessRpcServiceServer := bifrost_rpc_access.NewAccessRpcServiceServer(
		b,
		true,
		func(remoteServerID string) (string, error) {
			if remoteServerID == "" {
				remoteServerID = "default"
			}
			// simplify plugin-host/web-view/ to web-view/
			if strings.HasPrefix(remoteServerID, "web-view/") {
				return remoteServerID, nil
			}
			return bldr_plugin.HostServerIDPrefix + remoteServerID, nil
		},
	)
	_ = bifrost_rpc_access.SRPCRegisterAccessRpcService(rpcMux, accessRpcServiceServer)

	// handle incoming PluginRpc calls by forwarding to the bus
	_ = bldr_plugin.SRPCRegisterPlugin(rpcMux, bldr_plugin.NewPluginServer(b))

	// construct the rpc client controller
	// listen for incoming requests
	if acceptPluginHostStreams != nil {
		go func() {
			srv := srpc.NewServer(rpcMux)
			if err := acceptPluginHostStreams(ctx, srv); err != nil {
				errCh <- err
			}
		}()
	}

	// start the plugin host storage controller and use the default storage id
	hostStorage := plugin_host_storage.NewPluginHostStorage()
	hostStorage.AddFactories(b, sr)
	hostStorageCtrl := storage_controller.BuildStorageController(
		bldr_plugin.HostStorageID,
		[]storage.Storage{hostStorage},
		controller.NewInfo(
			"plugin/host/storage",
			Version,
			"plugin host storage controller",
		),
	)
	relHostStorageCtrl, err := b.AddController(ctx, hostStorageCtrl, handleErr)
	if err != nil {
		return err
	}
	defer relHostStorageCtrl()

	// we have to use a separate goroutine because AcceptMuxedConn might not
	// notice ctx is canceled until after a connection arrives.
	select {
	case <-ctx.Done():
		rel()
		return context.Canceled
	case err := <-errCh:
		rel()
		return err
	}
}

// BuildPluginAssetsFSController builds a unixfs_access controller for the plugin assets.
func BuildPluginAssetsFSController(le *logrus.Entry, b bus.Bus, pluginHostClient srpc.Client) *unixfs_access.Controller {
	fsCursorSvcClient := unixfs_rpc.NewSRPCFSCursorServiceClientWithServiceID(pluginHostClient, bldr_plugin.PluginAssetsServiceID)
	return unixfs_access.NewController(
		le,
		b,
		controller.NewInfo(
			"plugin/entrypoint/client/fs/assets",
			Version,
			"plugin assets filesystem",
		),
		[]string{bldr_plugin.PluginAssetsFsId},
		unixfs_rpc_client.NewFSHandleBuilder(fsCursorSvcClient),
	)
}

// BuildPluginDistFSController builds a unixfs_access controller for the plugin dist fs.
func BuildPluginDistFSController(le *logrus.Entry, b bus.Bus, pluginHostClient srpc.Client) *unixfs_access.Controller {
	fsCursorSvcClient := unixfs_rpc.NewSRPCFSCursorServiceClientWithServiceID(pluginHostClient, bldr_plugin.PluginDistServiceID)
	return unixfs_access.NewController(
		le,
		b,
		controller.NewInfo(
			"plugin/entrypoint/client/fs/dist",
			Version,
			"plugin dist filesystem",
		),
		[]string{bldr_plugin.PluginDistFsId},
		unixfs_rpc_client.NewFSHandleBuilder(fsCursorSvcClient),
	)
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

// PluginDevInfoFromFile loads a PluginDevInfo object from a .bin file.
func PluginDevInfoFromFile(filePath string) (*vardef.PluginDevInfo, error) {
	dat, err := readFile(filePath)
	if err != nil {
		return nil, err
	}
	info := &vardef.PluginDevInfo{}
	if err := info.UnmarshalVT(dat); err != nil {
		return nil, err
	}
	return info, nil
}

// UnmarshalPluginStartInfo unmarshals the plugin start information.
func UnmarshalPluginStartInfo(
	pluginStartInfoB58,
	pluginMetaB58 string,
) (
	*bldr_plugin.PluginStartInfo,
	*bldr_plugin.PluginMeta,
	error,
) {
	pluginMeta, err := bldr_plugin.UnmarshalPluginMetaB58(pluginMetaB58)
	if err != nil {
		return nil, nil, err
	}

	pluginStartInfo, err := bldr_plugin.UnmarshalPluginStartInfoB58(pluginStartInfoB58)
	if err != nil {
		return nil, nil, err
	}

	return pluginStartInfo, pluginMeta, nil
}
