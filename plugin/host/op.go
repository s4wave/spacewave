package plugin_host

import (
	"context"

	"github.com/aperturerobotics/hydra/world"
)

// LookupOp performs the lookup operation for the world op types.
func LookupOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	switch opTypeID {
	case UpdatePluginManifestOpId:
		return &UpdatePluginManifestOp{}, nil
	}
	return nil, nil
}
