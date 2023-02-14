package plugin

// HostServiceIDPrefix is the prefix used for the plugin-host RPC services. This
// ID can be prepended to RPC service IDs to indicate the service is located on
// the plugin host (while running as a plugin).
const HostServiceIDPrefix = "plugin-host/"

// HostClientID is the client ID used for plugin-host originating RPC calls.
const HostClientID = "plugin-host"

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

// HostVolumeServiceIDPrefix is the service ID prefix for the host ProxyVolume.
const HostVolumeServiceIDPrefix = "host-volume"
