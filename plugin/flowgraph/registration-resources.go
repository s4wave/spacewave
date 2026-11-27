package flowgraph_plugin

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
)

// RegistrationResources retains declarations and lends their compiler resource.
type RegistrationResources interface {
	// CreateResourceReference adopts a returned registration until its release.
	CreateResourceReference(resourceID uint32) resource_client.ResourceRef
	// AttachResource lends the compiler to the registry.
	AttachResource(ctx context.Context, label string, mux srpc.Invoker) (uint32, error)
	// DetachResource withdraws the compiler after releasing its declarations.
	DetachResource(ctx context.Context, resourceID uint32) error
}
