package resource_objecttype_registry

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
)

// handlerResources supplies the resource operations used by one handler session.
// A plugin connection retains its load; a caller-attached client needs no load.
type handlerResources interface {
	// AccessRootResource acquires the handler root until its release.
	AccessRootResource() resource_client.ResourceRef
	// CreateResourceReference adopts the typed child returned by the handler.
	CreateResourceReference(resourceID uint32) resource_client.ResourceRef
	// AttachResource lends the session's World engine to the handler.
	AttachResource(ctx context.Context, label string, mux srpc.Invoker) (uint32, error)
	// DetachResource withdraws the session's World engine.
	DetachResource(ctx context.Context, resourceID uint32) error
	// Release ends the retained resource generation.
	Release()
}
