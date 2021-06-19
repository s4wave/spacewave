package world_mock

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/world"
)

// MockObjectOpId is the mock object operation identifier.
var MockObjectOpId = "hydra/world/mock/mock-object-op"

// ApplyMockObjectOp applies a mock operation.
func ApplyMockObjectOp(
	ctx context.Context,
	objectHandle world.ObjectState,
	operationTypeID string,
	op world.Operation,
) (handled bool, err error) {
	if operationTypeID != MockObjectOpId {
		return false, nil
	}

	return false, errors.New("TODO apply mock object op")
}

// _ is a type assertion
var _ world.ApplyObjectOpFunc = ApplyMockObjectOp
