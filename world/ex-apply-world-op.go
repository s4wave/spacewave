package world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/sirupsen/logrus"
)

// ExApplyWorldOp executes a lookup against a bus for a handler function to
// apply the world object op.
func ExApplyWorldOp(
	ctx context.Context,
	b bus.Bus,
	le *logrus.Entry,
	operationTypeID string,
	engineID string,
) ([]ApplyWorldOpValue, directive.Reference, error) {
	vs, ref, err := bus.ExecCollectValues(
		ctx,
		b,
		NewApplyWorldOp(operationTypeID, engineID),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	res := make([]ApplyWorldOpValue, 0, len(vs))
	for _, v := range vs {
		applyVal, ok := v.(ApplyWorldOpValue)
		if !ok {
			le.Warnf("invalid apply world op value returned: %v", v)
			continue
		}
		res = append(res, applyVal)
	}
	return res, ref, nil
}

// BuildApplyWorldOpFunc implements a apply world op handler backed by a
// directive lookup against a bus.
func BuildApplyWorldOpFunc(b bus.Bus, le *logrus.Entry, engineID string) ApplyWorldOpFunc {
	return func(
		ctx context.Context,
		worldHandle WorldState,
		operationTypeID string,
		op Operation,
	) (handled bool, err error) {
		vs, ref, err := ExApplyWorldOp(
			ctx,
			b,
			le,
			operationTypeID,
			engineID,
		)
		if err != nil {
			return false, err
		}
		defer ref.Release()

		for _, handler := range vs {
			h, err := handler(ctx, worldHandle, operationTypeID, op)
			if err != nil {
				return false, err
			}
			if h {
				handled = true
			}
		}
		return handled, nil
	}
}
