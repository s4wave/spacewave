//go:build !js

package plugin_host_scheduler

import bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"

func emitManifestCopyStartupMarkToBrowser(
	_ manifestCopyPhase,
	_ bucket_lookup.ObjectCopyStats,
) {
}

func emitPluginManifestRootToBrowser(_, _ string) {
}
