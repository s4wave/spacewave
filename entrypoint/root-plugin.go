package entrypoint

import (
	plugin_static "github.com/aperturerobotics/bldr/plugin/static"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver"
)

// RootPluginControllerID is the controller id for the root plugin static loader.
var RootPluginControllerID = "bldr/entrypoint/root-plugin"

// RootPluginVersion is the version of the root plugin static loader controller.
var RootPluginVersion = semver.MustParse("0.0.1")

// RootPlugin is the root plugin to load on startup.
// If unset, loads nothing on startup.
// Expected to be overridden at compile-time or init-time.
var RootPlugin *plugin_static.StaticPlugin

// NewRootPluginInfo constructs the information for the root plugin.
func NewRootPluginInfo() *controller.Info {
	return controller.NewInfo(RootPluginControllerID, RootPluginVersion, "bldr entrypoint plugin")
}
