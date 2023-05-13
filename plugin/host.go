package bldr_plugin

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

// PluginAssetsFsId is the identifier to use for the plugin assets fs.
const PluginAssetsFsId = "plugin-assets"

// PluginHttpPrefix is the route prefix for plugin-controlled URL space.
// /p/{pluginId}/
const PluginHttpPrefix = "/p/"

// PluginAssetsHttpPrefix is the route prefix to use for plugin assets.
// This is within the plugin HTTP url space:
// /p/{plugin-id}/a/{assets-fs-path}
const PluginAssetsHttpPrefix = "/a/"

// PluginVolumeID is an alias to the host volume (while running as a plugin).
const PluginVolumeID = "plugin-host"
