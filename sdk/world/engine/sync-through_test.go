//go:build !js

package sdk_world_engine_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/testbed"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
	"github.com/sirupsen/logrus"
)

// gatedAuthority holds one armed commit at its Admit, so the engine has
// sealed it but it is not durable.
type gatedAuthority struct {
	*world_block.LocalAuthority
	mu      sync.Mutex
	entered chan struct{}
	release chan struct{}
}

// arm holds the next commit until the returned release function runs.
func (a *gatedAuthority) arm() (entered <-chan struct{}, release func()) {
	// Replace the gate under the lock.
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entered, a.release = make(chan struct{}), make(chan struct{})
	rel := a.release
	var once sync.Once
	return a.entered, func() { once.Do(func() { close(rel) }) }
}

// Begin opens the local turn and gates its Admit.
func (a *gatedAuthority) Begin(ctx context.Context) (world.Turn, error) {
	turn, err := a.LocalAuthority.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return &gatedTurn{Turn: turn, a: a}, nil
}

// gatedTurn is a turn of a gatedAuthority.
type gatedTurn struct {
	world.Turn
	a *gatedAuthority
}

// Admit holds an armed commit until released, then admits it.
func (t *gatedTurn) Admit(ctx context.Context, candidate world.Candidate) (world.Admission, error) {
	// Take the armed gate so only one commit is held.
	t.a.mu.Lock()
	entered, release := t.a.entered, t.a.release
	t.a.entered, t.a.release = nil, nil
	t.a.mu.Unlock()

	// Hold the commit until released.
	if entered != nil {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return t.Turn.Admit(ctx, candidate)
}

// engineRoot serves one prebuilt World engine as the root resource's CreateWorld.
type engineRoot struct {
	s4wave_testbed.SRPCTestbedResourceServiceServer
	le     *logrus.Entry
	engine *world_block.Engine
}

// CreateWorld publishes the prebuilt engine as an EngineResource.
func (r *engineRoot) CreateWorld(ctx context.Context, _ *s4wave_testbed.CreateWorldRequest) (*s4wave_testbed.CreateWorldResponse, error) {
	// Add the engine's resource to the calling client.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	res := resource_world.NewEngineResource(r.le, nil, r.engine, world_mock.LookupMockOp, &s4wave_world.EngineInfo{EngineId: "sync-through"})
	id, err := resourceCtx.AddResource(res.GetMux(), res.Close)
	if err != nil {
		return nil, err
	}
	return &s4wave_testbed.CreateWorldResponse{ResourceId: id}, nil
}

// syncThroughFixture is an engine whose commits can be held at its
// authority, served over the resource RPC.
type syncThroughFixture struct {
	engine    *world_block.Engine
	authority *gatedAuthority
	sdk       *sdk_world_engine.SDKEngine
	load      func(context.Context) (*block.BlockRef, error)
}

// newSyncThroughFixture builds the engine and attaches an SDKEngine to it.
func newSyncThroughFixture(t *testing.T) *syncThroughFixture {
	t.Helper()

	// Keep the test log quiet.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	le := logrus.NewEntry(log)

	// Open a native volume whose bucket holds roots.
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVolumeConfig(&volume_s4db.Config{
		Path: filepath.Join(t.TempDir(), "sync-through.s4wave"), VolumeConfig: &volume_controller.Config{GcIntervalDur: "1h"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cursor.Release)

	// Open the engine on the gated local authority, whose root holds the
	// durable head.
	authority := &gatedAuthority{LocalAuthority: world_block.NewLocalAuthority(le, cursor, "world:sync-through", tb.Volume,
		coord.Scope{VolumeID: tb.Volume.GetID(), ObjectStoreID: "world:sync-through", ParticipantID: "sync-through"})}
	f := &syncThroughFixture{authority: authority}
	f.load = func(ctx context.Context) (*block.BlockRef, error) {
		head, err := authority.Head(ctx)
		return head.Root, err
	}
	f.engine, err = world_block.NewEngine(ctx, le, cursor, world_mock.LookupMockOp, authority, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.engine.Close(); err != nil {
			t.Error(err)
		}
	})

	// Serve the engine over the resource RPC.
	mux := srpc.NewMux()
	if err := s4wave_testbed.SRPCRegisterTestbedResourceService(mux, &engineRoot{le: le, engine: f.engine}); err != nil {
		t.Fatal(err)
	}
	resClient, cleanup := resource_testbed.SetupResourceClientWithRoot(ctx, t, mux)
	t.Cleanup(cleanup)

	// Create the engine resource through the root and attach the SDKEngine.
	rootRef := resClient.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	created, err := s4wave_testbed.NewSRPCTestbedResourceServiceClient(rootClient).CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		t.Fatal(err)
	}

	// Attach the SDKEngine to the created resource.
	f.sdk, err = sdk_world_engine.NewSDKEngine(resClient, resClient.CreateResourceReference(created.GetResourceId()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.sdk.Release)
	return f
}

// commit creates one object in a write transaction and commits it.
func (f *syncThroughFixture) commit(ctx context.Context, key string) error {
	// Create the object in a write transaction.
	tx, err := f.sdk.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	obj, err := tx.CreateObject(ctx, key, nil) //nolint:lostresource // obj is nil when CreateObject fails.
	if err != nil {
		return err
	}
	world.ReleaseObjectState(obj)
	return tx.Commit(ctx)
}

// TestSDKEngine_SyncThroughSkipsLaterWriter checks that SyncThrough for the
// first writer's seqno returns while a second writer holds a sealed but
// undurable commit, and that the first commit is durable.
func TestSDKEngine_SyncThroughSkipsLaterWriter(t *testing.T) {
	// Start from an empty world.
	ctx := t.Context()
	f := newSyncThroughFixture(t)

	// The first writer commits durably.
	if err := f.commit(ctx, "first"); err != nil {
		t.Fatal(err)
	}
	firstSeqno, err := f.sdk.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	firstHead := f.engine.GetRootRef()

	// The second writer's commit is sealed and held before it is durable.
	entered, release := f.authority.arm()
	t.Cleanup(release)
	secondDone := make(chan error, 1)
	go func() { secondDone <- f.commit(ctx, "second") }()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("second commit never reached the gate")
	}

	// SyncThrough for the first writer returns without it.
	fenceCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	durable, err := f.sdk.SyncThrough(fenceCtx, firstSeqno)
	cancel()
	if err != nil || durable != firstSeqno {
		t.Fatalf("SyncThrough(%d) = %d, %v; want %d, nil", firstSeqno, durable, err, firstSeqno)
	}
	select {
	case err := <-secondDone:
		t.Fatalf("second commit finished while held: %v", err)
	default:
	}

	// The first writer's commit is in the durable head.
	head, err := f.load(ctx)
	if err != nil || !head.EqualsRef(firstHead.GetRootRef()) {
		t.Fatalf("durable head %v, %v; want the first commit's head %v", head, err, firstHead)
	}

	// The held commit's own seqno still waits for it.
	waitCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	_, err = f.sdk.SyncThrough(waitCtx, firstSeqno+1)
	cancel()
	if err == nil {
		t.Fatal("SyncThrough for the held commit returned before it was durable")
	}

	// Releasing the gate lets the second commit finish and become fenceable.
	release()
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	fenceCtx, cancel = context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	durable, err = f.sdk.SyncThrough(fenceCtx, firstSeqno+1)
	if err != nil || durable != firstSeqno+1 {
		t.Fatalf("SyncThrough(%d) = %d, %v; want %d, nil", firstSeqno+1, durable, err, firstSeqno+1)
	}
}
