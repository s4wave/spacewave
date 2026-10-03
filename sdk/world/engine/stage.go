package sdk_world_engine

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	s4wave_bucket_lookup "github.com/s4wave/spacewave/sdk/bucket/lookup"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// StageWorldState opens a remote World stage. Releasing it releases the
// stage resource.
func (e *SDKEngine) StageWorldState(ctx context.Context) (world.WorldStage, error) {
	// Open the stage on the remote World engine.
	resp, err := e.service.StageWorldState(ctx, &s4wave_world.StageWorldStateRequest{})
	if err != nil {
		return nil, err
	}

	// Wrap the stage resource and release it if construction fails.
	ref := e.client.CreateResourceReference(resp.GetResourceId())
	srpcClient, err := ref.GetClient()
	if err != nil {
		ref.Release()
		return nil, err
	}
	return &sdkStage{
		client:  e.client,
		ref:     ref,
		service: s4wave_world.NewSRPCWorldStageResourceServiceClient(srpcClient),
	}, nil
}

// sdkStage implements world.WorldStage over a remote stage resource.
type sdkStage struct {
	client  ResourceClient
	ref     resource_client.ResourceRef
	service s4wave_world.SRPCWorldStageResourceServiceClient
}

// BuildStorageCursor builds a staged cursor to the world storage.
func (s *sdkStage) BuildStorageCursor(ctx context.Context) (*bucket_lookup.Cursor, error) {
	// Request a staged storage cursor.
	resp, err := s.service.BuildStorageCursor(ctx, &s4wave_world.BuildStorageCursorRequest{})
	if err != nil {
		return nil, err
	}

	// Wrap the cursor resource and release it if construction fails.
	ref := s.client.CreateResourceReference(resp.GetResourceId())
	cursor, err := s4wave_bucket_lookup.NewCursor(ctx, ref)
	if err != nil {
		ref.Release()
		return nil, err
	}
	return cursor, nil
}

// AccessWorldState builds a staged cursor with an optional ref.
func (s *sdkStage) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	resp, err := s.service.AccessWorldState(ctx, &s4wave_world.AccessWorldStateRequest{Ref: ref})
	if err != nil {
		return err
	}
	return s4wave_bucket_lookup.AccessCursor(ctx, s.client, resp.GetResourceId(), cb)
}

// Release releases the stage resource.
func (s *sdkStage) Release() {
	s.ref.Release()
}

// _ is a type assertion
var _ world.WorldStage = (*sdkStage)(nil)
