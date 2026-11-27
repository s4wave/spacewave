package bldr_plugin

// PinnedManifestMissingError reports that the manifest root a plugin is
// pinned to holds no build for any platform this host runs. The scheduler
// raises it only after reading a manifest at that root whole and finding its
// platform foreign, so it is final on this host until the host's platforms
// change. A root the scheduler could not read is not missing: that load waits.
type PinnedManifestMissingError struct {
	// PluginID is the plugin the load selected.
	PluginID string
	// ManifestRoot is the pinned manifest root.
	ManifestRoot string
}

// Error returns the explanation.
func (e *PinnedManifestMissingError) Error() string {
	return "plugin " + e.PluginID + ": manifest " + e.ManifestRoot + " has no build for this host's platforms"
}

// _ is a type assertion
var _ error = (*PinnedManifestMissingError)(nil)
