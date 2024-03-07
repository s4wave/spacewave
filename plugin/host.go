package bldr_plugin

import (
	"context"
	"strings"
)

// HostServiceIDPrefix is the prefix used for the plugin-host RPC services. This
// ID can be prepended to RPC service IDs to indicate the service is located on
// the plugin host (while running as a plugin).
const HostServiceIDPrefix = "plugin-host/"

// HostServerIDPrefix is the server ID prefix used for plugin-host originating RPC calls.
const HostServerIDPrefix = "plugin-host/"

// HostServerID constructs a server id for a component on the plugin host.
// remoteServerID can be empty
func HostServerID(remoteServerID string) string {
	var id string
	if remoteServerID != "" {
		id = HostServerIDPrefix + remoteServerID
	} else {
		id = HostServerIDPrefix[:len(HostServerIDPrefix)-1]
	}
	return id
}

// PluginServerIDPrefix is the server id prefix for plugins.
// Incoming RPC calls from other plugins will have ServerID=PluginServerIDPrefix+RemotePluginID
const PluginServerIDPrefix = "plugin/"

// PluginServerID constructs a server id for a component on a plugin.
// remoteServerID can be empty
func PluginServerID(pluginID, remoteServerID string) string {
	id := PluginServerIDPrefix + pluginID
	if remoteServerID != "" {
		id += "/" + remoteServerID
	}
	return id
}

// PluginServiceIDPrefix is the prefix used for calling services on other plugins.
//
// ID can be prepended to RPC service IDs to indicate the service is located on
// another plugin.
//
// For example: LookupRpcService<plugin/foo/my.Service> will access the
// my.Service service on the plugin with ID "foo".
const PluginServiceIDPrefix = "plugin/"

// HostVolumeServiceIDPrefix is the service ID prefix for the host ProxyVolume.
const HostVolumeServiceIDPrefix = "host-volume/"

// PluginAssetsFsId is the identifier to use for the plugin assets fs on the plugin bus.
const PluginAssetsFsId = "plugin-assets"

// PluginDistFsId is the identifier to use for the plugin dist fs on the plugin bus.
const PluginDistFsId = "plugin-dist"

// BldrHttpPrefix is the route prefix for bldr-controlled URL space.
// /b/
const BldrHttpPrefix = "/b/"

// PluginDistHttpPrefix is the route prefix to use for plugin dist.
// This is only available when the web plugin host is running.
// /b/pd/
const PluginDistHttpPrefix = BldrHttpPrefix + "pd/"

// PluginWebPkgHttpPrefix is the public URL path prefix for web packages.
// /b/pkg/
const PluginWebPkgHttpPrefix = BldrHttpPrefix + "pkg/"

// PluginHttpPrefix is the route prefix for plugin-controlled URL space.
// /p/{pluginId}/
const PluginHttpPrefix = "/p/"

// PluginAssetsHttpPrefix is the route prefix to use for plugin assets.
// This is within the plugin HTTP url space:
// /p/{plugin-id}/a/{assets-fs-path}
const PluginAssetsHttpPrefix = "/a/"

// PluginAssetsWebPkgsDir is the directory within assets fs for web pkgs.
const PluginAssetsWebPkgsDir = "bldr-web-pkgs"

// PluginVolumeID is an alias to the host volume (while running as a plugin).
const PluginVolumeID = "plugin-host"

// PluginHTTPPath adds the plugin http prefix to the given path.
func PluginHTTPPath(pluginID, httpPath string) string {
	var sb strings.Builder
	_, _ = sb.WriteString(PluginHttpPrefix)
	_, _ = sb.WriteString(pluginID)
	if !strings.HasPrefix(httpPath, "/") {
		_, _ = sb.WriteString("/")
	}
	_, _ = sb.WriteString(httpPath)
	return sb.String()
}

// PluginHTTPPathFromContext detects the current plugin from the context and
// conditionally adds the plugin http path prefix to the given path.
func PluginHTTPPathFromContext(ctx context.Context, httpPath string) string {
	info := GetPluginContextInfo(ctx)
	pluginID := info.GetPluginMeta().GetPluginId()
	if pluginID != "" {
		return PluginHTTPPath(pluginID, httpPath)
	}
	return httpPath
}

// ParseHTTPPathPluginID parses and validates a {plugin-id}/ prefix from a HTTP path.
func ParseHTTPPathPluginID(httpPath string) (pluginID string, suffix string, err error) {
	httpPath = strings.TrimPrefix(httpPath, "/")
	slashIdx := strings.IndexRune(httpPath, '/')

	pluginID = httpPath
	if slashIdx != -1 {
		pluginID = httpPath[:slashIdx]
		suffix = httpPath[slashIdx:]
	}

	return pluginID, suffix, ValidatePluginID(pluginID, false)
}

// PluginDistHTTPPath adds the plugin distribution file prefix to the given path.
func PluginDistHTTPPath(pluginID, httpPath string) string {
	var sb strings.Builder
	_, _ = sb.WriteString(PluginDistHttpPrefix)
	_, _ = sb.WriteString(pluginID)
	if !strings.HasPrefix(httpPath, "/") {
		_, _ = sb.WriteString("/")
	}
	_, _ = sb.WriteString(httpPath)
	return sb.String()
}
