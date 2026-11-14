package world

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Operation is a batch operation against World or Object.
//
// A generated message becomes an Operation by declaring its (typeid.id)
// option, which generates TypeID, and implementing ApplyWorldOp.
type Operation interface {
	// Block indicates this operation is serializable to a block.
	block.Block

	// TypeID returns the operation type identifier.
	TypeID() string

	// ApplyWorldOp applies the operation as a world operation. It returns
	// false, ErrUnhandledOp if the operation cannot handle a world op.
	ApplyWorldOp(
		ctx context.Context,
		le *logrus.Entry,
		worldHandle WorldState,
		sender peer.ID,
	) (sysErr bool, err error)
}

// ValidatedOperation is an Operation with cursory validation.
type ValidatedOperation interface {
	Operation

	// Validate performs cursory validation of the operation.
	// Should not block.
	Validate() error
}

// ObjectOperation is an Operation that can also apply to one object handle.
type ObjectOperation interface {
	Operation

	// ApplyWorldObjectOp applies the operation to a world object handle. It
	// returns false, ErrUnhandledOp if the operation cannot handle an object op.
	ApplyWorldObjectOp(
		ctx context.Context,
		le *logrus.Entry,
		objectHandle ObjectState,
		sender peer.ID,
	) (sysErr bool, err error)
}

// ValidateOp validates op when it implements ValidatedOperation.
func ValidateOp(op Operation) error {
	if v, ok := op.(ValidatedOperation); ok {
		return v.Validate()
	}
	return nil
}

// ApplyWorldObjectOp applies op to an object handle. It returns false,
// ErrUnhandledOp when op is not an ObjectOperation.
func ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	op Operation,
	objectHandle ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	objOp, ok := op.(ObjectOperation)
	if !ok {
		return false, ErrUnhandledOp
	}
	return objOp.ApplyWorldObjectOp(ctx, le, objectHandle, sender)
}

// PayloadOperation is an Operation whose payload was written outside the World
// tree before the operation was applied. The World after the operation need not
// reference the payload, but replay reads it again through its roots, so the
// replay keeps the payload owned while the operation is above the checkpoint.
type PayloadOperation interface {
	Operation

	// GetPayloadRefs returns the roots of the operation's payload.
	GetPayloadRefs() []*block.BlockRef
	// PayloadRefsVersion identifies what GetPayloadRefs names for the
	// operation type. Saved payload roots are reused only under the same
	// version; change what GetPayloadRefs names and the golden test makes you
	// bump it. It is never zero.
	PayloadRefsVersion() uint32
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
