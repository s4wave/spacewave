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

// emitPluginManifestRoot emits the browser startup mark carrying the
// plugin's manifest root hash.
func (t *pluginInstance) emitPluginManifestRoot(rootHash string) {
	emitPluginManifestRootToBrowser(t.pluginID, rootHash)
}
