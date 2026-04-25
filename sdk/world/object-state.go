package s4wave_world

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// ObjectState wraps ObjectStateResourceService and implements world.ObjectState.
// Represents a handle to an object in the store.
//
// In the Go implementation (hydra/world/object-state.go), ObjectState provides:
// - GetKey() string
// - GetRootRef(ctx) (*bucket.ObjectRef, uint64, error)
// - SetRootRef(ctx, *bucket.ObjectRef) (uint64, error)
// - AccessWorldState(ctx, *bucket.ObjectRef, callback) error
// - ApplyObjectOp(ctx, Operation, peer.ID) (rev, sysErr, error)
// - IncrementRev(ctx) (uint64, error)
// - WaitRev(ctx, uint64, ignoreNotFound bool) (uint64, error)
//
// This RPC-based implementation wraps ObjectStateResourceService.
type ObjectState struct {
	client    *resource_client.Client
	ref       resource_client.ResourceRef
	service   SRPCObjectStateResourceServiceClient
	objectKey string
}

// NewObjectState creates a new ObjectState resource wrapper.
func NewObjectState(client *resource_client.Client, ref resource_client.ResourceRef, objectKey string) (world.ObjectState, error) {
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	return &ObjectState{
		client:    client,
		ref:       ref,
		service:   NewSRPCObjectStateResourceServiceClient(srpcClient),
		objectKey: objectKey,
	}, nil
}

// GetResourceRef returns the resource reference.
func (os *ObjectState) GetResourceRef() resource_client.ResourceRef {
	return os.ref
}

// Release releases the resource reference.
func (os *ObjectState) Release() {
	os.ref.Release()
}

// GetKey returns the key this state object is for.
// Returns stored metadata without RPC call.
func (os *ObjectState) GetKey() string {
	return os.objectKey
}

// GetRootRef returns the root reference.
// Returns the revision number.
func (os *ObjectState) GetRootRef(ctx context.Context) (*bucket.ObjectRef, uint64, error) {
	resp, err := os.service.GetRootRef(ctx, &GetRootRefRequest{})
	if err != nil {
		return nil, 0, err
	}
	return resp.RootRef, resp.Rev, nil
}

// SetRootRef changes the root reference of the object.
// Increments the revision of the object if changed.
// Returns revision just after the change was applied.
func (os *ObjectState) SetRootRef(ctx context.Context, rootRef *bucket.ObjectRef) (uint64, error) {
	resp, err := os.service.SetRootRef(ctx, &SetRootRefRequest{RootRef: rootRef})
	if err != nil {
		return 0, err
	}
	return resp.Rev, nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, will default to the object RootRef.
// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
// The lookup cursor will be released after cb returns.
func (os *ObjectState) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	resp, err := os.service.AccessWorldState(ctx, &AccessWorldStateRequest{Ref: ref})
	if err != nil {
		return err
	}

	cursorRef := os.client.CreateResourceReference(resp.ResourceId)
	defer cursorRef.Release()

	// TODO: Need to create a proper bucket lookup cursor wrapper
	// For now, return an error indicating this is not yet implemented
	return nil
}

// ApplyObjectOp applies a batch operation at the object level.
// The handling of the operation is operation-type specific.
// Returns the revision following the operation execution.
// If nil is returned for the error, implies success.
// If sysErr is set, the error is treated as a transient system error.
func (os *ObjectState) ApplyObjectOp(ctx context.Context, op world.Operation, sender peer.ID) (uint64, bool, error) {
	opData, err := op.MarshalBlock()
	if err != nil {
		return 0, false, err
	}

	resp, err := os.service.ApplyObjectOp(ctx, &ApplyObjectOpRequest{
		OpTypeId: op.GetOperationTypeId(),
		OpData:   opData,
		OpSender: sender.String(),
	})
	if err != nil {
		return 0, false, err
	}

	return resp.Rev, resp.SysErr, nil
}

// IncrementRev increments the revision of the object.
// Returns revision just after the change was applied.
func (os *ObjectState) IncrementRev(ctx context.Context) (uint64, error) {
	resp, err := os.service.IncrementRev(ctx, &IncrementRevRequest{})
	if err != nil {
		return 0, err
	}
	return resp.Rev, nil
}

// WaitRev waits until the object rev is >= the specified.
// Returns ErrObjectNotFound if the object is deleted.
// If ignoreNotFound is set, waits for the object to exist.
// Returns the new rev.
func (os *ObjectState) WaitRev(ctx context.Context, rev uint64, ignoreNotFound bool) (uint64, error) {
	resp, err := os.service.WaitRev(ctx, &WaitRevRequest{
		Rev:            rev,
		IgnoreNotFound: ignoreNotFound,
	})
	if err != nil {
		return 0, err
	}
	return resp.Rev, nil
}

// _ is a type assertion
var _ world.ObjectState = (*ObjectState)(nil)
