//go:build !js

package resource_world_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	s4wave_bucket_lookup "github.com/s4wave/spacewave/sdk/bucket/lookup"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// lifetimeEngine counts open read transactions and AccessWorldState callbacks.
type lifetimeEngine struct {
	world.Engine
	openTxs       atomic.Int32
	openCallbacks atomic.Int32
}

func (e *lifetimeEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	tx, err := e.Engine.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	e.openTxs.Add(1)
	return &lifetimeTx{Tx: tx, engine: e}, nil
}

func (e *lifetimeEngine) AccessWorldState(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	return e.Engine.AccessWorldState(ctx, ref, func(c *bucket_lookup.Cursor) error {
		e.openCallbacks.Add(1)
		defer e.openCallbacks.Add(-1)
		return cb(c)
	})
}

type lifetimeTx struct {
	world.Tx
	engine    *lifetimeEngine
	discarded atomic.Bool
}

func (tx *lifetimeTx) Discard() {
	if tx.discarded.CompareAndSwap(false, true) {
		tx.engine.openTxs.Add(-1)
	}
	tx.Tx.Discard()
}

// watchWorldStateTestStream records WatchWorldState responses.
type watchWorldStateTestStream struct {
	srpc.Stream
	ctx  context.Context
	sent chan *s4wave_world.WatchWorldStateResponse
}

func (s *watchWorldStateTestStream) Context() context.Context { return s.ctx }

func (s *watchWorldStateTestStream) Send(resp *s4wave_world.WatchWorldStateResponse) error {
	s.sent <- resp
	return nil
}

func (s *watchWorldStateTestStream) SendAndClose(resp *s4wave_world.WatchWorldStateResponse) error {
	return s.Send(resp)
}

func waitForCount(t *testing.T, name string, count *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for count.Load() != want {
		if time.Now().After(deadline) {
			t.Fatalf("%s = %d, want %d", name, count.Load(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestWatchWorldStateDiscardsTrackedTransaction proves each tracked
// WatchWorldState snapshot transaction is discarded when the watch ends.
func TestWatchWorldStateDiscardsTrackedTransaction(t *testing.T) {
	ctx := t.Context()
	tb, cleanup := setupWorldTestbed(ctx, t)
	defer cleanup()

	engine := &lifetimeEngine{Engine: tb.Engine}
	resourceCtx := &worldStateOperationResourceContext{ctx: ctx}
	watchCtx, watchCancel := context.WithCancel(resource_server.WithResourceClientContext(ctx, resourceCtx))
	defer watchCancel()

	le := logrus.NewEntry(logrus.New())
	engineResource := resource_world.NewEngineResource(le, tb.Bus, engine, nil, nil)
	defer engineResource.Close()

	stream := &watchWorldStateTestStream{ctx: watchCtx, sent: make(chan *s4wave_world.WatchWorldStateResponse, 1)}
	errCh := make(chan error, 1)
	go func() {
		errCh <- engineResource.WatchWorldState(&s4wave_world.WatchWorldStateRequest{}, stream)
	}()

	var tracked *s4wave_world.WatchWorldStateResponse
	select {
	case tracked = <-stream.sent:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for tracked world state")
	}
	if got := engine.openTxs.Load(); got != 1 {
		t.Fatalf("open transactions while watching = %d, want 1", got)
	}

	// Open a child resource from the tracked world state.
	trackedClient, err := resourceCtx.GetAttachedResource(tracked.GetResourceId())
	if err != nil {
		t.Fatal(err.Error())
	}
	trackedService := s4wave_world.NewSRPCWorldStateResourceServiceClient(trackedClient)
	child, err := trackedService.AccessWorldState(resource_server.WithResourceClientContext(ctx, resourceCtx), &s4wave_world.AccessWorldStateRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}

	watchCancel()
	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for watch to return")
	}

	// The adopted child keeps the snapshot transaction open.
	if got := engine.openTxs.Load(); got != 1 {
		t.Fatalf("open transactions with live child = %d, want 1", got)
	}
	if !resourceCtx.ReleaseResource(child.GetResourceId()) {
		t.Fatal("child resource was not registered")
	}
	waitForCount(t, "open transactions after child release", &engine.openTxs, 0)
}

// TestAccessWorldStateCursorLivesUntilRelease proves the AccessWorldState
// cursor stays inside its engine callback until the resource is released.
func TestAccessWorldStateCursorLivesUntilRelease(t *testing.T) {
	ctx := t.Context()
	tb, cleanup := setupWorldTestbed(ctx, t)
	defer cleanup()

	engine := &lifetimeEngine{Engine: tb.Engine}
	resourceCtx := &worldStateOperationResourceContext{ctx: ctx}
	rpcCtx := resource_server.WithResourceClientContext(ctx, resourceCtx)

	le := logrus.NewEntry(logrus.New())
	engineResource := resource_world.NewEngineResource(le, tb.Bus, engine, nil, nil)
	defer engineResource.Close()

	resp, err := engineResource.AccessWorldState(rpcCtx, &s4wave_world.AccessWorldStateRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	if got := engine.openCallbacks.Load(); got != 1 {
		t.Fatalf("open AccessWorldState callbacks = %d, want 1", got)
	}

	// The cursor remains usable after the RPC returns.
	client, err := resourceCtx.GetAttachedResource(resp.GetResourceId())
	if err != nil {
		t.Fatal(err.Error())
	}
	cursorService := s4wave_bucket_lookup.NewSRPCBucketLookupCursorResourceServiceClient(client)
	if _, err := cursorService.GetRef(ctx, &s4wave_bucket_lookup.GetRefRequest{}); err != nil {
		t.Fatal(err.Error())
	}

	if !resourceCtx.ReleaseResource(resp.GetResourceId()) {
		t.Fatal("cursor resource was not registered")
	}
	waitForCount(t, "open AccessWorldState callbacks after release", &engine.openCallbacks, 0)
}
