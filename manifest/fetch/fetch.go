package manifest_fetch

import (
	"github.com/aperturerobotics/controllerbus/config"
)

// Config is a configuration for a ManifestFetch Controller.
type Config interface {
	// Config is the base config interface.
	config.Config

	// SetFetchManifestIdRegex sets the regex of manifest IDs to fetch with this controller.
	SetFetchManifestIdRegex(re string)
}
