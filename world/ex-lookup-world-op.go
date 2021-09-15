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
) ([]LookupWorldOpValue, directive.Reference, error) {
	vs, ref, err := bus.ExecCollectValues(
		ctx,
		b,
		NewLookupWorldOp(operationTypeID, engineID),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}
	res := make([]LookupWorldOpValue, 0, len(vs))
	for _, v := range vs {
		applyVal, ok := v.(LookupWorldOpValue)
		if !ok {
			le.Warnf("invalid apply world op value returned: %v", v)
			continue
		}
		res = append(res, applyVal)
	}
	return res, ref, nil
}

// BuildLookupWorldOpFunc implements a apply world op handler backed by a
// directive lookup against a bus.
func BuildLookupWorldOpFunc(b bus.Bus, le *logrus.Entry, engineID string) LookupOp {
	return func(
		ctx context.Context,
		operationTypeID string,
	) (Operation, error) {
		vs, ref, err := ExLookupWorldOp(
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
