package s4wave_world

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/bucket"
)

// Engine is the top-level resource for Hydra's World data structure.
// Engine implements a transactional world state container.
//
// In the Go implementation (hydra/world/engine.go), Engine provides:
// - NewTransaction(ctx, write bool) (Tx, error)
// - WorldStorage for bucket cursor access (BuildStorageCursor, AccessWorldState)
// - WorldWaitSeqno for sequence number waiting (GetSeqno, WaitSeqno)
//
// This Go SDK implementation wraps EngineResourceService and WatchWorldStateResourceService.
type Engine struct {
	client       *resource_client.Client
	ref          resource_client.ResourceRef
	service      SRPCEngineResourceServiceClient
	watchService SRPCWatchWorldStateResourceServiceClient
}

// NewEngine creates a new Engine resource wrapper.
func NewEngine(client *resource_client.Client, ref resource_client.ResourceRef) (*Engine, error) {
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	return &Engine{
		client:       client,
		ref:          ref,
		service:      NewSRPCEngineResourceServiceClient(srpcClient),
		watchService: NewSRPCWatchWorldStateResourceServiceClient(srpcClient),
	}, nil
}

// GetResourceRef returns the resource reference.
func (e *Engine) GetResourceRef() resource_client.ResourceRef {
	return e.ref
}

// Release releases the resource reference.
func (e *Engine) Release() {
	e.ref.Release()
}

// GetEngineInfo returns information about the world engine.
func (e *Engine) GetEngineInfo(ctx context.Context) (*GetEngineInfoResponse, error) {
	return e.service.GetEngineInfo(ctx, &GetEngineInfoRequest{})
}

// NewTransaction creates a new transaction against the world state.
// Set write=true if the transaction will perform write operations.
// Always call Release() when done with the transaction.
// Note: Engine might return a read-only transaction even if write=true.
func (e *Engine) NewTransaction(ctx context.Context, write bool) (*Tx, error) {
	resp, err := e.service.NewTransaction(ctx, &NewTransactionRequest{Write: write})
	if err != nil {
		return nil, err
	}

	// Create resource reference and wrapper for the transaction
	txRef := e.client.CreateResourceReference(resp.ResourceId)
	return NewTx(e.client, txRef, resp.ReadOnly)
}

// GetSeqno returns the current sequence number of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (e *Engine) GetSeqno(ctx context.Context) (uint64, error) {
	resp, err := e.service.GetSeqno(ctx, &GetSeqnoRequest{})
	if err != nil {
		return 0, err
	}
	return resp.Seqno, nil
}

// WaitSeqno waits for the world state sequence number to reach or exceed the specified value.
// Returns the seqno when the condition is reached.
// If seqno == 0, this might return immediately unconditionally.
func (e *Engine) WaitSeqno(ctx context.Context, seqno uint64) (uint64, error) {
	resp, err := e.service.WaitSeqno(ctx, &WaitSeqnoRequest{Seqno: seqno})
	if err != nil {
		return 0, err
	}
	return resp.Seqno, nil
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
// The cursor should be released independently of the Engine.
// Be sure to call Release on the cursor resource when done.
// Returns the resource ID of the cursor.
func (e *Engine) BuildStorageCursor(ctx context.Context) (uint32, error) {
	resp, err := e.service.BuildStorageCursor(ctx, &BuildStorageCursorRequest{})
	if err != nil {
		return 0, err
	}
	return resp.ResourceId, nil
}

// AccessWorldState builds a bucket lookup cursor with an optional ref.
// If the ref is empty, returns a cursor pointing to the root world state.
// The lookup cursor must be released when done.
// Returns the resource ID of the cursor.
func (e *Engine) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef) (uint32, error) {
	resp, err := e.service.AccessWorldState(ctx, &AccessWorldStateRequest{Ref: ref})
	if err != nil {
		return 0, err
	}
	return resp.ResourceId, nil
}

// WatchWorldState creates a streaming watch of the WorldState.
// Returns a stream that sends resource IDs whenever tracked resources change.
// The watch tracks changes across the entire engine, not just a single transaction.
func (e *Engine) WatchWorldState(ctx context.Context) (SRPCWatchWorldStateResourceService_WatchWorldStateClient, error) {
	return e.watchService.WatchWorldState(ctx, &WatchWorldStateRequest{})
}
