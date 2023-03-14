package bldr_manifest_world

import (
	"context"

	"github.com/aperturerobotics/hydra/world"
)

// LookupOp performs the lookup operation for the world op types.
func LookupOp(ctx context.Context, opTypeID string) (world.Operation, error) {
	switch opTypeID {
	case ExtractManifestBundleOpId:
		return &ExtractManifestBundleOp{}, nil
	case StoreManifestOpId:
		return &StoreManifestOp{}, nil
	}
	return nil, nil
}
