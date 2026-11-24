package resource_root

// PluginSecretGrant decides which plugins may use a Space's Secrets on this
// Device. The Device policy implements it.
type PluginSecretGrant interface {
	// PermitsSecretPlugin reports whether pluginID may read, create and write
	// the Secrets of the Space with shared object ID spaceID.
	PermitsSecretPlugin(spaceID, pluginID string) bool
}
