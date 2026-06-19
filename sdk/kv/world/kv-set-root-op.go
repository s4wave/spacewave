package s4wave_kv_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// KvSetRootOpId is the replayable operation id for kv/store root advances.
var KvSetRootOpId = "kv/store/set-root"

// NewKvSetRootOp constructs a root-advance operation.
func NewKvSetRootOp(objectKey string, rootRef *bucket.ObjectRef) *KvSetRootOp {
	return &KvSetRootOp{
		ObjectKey: objectKey,
		RootRef:   rootRef.Clone(),
	}
}

// NewKvSetRootOpBlock constructs a KvSetRootOp block.
func NewKvSetRootOpBlock() block.Block {
	return &KvSetRootOp{}
}

// GetOperationTypeId returns the operation type identifier.
func (o *KvSetRootOp) GetOperationTypeId() string {
	return KvSetRootOpId
}

// Validate performs cursory checks on the operation.
func (o *KvSetRootOp) Validate() error {
	if o.GetObjectKey() == "" {
		return world.ErrEmptyObjectKey
	}
	rootRef := o.GetRootRef()
	if rootRef == nil || rootRef.GetEmpty() {
		return errors.New("kv/store: root ref is required")
	}
	return rootRef.Validate()
}

// ApplyWorldOp applies the root update to a kv/store world object.
func (o *KvSetRootOp) ApplyWorldOp(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}
	if err := world_types.CheckObjectType(ctx, ws, o.GetObjectKey(), KvStoreTypeID); err != nil {
		return false, err
	}
	obj, err := world.MustGetObject(ctx, ws, o.GetObjectKey())
	if err != nil {
		return false, err
	}
	return o.ApplyWorldObjectOp(ctx, le, obj, sender)
}

// ApplyWorldObjectOp applies the root update to an object handle.
func (o *KvSetRootOp) ApplyWorldObjectOp(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	sender peer.ID,
) (sysErr bool, err error) {
	if err := o.Validate(); err != nil {
		return false, err
	}
	if os.GetKey() != o.GetObjectKey() {
		return false, errors.Errorf("kv/store: op target %s does not match object %s", o.GetObjectKey(), os.GetKey())
	}
	_, err = os.SetRootRef(ctx, o.GetRootRef().Clone())
	return false, err
}

// MarshalBlock marshals the block to binary.
func (o *KvSetRootOp) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block from binary.
func (o *KvSetRootOp) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// LookupKvSetRootOp looks up a KvSetRootOp operation type.
func LookupKvSetRootOp(ctx context.Context, operationTypeID string) (world.Operation, error) {
	if operationTypeID == KvSetRootOpId {
		return &KvSetRootOp{}, nil
	}
	return nil, nil
}

var _ world.Operation = ((*KvSetRootOp)(nil))
