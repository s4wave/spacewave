package world_block_engine

import (
	"context"
	"errors"

	"github.com/aperturerobotics/hydra/world"
)

// callApplyWorldOp handles applying a world operation to an engine store.
// uses the ApplyWorldOp directive to perform the action.
func (e *Controller) callApplyWorldOp(
	ctx context.Context,
	worldHandle world.WorldState,
	operationTypeID string,
	op world.Operation,
) (handled bool, err error) {
	return false, errors.New("TODO callApplyWorldOp")
}

// callApplyObjectOp handles applying a object operation to an engine store.
// uses the ApplyObjectOp directive to perform the action.
func (c *Controller) callApplyObjectOp(
	ctx context.Context,
	objectHandle world.ObjectState,
	operationTypeID string,
	op world.Operation,
) (handled bool, err error) {
	return false, errors.New("TODO callApplyObjectOp")
}

// _ is a type assertion
var (
	_ world.ApplyWorldOpFunc  = ((*Controller)(nil)).callApplyWorldOp
	_ world.ApplyObjectOpFunc = ((*Controller)(nil)).callApplyObjectOp
)
