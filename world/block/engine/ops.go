package world_block_engine

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/world"
)

// callApplyWorldOp handles applying a world operation to an engine store.
// uses the ApplyWorldOp directive to perform the action.
func (c *Controller) callApplyWorldOp(
	ctx context.Context,
	worldHandle world.WorldState,
	operationTypeID string,
	op world.Operation,
	opSender peer.ID,
) (handled bool, err error) {
	if c.conf.GetDisableLookup() || c.conf.GetDisableApplyWorldOp() {
		c.le.
			WithField("operation-type-id", operationTypeID).
			Warn("apply world op was called but disable_lookup or disable_apply_world_op is set")
		return false, nil
	}

	le := c.le.WithField("operation-type-id", operationTypeID)
	if opSender != "" {
		le = le.WithField("operation-sender", opSender.Pretty())
	}
	applyOpFn := world.BuildApplyWorldOpFunc(c.bus, le, c.engineID)
	return applyOpFn(ctx, worldHandle, operationTypeID, op, opSender)
}

// callApplyObjectOp handles applying a object operation to an engine store.
// uses the ApplyObjectOp directive to perform the action.
func (c *Controller) callApplyObjectOp(
	ctx context.Context,
	objectHandle world.ObjectState,
	operationTypeID string,
	op world.Operation,
	opSender peer.ID,
) (handled bool, err error) {
	if c.conf.GetDisableLookup() || c.conf.GetDisableApplyWorldOp() {
		c.le.
			WithField("object-key", objectHandle.GetKey()).
			WithField("operation-type-id", operationTypeID).
			Warn("apply object op was called but disable_lookup or disable_apply_object_op is set")
		return false, nil
	}

	le := c.le.WithField("operation-type-id", operationTypeID)
	applyOpFn := world.BuildApplyObjectOpFunc(c.bus, le, c.engineID)
	return applyOpFn(ctx, objectHandle, operationTypeID, op, opSender)
}

// _ is a type assertion
var (
	_ world.ApplyWorldOpFunc  = ((*Controller)(nil)).callApplyWorldOp
	_ world.ApplyObjectOpFunc = ((*Controller)(nil)).callApplyObjectOp
)
