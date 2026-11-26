package publisher

import (
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/bucket"
)

// Released is the result of a release.
type Released struct {
	// Metadata is the staged channel metadata.
	Metadata *release.ReleaseMetadata
	// Checkpoint is the checkpoint the Space posted.
	Checkpoint *sobject.SOCheckpointInner
	// Head is the release World root the checkpoint names.
	Head *bucket.ObjectRef
	// DistConfig selects the channel for launchers.
	DistConfig *spacewave_launcher.DistConfig
	// Signed is DistConfig as the signed packed message launchers fetch.
	Signed string
}
