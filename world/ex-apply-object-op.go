package world

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/sirupsen/logrus"
)

// ExApplyObjectOp executes a lookup against a bus for a handler function to
// apply the world object op.
func ExApplyObjectOp(
	ctx context.Context,
	b bus.Bus,
	le *logrus.Entry,
	operationTypeID string,
	objectID, engineID string,
) ([]ApplyObjectOpValue, directive.Reference, error) {
	vs, ref, err := bus.ExecCollectValues(
		ctx,
		b,
		NewApplyObjectOp(operationTypeID, objectID, engineID),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	res := make([]ApplyObjectOpValue, 0, len(vs))
	for _, v := range vs {
		applyVal, ok := v.(ApplyObjectOpValue)
		if !ok {
			le.Warnf("invalid apply object op value returned: %v", v)
			continue
		}
		res = append(res, applyVal)
	}
	return res, ref, nil
}

// BuildApplyObjectOpFunc implements a apply object op handler backed by a
// directive lookup against a bus.
func BuildApplyObjectOpFunc(b bus.Bus, le *logrus.Entry, engineID string) ApplyObjectOpFunc {
	return func(
		ctx context.Context,
		objectHandle ObjectState,
		operationTypeID string,
		op Operation,
		opSender peer.ID,
	) (handled bool, err error) {
		var objectID string
		if objectHandle != nil {
			objectID = objectHandle.GetKey()
		}
		vs, ref, err := ExApplyObjectOp(
			ctx,
			b,
			le,
			operationTypeID,
			objectID,
			engineID,
		)
		if err != nil {
			return false, err
		}
		defer ref.Release()

		for _, handler := range vs {
			h, err := handler(ctx, objectHandle, operationTypeID, op, opSender)
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
