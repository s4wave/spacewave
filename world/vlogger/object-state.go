package world_vlogger

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/world"
	"github.com/sirupsen/logrus"
)

// ObjectState implements ObjectState wrapped with verbose logging.
type ObjectState struct {
	// ObjectState is the underlying ObjectState object.
	world.ObjectState

	// le is the base logger
	ble *logrus.Entry
}

// NewObjectState constructs a new object state vlogger.
func NewObjectState(le *logrus.Entry, objectState world.ObjectState) *ObjectState {
	return &ObjectState{
		ObjectState: objectState,
		ble:         le,
	}
}

// le returns a logger with object fields
func (o *ObjectState) le() *logrus.Entry {
	return o.ble.WithField("object-key", o.GetKey())
}

// GetRootRef returns the root reference.
// Returns the revision number.
func (o *ObjectState) GetRootRef() (ref *bucket.ObjectRef, rev uint64, err error) {
	defer func() {
		o.le().Debugf(
			"GetRootRef() => ref(%v) rev(%v) err(%v)",
			ref.MarshalString(),
			rev,
			err,
		)
	}()

	return o.ObjectState.GetRootRef()
}

// SetRootRef changes the root reference of the object.
// Increments the revision of the object if changed.
// Returns revision just after the change was applied.
func (o *ObjectState) SetRootRef(nref *bucket.ObjectRef) (rev uint64, err error) {
	defer func() {
		o.le().Debugf(
			"SetRootRef(%s) => rev(%v) err(%v)",
			nref.MarshalString(),
			rev,
			err,
		)
	}()

	return o.ObjectState.SetRootRef(nref)
}

// ApplyObjectOp applies a batch operation at the object level.
// The handling of the operation is operation-type specific.
// Returns the revision following the operation execution.
// If nil is returned for the error, implies success.
// If sysErr is set, the error is treated as a transient system error.
// Returns rev, sysErr, err
func (o *ObjectState) ApplyObjectOp(op world.Operation, opSender peer.ID) (rev uint64, sysErr bool, err error) {
	if op == nil {
		return 0, false, world.ErrEmptyOp
	}

	le := o.le()
	defer func() {
		le.Debugf(
			"ApplyObjectOp(%s, %s) => rev(%v) sysErr(%v) err(%v)",
			op.GetOperationTypeId(),
			opSender.Pretty(),
			rev, sysErr, err,
		)
	}()
	return o.ObjectState.ApplyObjectOp(NewOperation(le, op), opSender)
}

// IncrementRev increments the revision of the object.
// Returns revision just after the change was applied.
func (o *ObjectState) IncrementRev() (rev uint64, err error) {
	defer func() {
		o.le().Debugf(
			"IncrementRev() => rev(%v) err(%v)",
			rev,
			err,
		)
	}()
	return o.ObjectState.IncrementRev()
}

// WaitRev waits until the object rev is >= the specified.
// Returns ErrObjectNotFound if the object is deleted.
// If ignoreNotFound is set, waits for the object to exist.
// Returns the new rev.
func (o *ObjectState) WaitRev(ctx context.Context, rev uint64, ignoreNotFound bool) (orev uint64, oerr error) {
	defer func() {
		if oerr != context.Canceled {
			o.le().Debugf(
				"WaitRev(%v, %v) => rev(%v) err(%v)",
				rev,
				ignoreNotFound,
				orev,
				oerr,
			)
		}
	}()

	return o.ObjectState.WaitRev(ctx, rev, ignoreNotFound)
}

// _ is a type assertion
var _ world.ObjectState = ((*ObjectState)(nil))
