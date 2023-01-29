package world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/sirupsen/logrus"
)

// ExLookupWorldOp executes a lookup against a bus for a operation handler.
func ExLookupWorldOp(
	ctx context.Context,
	b bus.Bus,
	le *logrus.Entry,
	operationTypeID string,
	engineID string,
) ([]LookupWorldOpValue, directive.Instance, directive.Reference, error) {
	return bus.ExecCollectValues[LookupWorldOpValue](
		ctx,
		b,
		NewLookupWorldOp(operationTypeID, engineID),
		nil,
	)
}

// BuildLookupWorldOpFunc implements a apply world op handler backed by a
// directive lookup against a bus.
func BuildLookupWorldOpFunc(b bus.Bus, le *logrus.Entry, engineID string) LookupOp {
	return func(
		ctx context.Context,
		operationTypeID string,
	) (Operation, error) {
		vs, _, ref, err := ExLookupWorldOp(
			ctx,
			b,
			le,
			operationTypeID,
			engineID,
		)
		if err != nil {
			return nil, err
		}
		defer ref.Release()

		return NewLookupOpFromSlice(vs)(ctx, operationTypeID)
	}
}
