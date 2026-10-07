package sdk_world_engine

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_bucket_lookup "github.com/s4wave/spacewave/sdk/bucket/lookup"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// SDKObjectState implements world.ObjectState over SRPC by delegating to
// ObjectStateResourceService calls on a remote resource.
type SDKObjectState struct {
	// client creates references to resources returned by the service.
	client ResourceClient
	// ref retains the remote object state.
	ref resource_client.ResourceRef
	// service accesses the remote object state.
	service s4wave_world.SRPCObjectStateResourceServiceClient
	// objectKey identifies the object addressed by this handle.
	objectKey string
	// readOnly is the containing World state's transaction mode.
	readOnly bool
}

// NewSDKObjectState wraps an object resource with its containing World's read-only mode.
func NewSDKObjectState(client ResourceClient, ref resource_client.ResourceRef, objectKey string, readOnly bool) (*SDKObjectState, error) {
	// Acquire the remote object state client.
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}

	// Retain the resource, object key, and transaction mode.
	return &SDKObjectState{
		client:    client,
		ref:       ref,
		service:   s4wave_world.NewSRPCObjectStateResourceServiceClient(srpcClient),
		objectKey: objectKey,
		readOnly:  readOnly,
	}, nil
}

// GetReadOnly returns whether the containing World state is read-only.
func (os *SDKObjectState) GetReadOnly() bool {
	return os.readOnly
}

// Release releases the underlying resource reference.
func (os *SDKObjectState) Release() {
	os.ref.Release()
}

// GetKey returns the key this state object is for.
func (os *SDKObjectState) GetKey() string {
	return os.objectKey
}

// GetRootRef returns the root reference and current revision number.
func (os *SDKObjectState) GetRootRef(ctx context.Context) (*bucket.ObjectRef, uint64, error) {
	resp, err := os.service.GetRootRef(ctx, &s4wave_world.GetRootRefRequest{})
	if err != nil {
		return nil, 0, err
	}
	return resp.RootRef, resp.Rev, nil
}

// SetRootRef changes the root reference of the object.
// Increments the revision of the object if changed.
// Returns revision just after the change was applied.
func (os *SDKObjectState) SetRootRef(ctx context.Context, rootRef *bucket.ObjectRef) (uint64, error) {
	// Preserve the read-only error identity before contacting the server.
	if os.GetReadOnly() {
		return 0, tx.ErrNotWrite
	}

	// Change the remote object root and return its revision.
	resp, err := os.service.SetRootRef(ctx, &s4wave_world.SetRootRefRequest{RootRef: rootRef})
	if err != nil {
		return 0, err
	}
	return resp.Rev, nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
func (os *SDKObjectState) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	resp, err := os.service.AccessWorldState(ctx, &s4wave_world.AccessWorldStateRequest{Ref: ref})
	if err != nil {
		return err
	}
	return s4wave_bucket_lookup.AccessCursor(ctx, os.client, resp.GetResourceId(), cb)
}

// ApplyObjectOp applies a batch operation at the object level.
// Returns rev, sysErr, err.
func (os *SDKObjectState) ApplyObjectOp(ctx context.Context, op world.Operation, sender peer.ID) (uint64, bool, error) {
	// Preserve the read-only error identity before encoding or sending the operation.
	if os.GetReadOnly() {
		return 0, false, tx.ErrNotWrite
	}

	// Encode the object operation for the resource service.
	opData, err := op.MarshalBlock()
	if err != nil {
		return 0, false, err
	}

	// Apply the encoded operation to the remote object and return its outcome.
	resp, err := os.service.ApplyObjectOp(ctx, &s4wave_world.ApplyObjectOpRequest{
		OpTypeId: op.GetOperationTypeId(),
		OpData:   opData,
		OpSender: sender.String(),
	})
	if err != nil {
		return 0, false, err
	}
	if err := resp.GetError(); err != nil {
		return 0, false, err
	}
	return resp.Rev, resp.SysErr, nil
}

// IncrementRev increments the revision of the object.
// Returns revision just after the change was applied.
func (os *SDKObjectState) IncrementRev(ctx context.Context) (uint64, error) {
	// Preserve the read-only error identity before contacting the server.
	if os.GetReadOnly() {
		return 0, tx.ErrNotWrite
	}

	// Increment the remote object revision.
	resp, err := os.service.IncrementRev(ctx, &s4wave_world.IncrementRevRequest{})
	if err != nil {
		return 0, err
	}
	return resp.Rev, nil
}

// WaitRev waits until the object rev is >= the specified.
// Returns ErrObjectNotFound if the object is deleted.
// If ignoreNotFound is set, waits for the object to exist.
// Returns the new rev.
func (os *SDKObjectState) WaitRev(ctx context.Context, rev uint64, ignoreNotFound bool) (uint64, error) {
	resp, err := os.service.WaitRev(ctx, &s4wave_world.WaitRevRequest{
		Rev:            rev,
		IgnoreNotFound: ignoreNotFound,
	})
	if err != nil {
		return 0, err
	}
	return resp.Rev, nil
}

// _ is a type assertion.
var _ world.ObjectState = (*SDKObjectState)(nil)
