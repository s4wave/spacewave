package bldr_dist

import (
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/hydra/bucket"
	lookup_concurrent "github.com/aperturerobotics/hydra/bucket/lookup/concurrent"
)

// StaticBlockStoreID is the BlockStoreId for the StaticBlockStore.
const StaticBlockStoreID = "entrypoint"

// DistWorldEngineID is the world engine id on the bus for the dist bundle.
const DistWorldEngineID = "dist"

// GetDistBucketID returns the bucket id for a project id dist.
func GetDistBucketID(projectID string) string {
	return "dist/" + projectID
}

// NewDistBucketConfig returns the bucket config for a project id dist.
func NewDistBucketConfig(projectID string) (*bucket.Config, error) {
	cc, err := configset_proto.NewControllerConfig(configset.NewControllerConfig(
		1, // rev
		&lookup_concurrent.Config{
			// Verbose:              true,
			FallbackBlockStoreId: StaticBlockStoreID,
			WritebackBehavior:    lookup_concurrent.WritebackBehavior_WritebackBehavior_ALL,
			PutBlockBehavior:     lookup_concurrent.PutBlockBehavior_PutBlockBehavior_ALL,
			NotFoundBehavior:     lookup_concurrent.NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE_WAIT,
		},
	), false)
	if err != nil {
		return nil, err
	}
	return bucket.NewConfig(
		GetDistBucketID(projectID),
		1, // rev
		nil,
		&bucket.LookupConfig{Controller: cc},
	)
}
