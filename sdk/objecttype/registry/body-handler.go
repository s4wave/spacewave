package s4wave_objecttype_registry

import (
	"context"

	"github.com/s4wave/spacewave/db/blocktype"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// BodyHandler serves ObjectTypeBodyService from a plugin's block types.
type BodyHandler struct {
	// lookup resolves an ObjectType ID to the block type of its root body.
	lookup func(ctx context.Context, typeID string) (blocktype.BlockType, error)
}

// NewBodyHandler builds a BodyHandler over lookup, such as a plugin World
// schema's LookupBlockType.
func NewBodyHandler(lookup func(ctx context.Context, typeID string) (blocktype.BlockType, error)) *BodyHandler {
	return &BodyHandler{lookup: lookup}
}

// MarshalBodiesJSON renders root block bodies of one ObjectType as JSON. A
// type that lookup does not know leaves every body without JSON.
func (h *BodyHandler) MarshalBodiesJSON(ctx context.Context, req *MarshalBodiesJSONRequest) (*MarshalBodiesJSONResponse, error) {
	bt, err := h.lookup(ctx, req.GetTypeId())
	if err != nil {
		return nil, err
	}
	if bt == nil {
		return &MarshalBodiesJSONResponse{BodiesJson: make([]string, len(req.GetBodies()))}, nil
	}
	return &MarshalBodiesJSONResponse{BodiesJson: blocktype.RenderJSON(bt, req.GetBodies())}, nil
}

// DeclareBodies reads what root block bodies of one ObjectType declare
// through the World trait interfaces. A type that lookup does not know leaves
// every declaration empty.
func (h *BodyHandler) DeclareBodies(ctx context.Context, req *DeclareBodiesRequest) (*DeclareBodiesResponse, error) {
	bt, err := h.lookup(ctx, req.GetTypeId())
	if err != nil {
		return nil, err
	}
	if bt == nil {
		return &DeclareBodiesResponse{Declarations: EmptyDeclarations(len(req.GetBodies()))}, nil
	}
	return &DeclareBodiesResponse{Declarations: s4wave_world.DeclareBodies(bt, req.GetBodies())}, nil
}

// EmptyDeclarations returns n empty declarations, for bodies nobody decodes.
func EmptyDeclarations(n int) []*s4wave_world.ObjectDeclaration {
	out := make([]*s4wave_world.ObjectDeclaration, n)
	for i := range out {
		out[i] = &s4wave_world.ObjectDeclaration{}
	}
	return out
}

// _ is a type assertion
var _ SRPCObjectTypeBodyServiceServer = (*BodyHandler)(nil)
