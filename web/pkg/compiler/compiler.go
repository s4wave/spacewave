package bldr_web_pkg_compiler

import (
	"context"
	"slices"
	"strings"

	bldr_manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	manifest_builder "github.com/aperturerobotics/bldr/manifest/builder"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	plugin_compiler "github.com/aperturerobotics/bldr/plugin/compiler"
	bldr_web_plugin_handle_web_pkg "github.com/aperturerobotics/bldr/web/plugin/handle-web-pkg"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/blang/semver"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/web/pkg/compiler"

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
) (*bldr_manifest_builder.BuilderResult, error) {
	builderConf := args.GetBuilderConfig()
	meta, _, err := builderConf.GetManifestMeta().Resolve()
	if err != nil {
		return nil, err
	}
	pluginID := strings.TrimSpace(meta.GetManifestId())

	pluginCompilerConf := plugin_compiler.NewConfig()
	pluginCompilerConf.ProjectId = c.GetConfig().GetProjectId()
	pluginCompilerConf.DisableFetchAssets = true
	pluginCompilerConf.DisableRpcFetch = true
	pluginCompilerConf.DelveAddr = c.GetConfig().GetDelveAddr()
	pluginCompilerConf.ConfigSet = c.GetConfig().GetConfigSet()

	pluginCompilerConf.ConfigSet = map[string]*configset_proto.ControllerConfig{}
	configset_proto.MergeConfigSetMaps(pluginCompilerConf.ConfigSet, c.GetConfig().GetConfigSet())

	pluginCompilerConf.HostConfigSet = map[string]*configset_proto.ControllerConfig{}
	configset_proto.MergeConfigSetMaps(pluginCompilerConf.HostConfigSet, c.GetConfig().GetHostConfigSet())

	// Cleanup list of web packages
	webPkgs := slices.Clone(c.GetConfig().GetWebPkgs())
	slices.Sort(webPkgs)
	webPkgs = slices.Compact(webPkgs)
	pluginCompilerConf.WebPkgs = webPkgs

	// - handle-web-pkgs: handle web pkg lookups for the webPkgIds
	if len(webPkgs) != 0 {
		if _, err := configset_proto.ConfigSetMap(pluginCompilerConf.ConfigSet).ApplyConfig(
			"handle-web-pkgs",
			&bldr_web_plugin_handle_web_pkg.Config{
				WebPluginId:    c.GetConfig().GetWebPluginId(),
				HandlePluginId: pluginID,
				WebPkgIdList:   webPkgs,
			},
			1,
			false,
		); err != nil {
			return nil, err
		}
	}

	pluginCompilerCtrl, err := plugin_compiler.NewController(c.GetLogger(), c.GetBus(), pluginCompilerConf)
	if err != nil {
		return nil, err
	}

	// build the manifest
	return pluginCompilerCtrl.BuildManifest(ctx, args)
}

// GetElectronApplicable returns if electron should be bundled for this platform.
func GetElectronApplicable(parsedPlatform bldr_platform.Platform) bool {
	_, ok := parsedPlatform.(*bldr_platform.NativePlatform)
	return ok
}

// _ is a type assertion
var _ manifest_builder.Controller = ((*Controller)(nil))
