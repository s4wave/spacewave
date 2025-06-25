package plugin_host_scheduler

import (
	"context"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
)

// execDownloadManifest executes downloading the manifest fetched from FetchManifest.
func (t *pluginInstance) execDownloadManifest(ctx context.Context, manifestSnapshot *bldr_manifest.ManifestSnapshot) error {
	if t.c.conf.GetDisableCopyManifest() {
		return nil
	}

	// TODO
	// t.le.Warnf("TODO execDownloadManifest: %v", manifestValue.String())
	return nil
}
