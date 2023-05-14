package forge_worker

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/world"
	world_types "github.com/aperturerobotics/hydra/world/types"
	"github.com/aperturerobotics/identity"
	identity_world "github.com/aperturerobotics/identity/world"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// WorkerCreateOpId is the worker create operation id.
var WorkerCreateOpId = WorkerTypeID + "/create"

// NewWorkerCreateOp constructs a new WorkerCreateOp block.
func NewWorkerCreateOp(objectKey, name string, keypairs []*identity.Keypair) *WorkerCreateOp {
	return &WorkerCreateOp{
		ObjectKey: objectKey,
		Name:      name,
		Keypairs:  keypairs,
	}
}

// CreateWorker stores a Worker in a object.
// Optionally creates keypairs linked to the Worker.
// Returns seqno, sysErr, error.
func CreateWorker(
	ctx context.Context,
	w world.WorldState,
	objKey string,
	name string,
	keypairs []*identity.Keypair,
	sender peer.ID,
) (uint64, bool, error) {
	op := NewWorkerCreateOp(objKey, name, keypairs)
	return w.ApplyWorldOp(ctx, op, sender)
}

// Validate performs cursory validation of the operation.
// Should not block.
func (o *WorkerCreateOp) Validate() error {
	if err := o.BuildWorker().Validate(); err != nil {
		return err
	}
	return nil
}

// GetOperationTypeId returns the operation type identifier.
func (o *WorkerCreateOp) GetOperationTypeId() string {
	return WorkerCreateOpId
}

// BuildWorker builds the worker object from the create op.
func (o *WorkerCreateOp) BuildWorker() *Worker {
	return &Worker{Name: o.GetName()}
}

// ApplyWorldOp applies the operation as a world operation.
func (o *WorkerCreateOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	worldHandle world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	objKey := o.GetObjectKey()
	wrk := o.BuildWorker()
	err = wrk.Validate()
	if err != nil {
		return false, err
	}

	_, _, err = world.CreateWorldObject(ctx, worldHandle, objKey, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(wrk, true)
		return nil
	})
	if err != nil {
		return false, err
	}

	// create the <type> ref
	err = world_types.SetObjectType(ctx, worldHandle, objKey, WorkerTypeID)
	if err != nil {
		return false, err
	}

	// create the keypair objects
	keypairs := o.GetKeypairs()
	kpKeys, err := identity_world.EnsureKeypairsExist(ctx, worldHandle, sender, keypairs, false)
	if err != nil {
		return false, err
	}

	// link to the keypair objects
	for _, kpKey := range kpKeys {
		err := worldHandle.SetGraphQuad(ctx, identity_world.NewObjectToKeypairQuad(objKey, kpKey))
		if err != nil {
			return false, errors.Wrap(err, "link worker to keypair")
		}
	}

	return false, nil
}

// ApplyWorldObjectOp applies the operation to a world object handle.
func (o *WorkerCreateOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	objectHandle world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	return false, world.ErrUnhandledOp
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *WorkerCreateOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *WorkerCreateOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ world.Operation = ((*WorkerCreateOp)(nil))
