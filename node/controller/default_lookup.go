package node_controller

import (
	lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/bucket/lookup/concurrent"
)

// BuildDefaultLookupConfig builds a new default lookup config.
func BuildDefaultLookupConfig() lookup.Config {
	return &lookup_concurrent.Config{}
}
