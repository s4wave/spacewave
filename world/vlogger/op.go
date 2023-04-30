package world_vlogger

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/world"
	"github.com/sirupsen/logrus"
)

// Operation wraps a world operation with a verbose logger.
type Operation struct {
	// Operation is the underlying world op
	world.Operation
	// ble is the base logger
	ble *logrus.Entry
}

// NewOperation constructs a new verbose operation.
func NewOperation(le *logrus.Entry, op world.Operation) *Operation {
	return &Operation{ble: le, Operation: op}
}

// le returns a logger with operation fields
func (o *Operation) le() *logrus.Entry {
	return o.ble.WithField("op-type", o.GetOperationTypeId())
}

// ApplyWorldOp applies the operation as a world operation.
// returns false, ErrUnhandledOp if the operation cannot handle a world op
func (o *Operation) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	defer func() {
		o.le().Debugf(
			"ApplyWorldOp(%s) => sysErr(%v) err(%v)",
			sender.Pretty(),
			sysErr,
			err,
		)
	}()

	// NOTE: ApplyWorldOp is called by the world state without a verbose logger.
	if _, isVerbose := worldHandle.(*WorldState); !isVerbose {
		worldHandle = NewWorldState(o.le(), worldHandle)
	}

	return o.Operation.ApplyWorldOp(ctx, le, worldHandle, sender)
}

// ApplyWorldObjectOp applies the operation to a world object handle.
// returns false, ErrUnhandledOp if the operation cannot handle a object op
func (o *Operation) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	defer func() {
		o.le().Debugf(
			"ApplyWorldObjectOp(%s) => sysErr(%v) err(%v)",
			sender.Pretty(),
			sysErr,
			err,
		)
	}()

	// NOTE: ApplyWorldObjectOp is called by the world state without a verbose logger.
	if _, isVerbose := objectHandle.(*ObjectState); !isVerbose {
		objectHandle = NewObjectState(o.le(), objectHandle)
	}

	return o.Operation.ApplyWorldObjectOp(ctx, le, objectHandle, sender)
}

// _ is a type assertion
var _ world.Operation = ((*Operation)(nil))
