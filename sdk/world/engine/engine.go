package sdk_world_engine

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_bucket_lookup "github.com/s4wave/spacewave/sdk/bucket/lookup"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// SDKEngine implements world.Engine over SRPC by delegating to
// EngineResourceService calls on a remote resource.
type SDKEngine struct {
	client  ResourceClient
	ref     resource_client.ResourceRef
	service s4wave_world.SRPCEngineResourceServiceClient
}

// OperationAuthor returns no local signer for a remote SDK World handle.
func (e *SDKEngine) OperationAuthor(context.Context) (peer.ID, string, error) {
	return "", "", nil
}

// NewSDKEngine creates a new SDKEngine wrapping a resource reference.
func NewSDKEngine(client ResourceClient, ref resource_client.ResourceRef) (*SDKEngine, error) {
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

// GetEngineInfo returns the engine's identity and the session it serves.
func (e *SDKEngine) GetEngineInfo(ctx context.Context) (*s4wave_world.GetEngineInfoResponse, error) {
	return e.service.GetEngineInfo(ctx, &s4wave_world.GetEngineInfoRequest{})
}

// GetResourceRef returns the engine's resource reference, which Release frees.
func (e *SDKEngine) GetResourceRef() resource_client.ResourceRef {
	return e.ref
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
	// Open a transaction against the remote World.
	resp, err := e.service.NewTransaction(ctx, &s4wave_world.NewTransactionRequest{Write: write})
	if err != nil {
		return nil, err
	}

	// Wrap the transaction resource and release it if construction fails.
	txRef := e.client.CreateResourceReference(resp.ResourceId)
	tx, err := NewSDKTx(e.client, txRef, resp.ReadOnly)
	if err != nil {
		txRef.Release()
		return nil, err
	}
	if !resp.ReadOnly {
		tx.hold = newWriteHold()
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

// Sync fences durable storage and advances the durable head via the engine.
func (e *SDKEngine) Sync(ctx context.Context) (bool, error) {
	resp, err := e.service.Sync(ctx, &s4wave_world.SyncRequest{})
	if err != nil {
		return false, err
	}
	return resp.GetFenced(), nil
}

// SyncThrough fences durable storage through seqno via the remote engine.
func (e *SDKEngine) SyncThrough(ctx context.Context, seqno uint64) (uint64, error) {
	resp, err := e.service.SyncThrough(ctx, &s4wave_world.SyncThroughRequest{Seqno: seqno})
	if err != nil {
		return 0, err
	}
	return resp.GetDurableSeqno(), nil
}

// WaitSeqno waits for the world state sequence number to reach or exceed the specified value.
func (e *SDKEngine) WaitSeqno(ctx context.Context, value uint64) (uint64, error) {
	resp, err := e.service.WaitSeqno(ctx, &s4wave_world.WaitSeqnoRequest{Seqno: value})
	if err != nil {
		return 0, err
	}
	return resp.Seqno, nil
}

// WaitObjectRev waits until the object at key reaches rev through the remote
// engine's key watch, which wakes only on commits that change the object.
func (e *SDKEngine) WaitObjectRev(ctx context.Context, key string, rev uint64, ignoreNotFound bool) (uint64, error) {
	// Wait on the remote key watch.
	resp, err := e.service.WaitObjectRev(ctx, &s4wave_world.WaitObjectRevRequest{
		Key:            key,
		Rev:            rev,
		IgnoreNotFound: ignoreNotFound,
	})
	if err != nil {
		return 0, err
	}

	// Restore ErrObjectNotFound from the response code.
	if err := s4wave_world.ErrorFromCode(resp.GetErrorCode()); err != nil {
		return 0, err
	}
	return resp.GetRev(), nil
}

// WatchChanges follows the remote engine's change stream, calling cb with each
// later seqno touches accepts and the changes since the previous one. The
// remote engine streams every change, so the filter runs here.
func (e *SDKEngine) WatchChanges(ctx context.Context, afterSeqno uint64, touches world.ChangeFilter, cb func(seqno uint64, changes *world.ChangeSet) error) error {
	// Open the stream after afterSeqno.
	strm, err := e.service.WatchChanges(ctx, &s4wave_world.WatchChangesRequest{AfterSeqno: afterSeqno})
	if err != nil {
		return err
	}
	defer strm.Close()

	// Hand each change to cb until the stream or cb ends.
	for {
		msg, err := strm.Recv()
		if err != nil {
			return err
		}
		changes := msg.ToChangeSet()
		if !touches.Accepts(changes) {
			continue
		}
		if err := cb(msg.GetSeqno(), changes); err != nil {
			return err
		}
	}
}

// GetWorldRootSnapshot returns the current committed World root snapshot.
func (e *SDKEngine) GetWorldRootSnapshot(ctx context.Context) (*s4wave_world.WorldRootSnapshot, error) {
	return e.service.GetWorldRootSnapshot(ctx, &s4wave_world.GetWorldRootSnapshotRequest{})
}

// SetRetainedRoot retains the accepted World root ref under name in the
// World's shared state. An empty ref releases the name.
func (e *SDKEngine) SetRetainedRoot(ctx context.Context, name string, ref *block.BlockRef) error {
	_, err := e.service.SetRetainedRoot(ctx, &s4wave_world.SetRetainedRootRequest{Name: name, RootRef: ref})
	return err
}

// WatchWorldRootSnapshots streams committed World root snapshots.
func (e *SDKEngine) WatchWorldRootSnapshots(ctx context.Context) (s4wave_world.SRPCEngineResourceService_WatchWorldRootSnapshotsClient, error) {
	return e.service.WatchWorldRootSnapshots(ctx, &s4wave_world.WatchWorldRootSnapshotsRequest{})
}

// BuildStorageCursor builds a cursor to the world storage with an empty ref.
func (e *SDKEngine) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	// Request a storage cursor from the remote World engine.
	resp, err := e.service.BuildStorageCursor(ctx, &s4wave_world.BuildStorageCursorRequest{})
	if err != nil {
		return nil, err
	}

	// Wrap the cursor resource and release it if construction fails.
	ref := e.client.CreateResourceReference(resp.GetResourceId())
	cursor, err := s4wave_bucket_lookup.NewCursor(ctx, ref)
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
	return s4wave_bucket_lookup.AccessCursor(ctx, e.client, resp.GetResourceId(), cb)
}

// _ are type assertions.
var (
	_ world.Engine = (*SDKEngine)(nil)
)
