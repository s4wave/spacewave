package plugin_host_scheduler

import bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"

func (t *pluginInstance) emitManifestCopyStartupMark(
	phase manifestCopyPhase,
	stats bucket_lookup.ObjectCopyStats,
) {
	emitManifestCopyStartupMarkToBrowser(phase, stats)
}

func (t *pluginInstance) emitPluginManifestRoot(rootHash string) {
	emitPluginManifestRootToBrowser(t.pluginID, rootHash)
}
