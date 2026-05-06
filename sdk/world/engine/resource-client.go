package sdk_world_engine

import resource_client "github.com/s4wave/spacewave/bldr/resource/client"

// ResourceClient creates references for resource IDs returned by world RPCs.
type ResourceClient interface {
	CreateResourceReference(resourceID uint32) resource_client.ResourceRef
}
