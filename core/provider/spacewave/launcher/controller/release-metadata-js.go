//go:build js

package spacewave_launcher_controller

import (
	"context"

	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

// refreshReleaseMetadataStatus skips native release staging in browser builds.
func (c *Controller) refreshReleaseMetadataStatus(ctx context.Context, distConf *spacewave_launcher.DistConfig) {
}
