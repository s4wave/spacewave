package plugin_space_runtime

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// appPluginSource resolves the parent's application plugins and records local leaks.
type appPluginSource struct {
	// ids is the application's immutable plugin declaration.
	ids []string
	// plugin is the parent value returned for declared plugins.
	plugin bldr_plugin.RunningPlugin
	// loads records every parent plugin load.
	loads chan string
	// manifests records parent manifest requests, answered without an artifact.
	manifests chan string
}

// newAppPluginSource constructs a parent source with a distinct running plugin.
func newAppPluginSource(ids []string) *appPluginSource {
	return &appPluginSource{
		ids:       slices.Clone(ids),
		plugin:    bldr_plugin.NewRunningPlugin(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(srpc.NewMux())))),
		loads:     make(chan string, 16),
		manifests: make(chan string, 16),
	}
}

// GetControllerInfo identifies the parent application source.
func (s *appPluginSource) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/app-plugin-source", controller.MustParseVersion("0.0.1"), "parent application plugins")
}

// Execute leaves the source available for the controller lifetime.
func (s *appPluginSource) Execute(context.Context) error {
	return nil
}

// HandleDirective resolves declared loads and exposes unavailable manifest requests.
func (s *appPluginSource) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	switch dir := inst.GetDirective().(type) {
	case bldr_plugin.LoadPlugin:
		s.loads <- dir.LoadPluginID()
		if !slices.Contains(s.ids, dir.LoadPluginID()) {
			return nil, nil
		}
		return directive.R(directive.NewValueResolver([]bldr_plugin.RunningPlugin{s.plugin}), nil)
	case bldr_manifest.FetchManifest:
		s.manifests <- dir.GetManifestId()
		return directive.R(directive.NewValueResolver([]*bldr_manifest.FetchManifestValue{{}}), nil)
	default:
		return nil, nil
	}
}

// Close releases no additional resources.
func (s *appPluginSource) Close() error {
	return nil
}

// _ is a type assertion.
var _ controller.Controller = (*appPluginSource)(nil)
