package resource_world_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	bucket_mock "github.com/s4wave/spacewave/db/bucket/mock"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	s4wave_bucket_lookup "github.com/s4wave/spacewave/sdk/bucket/lookup"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

type ownedCursorTestEngine struct {
	world.Engine
	storage world.WorldStorage
}

func (e *ownedCursorTestEngine) BuildOwnedLookupCursor(ctx context.Context, ref *bucket.ObjectRef) (*world.OwnedLookupCursor, error) {
	return e.storage.BuildOwnedLookupCursor(ctx, ref)
}

type failingOwnedCursorResourceContext struct {
	*worldStateOperationResourceContext
	err error
}

func (c *failingOwnedCursorResourceContext) AddResource(srpc.Invoker, func()) (uint32, error) {
	return 0, c.err
}

func newOwnedCursorResourceTestStorage(releases *atomic.Int32) world.WorldStorage {
	return world.NewCursorWorldStorage(func(context.Context) (*bucket_lookup.Cursor, error) {
		bkt := bucket_mock.NewMockBucket("resource-owner", nil)
		return bucket_lookup.NewCursorWithRelease(
			context.Background(),
			nil,
			nil,
			nil,
			bkt,
			nil,
			&bucket.ObjectRef{BucketId: "resource-owner"},
			&bucket.BucketOpArgs{BucketId: "resource-owner"},
			nil,
			func() { releases.Add(1) },
		), nil
	})
}

func TestEngineResourceOwnedCursorRegistrationTransfer(t *testing.T) {
	ctx := context.Background()
	var releases atomic.Int32
	storage := newOwnedCursorResourceTestStorage(&releases)
	engine := &ownedCursorTestEngine{storage: storage}
	resource := resource_world.NewEngineResource(nil, nil, engine, nil, nil)
	resourceCtx := &worldStateOperationResourceContext{ctx: ctx}

	resp, err := resource.AccessWorldState(
		resource_server.WithResourceClientContext(ctx, resourceCtx),
		&s4wave_world.AccessWorldStateRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if releases.Load() != 0 {
		t.Fatalf("release count after registration = %d, want 0", releases.Load())
	}

	world.RetireWorldStorage(storage)
	client, err := resourceCtx.GetAttachedResource(resp.GetResourceId())
	if err != nil {
		t.Fatal(err)
	}
	service := s4wave_bucket_lookup.NewSRPCBucketLookupCursorResourceServiceClient(client)
	refResp, err := service.GetRef(ctx, &s4wave_bucket_lookup.GetRefRequest{})
	if err != nil {
		t.Fatalf("post-retirement cursor RPC: %v", err)
	}
	if refResp.GetRef().GetBucketId() != "resource-owner" {
		t.Fatalf("resource bucket = %q, want resource-owner", refResp.GetRef().GetBucketId())
	}
	if !resourceCtx.ReleaseResource(resp.GetResourceId()) {
		t.Fatal("resource cleanup was not registered")
	}
	if releases.Load() != 1 {
		t.Fatalf("release count after cleanup = %d, want 1", releases.Load())
	}
}

func TestEngineResourceOwnedCursorSurvivesEngineClose(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	base, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Release()
	engine, err := world_block.NewEngine(ctx, le, base, nil, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	engineClosed := false
	defer func() {
		if !engineClosed {
			_ = engine.Close()
		}
	}()

	resource := resource_world.NewEngineResource(nil, nil, engine, nil, nil)
	resourceCtx := &worldStateOperationResourceContext{ctx: ctx}
	resp, err := resource.AccessWorldState(
		resource_server.WithResourceClientContext(ctx, resourceCtx),
		&s4wave_world.AccessWorldStateRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Close(); err != nil {
		t.Fatal(err)
	}
	engineClosed = true

	client, err := resourceCtx.GetAttachedResource(resp.GetResourceId())
	if err != nil {
		t.Fatal(err)
	}
	service := s4wave_bucket_lookup.NewSRPCBucketLookupCursorResourceServiceClient(client)
	refResp, err := service.GetRef(ctx, &s4wave_bucket_lookup.GetRefRequest{})
	if err != nil {
		t.Fatalf("post-close cursor RPC: %v", err)
	}
	if refResp.GetRef().GetBucketId() != base.GetOpArgs().GetBucketId() {
		t.Fatalf("resource bucket = %q, want %q", refResp.GetRef().GetBucketId(), base.GetOpArgs().GetBucketId())
	}
	if !resourceCtx.ReleaseResource(resp.GetResourceId()) {
		t.Fatal("resource cleanup was not registered")
	}
}

func TestEngineResourceOwnedCursorRegistrationFailureReleases(t *testing.T) {
	ctx := context.Background()
	var releases atomic.Int32
	storage := newOwnedCursorResourceTestStorage(&releases)
	engine := &ownedCursorTestEngine{storage: storage}
	resource := resource_world.NewEngineResource(nil, nil, engine, nil, nil)
	wantErr := errors.New("registration failed")
	resourceCtx := &failingOwnedCursorResourceContext{
		worldStateOperationResourceContext: &worldStateOperationResourceContext{ctx: ctx},
		err:                                wantErr,
	}

	_, err := resource.AccessWorldState(
		resource_server.WithResourceClientContext(ctx, resourceCtx),
		&s4wave_world.AccessWorldStateRequest{},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("registration error = %v, want %v", err, wantErr)
	}
	if releases.Load() != 1 {
		t.Fatalf("release count after failed registration = %d, want 1", releases.Load())
	}
}
