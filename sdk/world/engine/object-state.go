package sdk_world_engine

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// SDKObjectState implements world.ObjectState over SRPC by delegating to
// ObjectStateResourceService calls on a remote resource.
type SDKObjectState struct {
	client    *resource_client.Client
	ref       resource_client.ResourceRef
	service   s4wave_world.SRPCObjectStateResourceServiceClient
	objectKey string
}

// NewSDKObjectState creates a new SDKObjectState wrapping a resource reference.
func NewSDKObjectState(client *resource_client.Client, ref resource_client.ResourceRef, objectKey string) (*SDKObjectState, error) {
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	return &SDKObjectState{
		client:    client,
		ref:       ref,
		service:   s4wave_world.NewSRPCObjectStateResourceServiceClient(srpcClient),
		objectKey: objectKey,
	}, nil
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
	return accessSDKBucketLookupCursor(ctx, os.client, resp.GetResourceId(), cb)
}

// ApplyObjectOp applies a batch operation at the object level.
// Returns rev, sysErr, err.
func (os *SDKObjectState) ApplyObjectOp(ctx context.Context, op world.Operation, sender peer.ID) (uint64, bool, error) {
	opData, err := op.MarshalBlock()
	if err != nil {
		return 0, false, err
	}

	resp, err := os.service.ApplyObjectOp(ctx, &s4wave_world.ApplyObjectOpRequest{
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
func (os *SDKObjectState) IncrementRev(ctx context.Context) (uint64, error) {
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

// _ is a type assertion
var _ world.ObjectState = (*SDKObjectState)(nil)
