//go:build js

package plugin_host_scheduler

import (
	"syscall/js"

	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

func emitManifestCopyStartupMarkToBrowser(
	phase manifestCopyPhase,
	stats bucket_lookup.ObjectCopyStats,
) {
	mark := js.Global().Get("BLDR_NOTIFY_STARTUP_MARK")
	if mark.IsUndefined() || mark.IsNull() || mark.Type() != js.TypeFunction {
		return
	}
	mark.Invoke(
		"manifest-copy."+string(phase),
		string(phase),
		float64(stats.BlocksSeen),
		float64(stats.BlocksCopied),
		float64(stats.BlocksExisting),
		float64(stats.BlocksWritten),
		float64(stats.BlocksDeduped),
		float64(stats.SubtreesSkipped),
		float64(stats.LogicalSourceBytes),
	)
}
