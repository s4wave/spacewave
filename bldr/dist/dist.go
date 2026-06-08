package bldr_dist

import (
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	"github.com/s4wave/spacewave/net/hash"
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
			WritebackBehavior:    lookup_concurrent.WritebackBehavior_WritebackBehavior_NONE,
			PutBlockBehavior:     lookup_concurrent.PutBlockBehavior_PutBlockBehavior_ALL,
			NotFoundBehavior:     lookup_concurrent.NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE_WAIT,
		},
	), false)
	if err != nil {
		return nil, err
	}
	conf, err := bucket.NewConfig(
		GetDistBucketID(projectID),
		1, // rev
		&bucket.LookupConfig{Controller: cc},
	)
	if err != nil {
		return nil, err
	}
	conf.PutOpts = &block.PutOpts{HashType: hash.HashType_HashType_SHA256}
	return conf, nil
}
