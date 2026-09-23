package plugin_host_scheduler

import bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"

// emitManifestCopyStartupMark emits the browser startup mark for one
// manifest copy phase.
func (t *pluginInstance) emitManifestCopyStartupMark(
	phase manifestCopyPhase,
	stats bucket_lookup.ObjectCopyStats,
	accounting *manifestCopyAccounting,
) bool {
	if t == nil || accounting == nil || t.manifestCopyAccounting.Load() != accounting {
		return false
	}
	emitManifestCopyStartupMarkToBrowser(t.pluginID, phase, accounting.apply(stats))
	return true
}

// emitPluginManifestRoot announces to the browser that this runtime serves
// the plugin's files at the manifest root.
func (t *pluginInstance) emitPluginManifestRoot(rootHash string) {
	emitPluginManifestRootToBrowser(t.pluginID, rootHash)
}
