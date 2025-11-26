package plugin_assets_http

import (
	"path"
	"strings"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	unixfs_access_http "github.com/aperturerobotics/hydra/unixfs/access/http"
	"github.com/blang/semver/v4"
)

// ControllerID is the controller ID for the plugin assets HTTP fetcher.
const ControllerID = "plugin/assets/http"

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

// Controller responds to LookupHTTPHandler with the plugin Assets FS.
type Controller = unixfs_access_http.Controller

// NewController constructs a new controller resolving LookupHTTPHandler.
func NewController(b bus.Bus, cc *Config) *Controller {
	info := controller.NewInfo(
		ControllerID,
		Version,
		"plugin assets http handler",
	)
	unixfsPathPrefix := strings.TrimPrefix(path.Clean(cc.GetFsPath()), ".")
	servePath := strings.TrimPrefix(path.Clean(cc.GetServePath()), ".")
	var matchPathPrefixes []string
	if servePath != "" {
		matchPathPrefixes = []string{servePath}
	}
	return unixfs_access_http.NewController(
		b,
		info,
		matchPathPrefixes,
		true,
		nil,
		bldr_plugin.PluginAssetsFsId(cc.GetPluginId()),
		unixfsPathPrefix,
		"",
		false,
	)
}
