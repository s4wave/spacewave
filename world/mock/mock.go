package world_mock

import "github.com/aperturerobotics/hydra/world"

// LookupMockOp looks up an operation type for a op type id.
// returns nil, nil if not found.
func LookupMockOp(operationTypeID string) (world.Operation, error) {
	switch operationTypeID {
	case MockWorldOpId:
		return &MockWorldOp{}, nil
	case MockObjectOpId:
		return &MockObjectOp{}, nil
	default:
		return nil, nil
	}
}

// _ is a type assertion
var _ world.LookupOp = LookupMockOp

// GetMockWorldOpHandlers builds the set of mock object op handlers.
func GetMockWorldOpHandlers() []world.ApplyWorldOpFunc {
	return []world.ApplyWorldOpFunc{
		ApplyMockWorldOp,
	}
}

// GetMockObjectOpHandlers builds the set of mock object op handlers.
func GetMockObjectOpHandlers() []world.ApplyObjectOpFunc {
	return []world.ApplyObjectOpFunc{
		ApplyMockObjectOp,
	}
}
