package world

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
)

// Operation is a batch operation against World or Object.
type Operation interface {
	// Block indicates this operation is serializable to a block.
	block.Block
}

// LookupOp looks up an operation type for a op type id.
//
// returns nil, nil if not found.
type LookupOp = func(operationTypeID string) (Operation, error)

// ApplyWorldOpFunc executes a custom world operation type.
// Returns false, nil if unable to handle this operation type.
type ApplyWorldOpFunc = func(
	ctx context.Context,
	worldHandle WorldState,
	operationTypeID string,
	op Operation,
	opSender peer.ID,
) (handled bool, err error)

// ApplyObjectOpFunc executes a custom object operation type.
// Returns false, nil if unable to handle this operation type.
// objectCursor is located at objectHandle's current rootRef value.
type ApplyObjectOpFunc = func(
	ctx context.Context,
	objectHandle ObjectState,
	operationTypeID string,
	op Operation,
	opSender peer.ID,
) (handled bool, err error)

// CallWorldOpFuncs calls a sequence of ApplyWorldOpFunc.
// Returns ErrUnhandledOp if none of the handlers return true, nil.
// Returns an error if any of the handlers returned an error.
func CallWorldOpFuncs(
	ctx context.Context,
	t WorldState,
	operationTypeID string,
	op Operation,
	opSender peer.ID,
	worldOpHandlers ...ApplyWorldOpFunc,
) error {
	var handled bool
	for _, handlerFn := range worldOpHandlers {
		h, err := handlerFn(
			ctx,
			t,
			operationTypeID,
			op,
			opSender,
		)
		if err != nil {
			return err
		}
		if h {
			handled = true
		}
	}
	if !handled {
		return ErrUnhandledOp
	}
	return nil
}

// CallObjectOpFuncs calls a sequence of ApplyObjectOpFunc.
// Returns ErrUnhandledOp if none of the handlers return true, nil.
// Returns an error if any of the handlers returned an error.
func CallObjectOpFuncs(
	ctx context.Context,
	t ObjectState,
	operationTypeID string,
	op Operation,
	opSender peer.ID,
	objectOpHandlers ...ApplyObjectOpFunc,
) error {
	var handled bool
	for _, handlerFn := range objectOpHandlers {
		h, err := handlerFn(
			ctx,
			t,
			operationTypeID,
			op,
			opSender,
		)
		if err != nil {
			return err
		}
		if h {
			handled = true
		}
	}
	if !handled {
		return ErrUnhandledOp
	}
	return nil
}
