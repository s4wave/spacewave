//go:build !js

package bldr_core_devtool

import (
	dist_compiler "github.com/aperturerobotics/bldr/dist/compiler"
	bldr_plugin_builder_controller "github.com/aperturerobotics/bldr/manifest/builder/controller"
	plugin_compiler_go "github.com/aperturerobotics/bldr/plugin/compiler/go"
	plugin_compiler_js "github.com/aperturerobotics/bldr/plugin/compiler/js"
	bldr_project_controller "github.com/aperturerobotics/bldr/project/controller"
	bldr_project_watcher "github.com/aperturerobotics/bldr/project/watcher"
	bldr_web_bundler_esbuild_compiler "github.com/aperturerobotics/bldr/web/bundler/esbuild/compiler"
	bldr_web_bundler_vite_compiler "github.com/aperturerobotics/bldr/web/bundler/vite/compiler"
	web_pkg_compiler "github.com/aperturerobotics/bldr/web/pkg/compiler"
	web_plugin_compiler "github.com/aperturerobotics/bldr/web/plugin/compiler"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_bolt "github.com/aperturerobotics/hydra/volume/bolt"
)

// AddFactories adds the devtool factories.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	// volumes
	sr.AddFactory(volume_bolt.NewFactory(b))

	addCommonFactories(b, sr)

	// project
	sr.AddFactory(bldr_project_controller.NewFactory(b))
	sr.AddFactory(bldr_project_watcher.NewFactory(b))

	// compiler
	sr.AddFactory(bldr_plugin_builder_controller.NewFactory(b))
	sr.AddFactory(dist_compiler.NewFactory(b))

	sr.AddFactory(plugin_compiler_go.NewFactory(b))
	sr.AddFactory(plugin_compiler_js.NewFactory(b))

	sr.AddFactory(web_pkg_compiler.NewFactory(b))
	sr.AddFactory(web_plugin_compiler.NewFactory(b))

	sr.AddFactory(bldr_web_bundler_esbuild_compiler.NewFactory(b))
	sr.AddFactory(bldr_web_bundler_vite_compiler.NewFactory(b))
}
