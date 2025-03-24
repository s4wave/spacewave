//go:build !js

package bldr_web_pkg_compiler

import (
	"context"
	"strings"

	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	plugin_compiler_go "github.com/aperturerobotics/bldr/plugin/compiler/go"
	bldr_web_bundler "github.com/aperturerobotics/bldr/web/bundler"
	bldr_web_plugin_handle_web_pkg "github.com/aperturerobotics/bldr/web/plugin/handle-web-pkg"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/blang/semver/v4"
)

// ControllerID is the controller ID.
const ControllerID = ConfigID

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "web pkg plugin compiler controller"

// Controller is the compiler controller.
type Controller struct {
	*bus.BusController[*Config]
}

// Factory is the factory for the compiler controller.
type Factory = bus.BusFactory[*Config, *Controller]

// NewFactory constructs a new plugin compiler controller factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		controllerDescrip,
		NewConfig,
		func(base *bus.BusController[*Config]) (*Controller, error) {
			return &Controller{
				BusController: base,
			}, nil
		},
	)
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// BuildManifest attempts to compile the manifest once.
func (c *Controller) BuildManifest(
	ctx context.Context,
	args *bldr_manifest_builder.BuildManifestArgs,
	host bldr_manifest_builder.BuildManifestHost,
) (*bldr_manifest_builder.BuilderResult, error) {
	conf := c.GetConfig()
	builderConf := args.GetBuilderConfig()
	meta, _, err := builderConf.GetManifestMeta().Resolve()
	if err != nil {
		return nil, err
	}
	pluginID := strings.TrimSpace(meta.GetManifestId())

	pluginCompilerConf := plugin_compiler_go.NewConfig()
	pluginCompilerConf.ProjectId = conf.GetProjectId()
	pluginCompilerConf.DisableFetchAssets = true
	pluginCompilerConf.DisableRpcFetch = true
	pluginCompilerConf.DelveAddr = conf.GetDelveAddr()
	pluginCompilerConf.ConfigSet = conf.GetConfigSet()

	pluginCompilerConf.ConfigSet = map[string]*configset_proto.ControllerConfig{}
	configset_proto.MergeConfigSetMaps(pluginCompilerConf.ConfigSet, conf.GetConfigSet())

	pluginCompilerConf.HostConfigSet = map[string]*configset_proto.ControllerConfig{}
	configset_proto.MergeConfigSetMaps(pluginCompilerConf.HostConfigSet, conf.GetHostConfigSet())

	// Cleanup list of web packages
	webPkgs := protobuf_go_lite.CloneVTSlice(conf.GetWebPkgs())
	pluginCompilerConf.WebPkgs = webPkgs

	// - handle-web-pkgs: handle web pkg lookups for the webPkgIds
	if len(webPkgs) != 0 {
		if _, err := configset_proto.ConfigSetMap(pluginCompilerConf.ConfigSet).ApplyConfig(
			"handle-web-pkgs",
			&bldr_web_plugin_handle_web_pkg.Config{
				WebPluginId:    conf.GetWebPluginId(),
				HandlePluginId: pluginID,
				WebPkgIdList:   bldr_web_bundler.WebPkgRefConfigSlice(webPkgs).ToIdList(),
			},
			1,
			false,
		); err != nil {
			return nil, err
		}
	}

	pluginCompilerCtrl, err := plugin_compiler_go.NewController(c.GetLogger(), c.GetBus(), pluginCompilerConf)
	if err != nil {
		return nil, err
	}

	// build the manifest
	return pluginCompilerCtrl.BuildManifest(ctx, args, host)
}

// GetElectronApplicable returns if electron should be bundled for this platform.
func GetElectronApplicable(parsedPlatform bldr_platform.Platform) bool {
	_, ok := parsedPlatform.(*bldr_platform.NativePlatform)
	return ok
}

// _ is a type assertion
var _ manifest_builder.Controller = ((*Controller)(nil))
