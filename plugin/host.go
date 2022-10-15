package plugin

// HostServiceIDPrefix is the prefix used for the plugin-host RPC services. This
// ID can be prepended to RPC service IDs to indicate the service is located on
// the plugin host (while running as a plugin).
const HostServiceIDPrefix = "plugin-host/"

// HostClientID is the client ID used for plugin-host originating RPC calls.
const HostClientID = "plugin-host"

// PluginAssetsFsId is the identifier to use for the plugin assets fs.
const PluginAssetsFsId = "plugin-assets"

// HostVolumeServiceID is the service ID for the host volume proxy server.
// Prepended with the HostServiceIDPrefix if running in a plugin.
const HostVolumeServiceID = "rpc.volume.AccessVolumes"
