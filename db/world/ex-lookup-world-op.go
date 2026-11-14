package world

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
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
		false,
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

// BuildHeldLookupWorldOpFunc returns a lookup like BuildLookupWorldOpFunc that
// looks each operation type up on b once, a type nothing resolves included,
// and holds each lookup's directive until release, so the handlers it found
// stay valid and a walk over many operations makes one bus lookup per type.
// A handler that registers after its type's lookup is not seen. Lookups run
// one at a time. After release the lookup fails.
func BuildHeldLookupWorldOpFunc(b bus.Bus, le *logrus.Entry, engineID string) (LookupOp, func()) {
	// Keep each type's handlers and the directives that hold them.
	var mtx sync.Mutex
	types := make(map[string]LookupOp)
	var refs []directive.Reference
	lookup := func(ctx context.Context, operationTypeID string) (Operation, error) {
		// Look the type up once, holding its directive.
		mtx.Lock()
		resolve, ok := types[operationTypeID]
		if !ok {
			if types == nil {
				mtx.Unlock()
				return nil, errors.New("operation lookup released")
			}
			vs, _, ref, err := ExLookupWorldOp(ctx, b, le, operationTypeID, engineID)
			if err != nil {
				mtx.Unlock()
				return nil, err
			}
			resolve = NewLookupOpFromSlice(vs)
			types[operationTypeID] = resolve
			refs = append(refs, ref)
		}
		mtx.Unlock()
		return resolve(ctx, operationTypeID)
	}
	release := func() {
		mtx.Lock()
		defer mtx.Unlock()
		for _, ref := range refs {
			ref.Release()
		}
		types, refs = nil, nil
	}
	return lookup, release
}
