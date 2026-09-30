package plugin_entrypoint

import (
	"context"
	"io/fs"
	"strings"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/core"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_entrypoint_controller "github.com/s4wave/spacewave/bldr/plugin/entrypoint/controller"
	plugin_host_configset "github.com/s4wave/spacewave/bldr/plugin/host/configset"
	plugin_host_storage_volume "github.com/s4wave/spacewave/bldr/plugin/host/storage/volume"
	vardef "github.com/s4wave/spacewave/bldr/plugin/vardef"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	sdk_plugin_host "github.com/s4wave/spacewave/bldr/sdk/plugin/host"
	"github.com/s4wave/spacewave/bldr/storage"
	storage_controller "github.com/s4wave/spacewave/bldr/storage/controller"
	web_fetch_service "github.com/s4wave/spacewave/bldr/web/fetch/service"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	unixfs_rpc_client "github.com/s4wave/spacewave/db/unixfs/rpc/client"
	volume_rpc_client "github.com/s4wave/spacewave/db/volume/rpc/client"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	bifrost_rpc_access "github.com/s4wave/spacewave/net/rpc/access"
	"github.com/sirupsen/logrus"
)

// AddFactoryFunc is a callback to add a factory.
type AddFactoryFunc func(b bus.Bus) []controller.Factory

// BuildConfigSetFunc is a function to build a list of ConfigSet to apply.
type BuildConfigSetFunc func(ctx context.Context, b bus.Bus, le *logrus.Entry) ([]configset.ConfigSet, error)

// AcceptPluginHostStreamsFunc serves incoming plugin host streams and reports
// when the handler is ready.
type AcceptPluginHostStreamsFunc func(ctx context.Context, srv *srpc.Server, ready func()) error

// ExecutePluginEntrypoint builds the bus & starts common controllers.
func ExecutePluginEntrypoint(
	rctx context.Context,
	le *logrus.Entry,
	meta *bldr_plugin.PluginMeta,
	addFactoryFuncs []AddFactoryFunc,
	configSetFuncs []BuildConfigSetFunc,
	pluginHostClient srpc.Client,
	acceptPluginHostStreams AcceptPluginHostStreamsFunc,
) error {
	// Build the release chain for every started controller.
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

	// Start the core controller bus.
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}

	// add built-in factories
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
		NotFoundIfIdle: true,
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

	// Record the manifest ref and the host storage id.
	pluginManifestRef := pluginInfo.GetManifestRef()
	if pluginInfo.GetHostStorageId() != "" {
		ctx = storage.WithHostStorageID(ctx, "default")
	}
	le.Infof(
		"plugin information received from host w/ manifest: %s",
		pluginManifestRef.GetManifestRef().MarshalString(),
	)

	// errCh will interrupt the program
	errCh := make(chan error, 5)
	handleErr := func(err error) {
		handlePluginEntrypointError(errCh, err)
	}

	// Controller errors interrupt the entrypoint through errCh.

	// Hosts without storage omit the volume capability.
	if hostVolumeInfo := pluginInfo.GetHostVolumeInfo(); hostVolumeInfo != nil {
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
	}

	// serve the plugin assets filesystem
	pluginAssetsFsCtrl := BuildPluginAssetsFSController(le, b, pluginHostClient, meta.GetPluginId())
	relPluginAssetsFsCtrl, err := b.AddController(ctx, pluginAssetsFsCtrl, handleErr)
	if err != nil {
		rel()
		return err
	}
	rels = append(rels, relPluginAssetsFsCtrl)

	// serve the plugin dist filesystem
	pluginDistFsCtrl := BuildPluginDistFSController(le, b, pluginHostClient, meta.GetPluginId())
	relPluginDistFsCtrl, err := b.AddController(ctx, pluginDistFsCtrl, handleErr)
	if err != nil {
		rel()
		return err
	}
	rels = append(rels, relPluginDistFsCtrl)

	// apply config sets
	mergedConfigSet := configset.MergeConfigSets(configSets...)
	configSetStarted, csetRel, err := applyStartupConfigSet(b, mergedConfigSet)
	if err != nil {
		rel()
		return err
	}
	rels = append(rels, csetRel)

	// Serve the plugin RPC mux and start initial capability registration.
	rpcMux := newPluginRpcMux(le, b)
	srv := srpc.NewServer(rpcMux)
	err = startInitialCapabilityRegistration(
		ctx,
		srv,
		acceptPluginHostStreams,
		errCh,
		func(ctx context.Context) error {
			// Startup controllers are initial capabilities: callers of a ready
			// plugin must reach the directives they resolve.
			select {
			case <-configSetStarted:
			case <-ctx.Done():
				return context.Cause(ctx)
			}
			if pluginInfo.GetStandalone() {
				return nil
			}
			return completeInitialCapabilityRegistration(ctx, b)
		},
	)
	if err != nil {
		rel()
		return err
	}

	// Start the plugin storage controller and use the default storage id.
	// On js/wasm this resolves to direct OPFS access, and on native it
	// proxies through the plugin host via RPC.
	storages := buildPluginStorages(b, sr, pluginInfo.GetHostStorageId())
	hostStorageCtrl := storage_controller.BuildStorageController(
		bldr_plugin.HostStorageID,
		storages,
		controller.NewInfo(
			"plugin/host/storage",
			Version,
			"plugin host storage controller",
		),
	)
	relHostStorageCtrl, err := b.AddController(ctx, hostStorageCtrl, handleErr)
	if err != nil {
		rel()
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

// newPluginRpcMux serves the plugin host's calls into this plugin. Unknown
// services wait on the plugin bus for a LookupRpcService resolver.
func newPluginRpcMux(le *logrus.Entry, b bus.Bus) srpc.Mux {
	// Build the mux around the bus-backed RPC invoker.
	rpcMux := srpc.NewMux(bifrost_rpc.NewInvoker(b, bldr_plugin.HostServerIDPrefix+"default", true))

	// Go plugins publish their registrations as they start, so they keep the
	// in-place replacement contract. Answer the scheduler's probe here: the bus
	// fallback would wait for an Activation service that never appears.
	_ = bldr_plugin.SRPCRegisterActivation(rpcMux, inPlaceActivation{})

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

	// Return the assembled mux.
	return rpcMux
}

// inPlaceActivation reports that a Go plugin has no staged registrations.
type inPlaceActivation struct{}

// Check reports the in-place replacement contract.
func (inPlaceActivation) Check(context.Context, *bldr_plugin.CheckActivationRequest) (*bldr_plugin.CheckActivationResponse, error) {
	return nil, srpc.ErrUnimplemented
}

// Activate is never called for a plugin that reports in-place replacement.
func (inPlaceActivation) Activate(context.Context, *bldr_plugin.ActivatePluginRequest) (*bldr_plugin.ActivatePluginResponse, error) {
	return nil, srpc.ErrUnimplemented
}

// startInitialCapabilityRegistration serves incoming plugin host streams and
// waits for the handler to become ready before completing initial capability
// registration.
func startInitialCapabilityRegistration(
	ctx context.Context,
	srv *srpc.Server,
	acceptPluginHostStreams AcceptPluginHostStreamsFunc,
	errCh chan error,
	complete func(context.Context) error,
) error {
	// The stream handler is required to serve the plugin host.
	if acceptPluginHostStreams == nil {
		return errors.New("plugin host stream handler is not configured")
	}

	// Serve streams until the handler reports readiness.
	readyCh := make(chan struct{})
	var readyOnce sync.Once
	go func() {
		if err := acceptPluginHostStreams(ctx, srv, func() {
			readyOnce.Do(func() {
				close(readyCh)
			})
		}); err != nil {
			errCh <- err
		}
	}()

	// Wait for readiness, an error, or the context ending.
	select {
	case <-readyCh:
	case err := <-errCh:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}

	// Complete the initial capability registration.
	return complete(ctx)
}

// applyStartupConfigSet applies the plugin's startup controllers. The returned
// channel closes once every controller in set is running or reported an error.
func applyStartupConfigSet(b bus.Bus, set configset.ConfigSet) (<-chan struct{}, func(), error) {
	// An empty set reports started immediately.
	started := make(chan struct{})
	if len(set) == 0 {
		close(started)
		return started, func() {}, nil
	}

	// Track the settled state of each controller key by directive value.
	// The started channel closes once every key has settled.
	var mtx sync.Mutex
	var closed bool
	settled := make(map[uint32]string)
	update := func() {
		keys := make(map[string]struct{}, len(settled))
		for _, key := range settled {
			keys[key] = struct{}{}
		}
		if !closed && len(keys) >= len(set) {
			closed = true
			close(started)
		}
	}
	_, ref, err := b.AddDirective(
		configset.NewApplyConfigSet(set),
		directive.NewCallbackHandler(
			func(v directive.AttachedValue) {
				// Ignore attached values that are not settled states.
				st, ok := v.GetValue().(configset.State)
				if !ok || (st.GetController() == nil && st.GetError() == nil) {
					return
				}

				// Record the settled controller key and check completion.
				mtx.Lock()
				settled[v.GetValueID()] = st.GetId()
				update()
				mtx.Unlock()
			},
			func(v directive.AttachedValue) {
				// Drop the value's settled key when it detaches.
				mtx.Lock()
				delete(settled, v.GetValueID())
				mtx.Unlock()
			},
			nil,
		),
	)
	if err != nil {
		return nil, nil, err
	}

	// Release the directive reference to stop tracking the config set.
	return started, ref.Release, nil
}

// completeInitialCapabilityRegistration tells the plugin host that the plugin
// finished registering its initial capabilities and can serve requests.
func completeInitialCapabilityRegistration(ctx context.Context, b bus.Bus) error {
	// Reach the plugin host root resource through the host service prefix.
	resourceService := resource.NewSRPCResourceServiceClientWithServiceID(
		bifrost_rpc.NewBusClient(b),
		bldr_plugin.HostServiceIDPrefix+resource.SRPCResourceServiceServiceID,
	)
	client, err := resource_client.NewClient(ctx, resourceService)
	if err != nil {
		return errors.Wrap(err, "connect to plugin host resource")
	}
	defer client.Release()

	// Access the plugin host root resource client.
	rootRef := client.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		return errors.Wrap(err, "access plugin host root resource")
	}

	// The host root outlives this client, so the signal stays recorded
	// after the client is released.
	service := sdk_plugin_host.NewSRPCPluginHostResourceServiceClient(rootClient)
	_, err = service.CompleteInitialCapabilityRegistration(
		ctx,
		&sdk_plugin_host.CompleteInitialCapabilityRegistrationRequest{},
	)
	return errors.Wrap(err, "complete initial capability registration")
}

// handlePluginEntrypointError forwards an entrypoint error to errCh,
// dropping normal client-close errors.
func handlePluginEntrypointError(errCh chan<- error, err error) {
	// Normal web runtime client closes are dropped.
	if web_runtime.IsNormalWebRuntimeClientClose(err) {
		return
	}

	// Forward the error without blocking when the channel is full.
	select {
	case errCh <- err:
	default:
	}
}

// isExpectedPluginEntrypointError reports whether the error is an
// expected entrypoint shutdown error.
func isExpectedPluginEntrypointError(err error) bool {
	return err == context.Canceled || web_runtime.IsNormalWebRuntimeClientClose(err)
}

// BuildPluginAssetsFSController builds a unixfs_access controller for the plugin assets.
func BuildPluginAssetsFSController(le *logrus.Entry, b bus.Bus, pluginHostClient srpc.Client, pluginID string) *unixfs_access.Controller {
	fsCursorSvcClient := unixfs_rpc.NewSRPCFSCursorServiceClientWithServiceID(pluginHostClient, bldr_plugin.PluginAssetsServiceID)
	return unixfs_access.NewController(
		le,
		b,
		controller.NewInfo(
			"plugin/entrypoint/client/fs/assets",
			Version,
			"plugin assets filesystem",
		),
		[]string{bldr_plugin.PluginAssetsFsId(""), bldr_plugin.PluginAssetsFsId(pluginID)},
		unixfs_rpc_client.NewFSHandleBuilder(fsCursorSvcClient),
	)
}

// BuildPluginDistFSController builds a unixfs_access controller for the plugin dist fs.
func BuildPluginDistFSController(le *logrus.Entry, b bus.Bus, pluginHostClient srpc.Client, pluginID string) *unixfs_access.Controller {
	fsCursorSvcClient := unixfs_rpc.NewSRPCFSCursorServiceClientWithServiceID(pluginHostClient, bldr_plugin.PluginDistServiceID)
	return unixfs_access.NewController(
		le,
		b,
		controller.NewInfo(
			"plugin/entrypoint/client/fs/dist",
			Version,
			"plugin dist filesystem",
		),
		[]string{bldr_plugin.PluginDistFsId(""), bldr_plugin.PluginDistFsId(pluginID)},
		unixfs_rpc_client.NewFSHandleBuilder(fsCursorSvcClient),
	)
}

// ConfigSetFuncFromFS builds a ConfigSetFunc which parses a file in a FS as a ConfigSet.
func ConfigSetFuncFromFS(ifs fs.FS, fileName string) BuildConfigSetFunc {
	return func(ctx context.Context, b bus.Bus, le *logrus.Entry) ([]configset.ConfigSet, error) {
		// Read and unmarshal the config set file.
		data, err := fs.ReadFile(ifs, fileName)
		if err != nil {
			return nil, err
		}
		set := &configset_proto.ConfigSet{}
		if err := set.UnmarshalVT(data); err != nil {
			return nil, err
		}

		// Resolve the config set against the bus.
		cset, err := set.Resolve(ctx, b)
		if err != nil {
			return nil, err
		}
		return []configset.ConfigSet{cset}, nil
	}
}

// PluginDevInfoFromFile loads a PluginDevInfo object from a .bin file.
func PluginDevInfoFromFile(filePath string) (*vardef.PluginDevInfo, error) {
	// Read and unmarshal the dev info file.
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

// UnmarshalPluginMeta unmarshals the plugin meta information.
func UnmarshalPluginMeta(pluginMetaB58 string) (*bldr_plugin.PluginMeta, error) {
	return bldr_plugin.UnmarshalPluginMetaB58(pluginMetaB58)
}

// UnmarshalPluginStartInfo unmarshals the plugin start information.
func UnmarshalPluginStartInfo(pluginStartInfoJsonB64 string) (*bldr_plugin.PluginStartInfo, error) {
	return bldr_plugin.UnmarshalPluginStartInfoJsonBase64(pluginStartInfoJsonB64)
}
