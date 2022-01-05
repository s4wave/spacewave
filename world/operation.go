package world

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/sirupsen/logrus"
)

// Operation is a batch operation against World or Object.
type Operation interface {
	// Block indicates this operation is serializable to a block.
	block.Block

	// Validate performs cursory validation of the operation.
	// Should not block.
	Validate() error

	// GetOperationTypeId returns the operation type identifier.
	GetOperationTypeId() string

	// ApplyWorldOp applies the operation as a world operation.
	// returns false, ErrUnhandledOp if the operation cannot handle a world op
	ApplyWorldOp(
		ctx context.Context,
		le *logrus.Entry,
		worldHandle WorldState,
		sender peer.ID,
	) (sysErr bool, err error)

	// ApplyWorldObjectOp applies the operation to a world object handle.
	// returns false, ErrUnhandledOp if the operation cannot handle a object op
	ApplyWorldObjectOp(
		ctx context.Context,
		le *logrus.Entry,
		objectHandle ObjectState,
		sender peer.ID,
	) (sysErr bool, err error)
}

// LookupOp looks up an operation type for a op type id.
//
// returns nil, nil if not found.
type LookupOp = func(ctx context.Context, operationTypeID string) (Operation, error)

// LookupOpSlice is a set of LookupOp calls called in sequence.
type LookupOpSlice []LookupOp

// LookupOp performs the LookupOp call against a list of funcs.
func (s LookupOpSlice) LookupOp(ctx context.Context, opTypeID string) (Operation, error) {
	for _, cb := range s {
		// we shouldn't have nil funcs, but check to be sure
		if cb == nil {
			continue
		}

		op, err := cb(ctx, opTypeID)
		if err != nil {
			return nil, err
		}
		if op != nil {
			return op, nil
		}
	}
	return nil, nil
}

// NewLookupOpFromSlice builds LookupOp from a LookupOpSlice.
func NewLookupOpFromSlice(sl LookupOpSlice) LookupOp {
	return sl.LookupOp
}
