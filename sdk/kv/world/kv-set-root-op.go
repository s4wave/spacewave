package s4wave_kv_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// KvSetRootOpId is the replayable operation id for kv/store root advances.
var KvSetRootOpId = "kv/store/set-root"

const kvSetRootMaxCASAttempts = 16

// NewKvSetRootOp constructs a root-advance operation.
func NewKvSetRootOp(
	objectKey string,
	baseRef *bucket.ObjectRef,
	rootRef *bucket.ObjectRef,
	mutations []*KvMutation,
) *KvSetRootOp {
	return &KvSetRootOp{
		ObjectKey: objectKey,
		BaseRef:   baseRef.Clone(),
		RootRef:   rootRef.Clone(),
		Mutations: cloneKvMutations(mutations),
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
	if err := rootRef.Validate(); err != nil {
		return err
	}
	// An empty base ref is the object's initial root: a first commit from the
	// empty root carries no base. Validate the base only when it is populated.
	if baseRef := o.GetBaseRef(); baseRef != nil && !baseRef.GetEmpty() {
		if err := baseRef.Validate(); err != nil {
			return err
		}
	}
	for _, mutation := range o.GetMutations() {
		if mutation == nil {
			return errors.New("kv/store: mutation is required")
		}
		if len(mutation.GetKey()) == 0 {
			return errors.New("kv/store: mutation key is required")
		}
		switch mutation.GetKind() {
		case KvMutationKind_KV_MUTATION_KIND_SET:
		case KvMutationKind_KV_MUTATION_KIND_DELETE:
		default:
			return errors.New("kv/store: invalid mutation kind")
		}
	}
	return nil
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
	// Divergent commits replay this transaction's writes on the current root;
	// conflicts resolve in world-op order per key, never by whole-root replace.
	expected := o.GetBaseRef().Clone()
	nextRoot := o.GetRootRef().Clone()
	for range kvSetRootMaxCASAttempts {
		current, _, err := os.GetRootRef(ctx)
		if err != nil {
			return false, err
		}
		if current.EqualsRef(expected) {
			_, err = os.SetRootRef(ctx, nextRoot.Clone())
			return false, err
		}
		nextRoot, err = o.rebaseRoot(ctx, le, os, current)
		if err != nil {
			return false, err
		}
		expected = current.Clone()
	}
	return false, errors.New("kv/store: root advance CAS attempts exhausted")
}

func (o *KvSetRootOp) rebaseRoot(
	ctx context.Context,
	le *logrus.Entry,
	os world.ObjectState,
	currentRoot *bucket.ObjectRef,
) (*bucket.ObjectRef, error) {
	var nextRoot *bucket.ObjectRef
	err := os.AccessWorldState(ctx, currentRoot, func(access *world.WorldAccess) error {
		rootCursor := access.Cursor().Clone()
		defer rootCursor.Release()
		store, err := kvtx_block.NewStore(ctx, le, rootCursor, func(root *bucket.ObjectRef) error {
			nextRoot = root.Clone()
			return nil
		})
		if err != nil {
			return err
		}

		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		defer tx.Discard()
		for _, mutation := range o.GetMutations() {
			switch mutation.GetKind() {
			case KvMutationKind_KV_MUTATION_KIND_SET:
				if err := tx.Set(ctx, mutation.GetKey(), mutation.GetValue()); err != nil {
					return err
				}
			case KvMutationKind_KV_MUTATION_KIND_DELETE:
				if err := tx.Delete(ctx, mutation.GetKey()); err != nil {
					return err
				}
			default:
				return errors.New("kv/store: invalid mutation kind")
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		if nextRoot == nil {
			nextRoot = store.GetRootRef()
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if nextRoot == nil || nextRoot.GetEmpty() {
		return nil, errors.New("kv/store: rebased root was not captured")
	}
	return nextRoot, nil
}

func cloneKvMutations(mutations []*KvMutation) []*KvMutation {
	if len(mutations) == 0 {
		return nil
	}
	out := make([]*KvMutation, len(mutations))
	for i, mutation := range mutations {
		out[i] = mutation.CloneVT()
	}
	return out
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
