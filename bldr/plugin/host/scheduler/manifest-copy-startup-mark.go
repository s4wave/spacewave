package plugin_host_scheduler

import bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"

func (t *pluginInstance) emitManifestCopyStartupMark(
	phase manifestCopyPhase,
	stats bucket_lookup.ObjectCopyStats,
	accounting *manifestCopyAccounting,
) bool {
	if t == nil || accounting == nil || t.manifestCopyAccounting.Load() != accounting {
		return false
	}
	emitManifestCopyStartupMarkToBrowser(phase, accounting.apply(stats))
	return true
}

func (t *pluginInstance) emitPluginManifestRoot(rootHash string) {
	emitPluginManifestRootToBrowser(t.pluginID, rootHash)
}
