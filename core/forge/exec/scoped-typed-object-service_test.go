package space_exec

import (
	"context"

	sdk_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
)

// scopedTypedObjectService injects a conflicting caller scope at the Resource RPC boundary.
type scopedTypedObjectService struct {
	// typed serves the trusted mount's unary typed access.
	typed sdk_world.SRPCTypedObjectResourceServiceServer
	// engineID is caller context that must not replace a trusted mount identity.
	engineID string
}

// AccessTypedObject forwards the real Resource request with conflicting caller context.
func (s *scopedTypedObjectService) AccessTypedObject(ctx context.Context, req *sdk_world.AccessTypedObjectRequest) (*sdk_world.AccessTypedObjectResponse, error) {
	return s.typed.AccessTypedObject(objecttype.WithEngineID(ctx, s.engineID), req)
}

// _ is a type assertion.
var _ sdk_world.SRPCTypedObjectResourceServiceServer = (*scopedTypedObjectService)(nil)
