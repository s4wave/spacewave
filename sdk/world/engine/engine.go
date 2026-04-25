package sdk_world_engine

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// SDKEngine implements world.Engine over SRPC by delegating to
// EngineResourceService calls on a remote resource.
type SDKEngine struct {
	client  *resource_client.Client
	ref     resource_client.ResourceRef
	service s4wave_world.SRPCEngineResourceServiceClient
}

// NewSDKEngine creates a new SDKEngine wrapping a resource reference.
func NewSDKEngine(client *resource_client.Client, ref resource_client.ResourceRef) (*SDKEngine, error) {
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	return &SDKEngine{
		client:  client,
		ref:     ref,
		service: s4wave_world.NewSRPCEngineResourceServiceClient(srpcClient),
	}, nil
}

// Release releases the underlying resource reference.
func (e *SDKEngine) Release() {
	e.ref.Release()
}

// NewTransaction creates a new transaction against the world state.
// Set write=true if the transaction will perform write operations.
// Always call Discard() when done with the transaction.
// Note: Engine might return a read-only transaction even if write=true.
func (e *SDKEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	resp, err := e.service.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{Write: write})
	if err != nil {
		return nil, err
	}

	txRef := e.client.CreateResourceReference(resp.ResourceId)
	tx, err := NewSDKTx(e.client, txRef, resp.ReadOnly)
	if err != nil {
		txRef.Release()
		return nil, err
	}
	return tx, nil
}

// GetSeqno returns the current sequence number of the world state.
func (e *SDKEngine) GetSeqno(ctx context.Context) (uint64, error) {
	resp, err := e.service.GetSeqno(ctx, &s4wave_world.GetSeqnoRequest{})
	if err != nil {
		return 0, err
	}
	return resp.Seqno, nil
}

// WaitSeqno waits for the world state sequence number to reach or exceed the specified value.
func (e *SDKEngine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	resp, err := e.service.WaitSeqno(ctx, &s4wave_world.WaitSeqnoRequest{Seqno: value})
	if err != nil {
		return 0, err
	}
	return resp.Seqno, nil
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
func (e *SDKEngine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	resp, err := e.service.BuildStorageCursor(ctx, &s4wave_world.BuildStorageCursorRequest{})
	if err != nil {
		return nil, err
	}

	ref := e.client.CreateResourceReference(resp.GetResourceId())
	cursor, err := newSDKBucketLookupCursor(ctx, ref)
	if err != nil {
		ref.Release()
		return nil, err
	}
	return cursor, nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
func (e *SDKEngine) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	resp, err := e.service.AccessWorldState(ctx, &s4wave_world.AccessWorldStateRequest{Ref: ref})
	if err != nil {
		return err
	}
	return accessSDKBucketLookupCursor(ctx, e.client, resp.GetResourceId(), cb)
}

// _ is a type assertion
var _ world.Engine = (*SDKEngine)(nil)
