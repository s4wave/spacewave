//go:build !js

package resource_world_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
	"github.com/sirupsen/logrus"
)

// TestRemoteNestedWorldResourceReleaseReturnsServerCountToBaseline proves that
// retiring an immutable nested snapshot releases its server resource without
// retiring the enclosing transaction or the connection.
func TestRemoteNestedWorldResourceReleaseReturnsServerCountToBaseline(t *testing.T) {
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	defer tb.Release()

	client, server, cleanup := setupCountingResourceClient(ctx, t, tb)
	defer cleanup()
	root := client.AccessRootResource()
	defer root.Release()
	rpc, err := root.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	created, err := s4wave_testbed.NewSRPCTestbedResourceServiceClient(rpc).CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		t.Fatal(err)
	}
	ref := client.CreateResourceReference(created.GetResourceId())
	engine, err := s4wave_world.NewEngine(client, ref)
	if err != nil {
		ref.Release()
		t.Fatal(err)
	}
	defer engine.Release()
	storageRef := client.CreateResourceReference(created.GetResourceId())
	storage, err := sdk_world_engine.NewSDKEngine(client, storageRef)
	if err != nil {
		storageRef.Release()
		t.Fatal(err)
	}
	defer storage.Release()

	// Publish a typed outer object whose root points at an immutable snapshot.
	snapshot, err := world_block.BuildSnapshot(ctx, tb.Logger, storage, func(ctx context.Context, state *world_block.WorldState) error {
		_, _, err := world.AccessWorldObject(ctx, state, "inner", true, func(cursor *block.Cursor) error {
			cursor.SetBlock(block_mock.NewExample("nested content"), true)
			return nil
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	const outerKey = "example/nested"
	err = world.ExecTransaction(ctx, storage, true, func(ctx context.Context, state world.WorldState) error {
		_, _, err := world.AccessWorldObject(ctx, state, outerKey, true, func(cursor *block.Cursor) error {
			nested, err := world_block.NewNestedWorld(tb.EngineBucketID, snapshot, nil)
			if err != nil {
				return err
			}
			cursor.SetBlock(nested, true)
			return nil
		})
		if err != nil {
			return err
		}
		return world_types.SetObjectType(ctx, state, outerKey, "example/custom")
	})
	if err != nil {
		t.Fatal(err)
	}

	outer, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer outer.Release()
	baseline := server.CountTrackedResources()

	for i := range 10 {
		nested, err := outer.OpenNestedWorld(ctx, outerKey)
		if err != nil {
			t.Fatal(err)
		}
		if got := server.CountTrackedResources(); got != baseline+1 {
			nested.Release()
			t.Fatalf("iteration %d: open count = %d, want %d", i, got, baseline+1)
		}
		obj, found, err := nested.GetObject(ctx, "inner")
		if err != nil || !found {
			world.ReleaseObjectState(obj)
			nested.Release()
			t.Fatalf("iteration %d: nested lookup: found %v, error %v", i, found, err)
		}
		var body *block_mock.Example
		_, _, err = world.AccessObjectState(ctx, obj, false, func(cursor *block.Cursor) error {
			var decodeErr error
			body, decodeErr = block.UnmarshalBlock[*block_mock.Example](ctx, cursor, block_mock.NewExampleBlock)
			return decodeErr
		})
		world.ReleaseObjectState(obj)
		nested.Release()
		if err != nil {
			t.Fatal(err)
		}
		if body.GetMsg() != "nested content" {
			t.Fatalf("iteration %d: nested result = %q", i, body.GetMsg())
		}
		waitForTrackedResourceCount(t, server, baseline)

		// The enclosing transaction remains usable after its child is released.
		obj, found, err = outer.GetObject(ctx, outerKey)
		world.ReleaseObjectState(obj)
		if err != nil || !found {
			t.Fatalf("iteration %d: outer lookup: found %v, error %v", i, found, err)
		}
		waitForTrackedResourceCount(t, server, baseline)
	}
}

// TestRemoteGetObjectReleaseReturnsServerCountToBaseline proves the real
// ResourceClient/ResourceServer value-only lookup seam left by the World
// object-state release fix. A repeated remote GetObject acquires exactly one
// server-side resource handle, and releasing the returned ObjectState returns
// the owner-side handle count to its baseline every iteration.
//
// It exercises the server-side object-state allocation and release owner:
// WorldStateResource.GetObject allocates a resource through
// resourceCtx.AddResource (core/resource/world/world-state.go GetObject), and
// ObjectState.Release queues the owning client's final Release control. A
// regressed release path would leave CountTrackedResources at baseline+1 after
// an iteration.
func TestRemoteGetObjectReleaseReturnsServerCountToBaseline(t *testing.T) {
	ctx := context.Background()

	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	resClient, server, cleanup := setupCountingResourceClient(ctx, t, tb)
	defer cleanup()

	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}

	testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(rootClient)
	createWorldResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		t.Fatal(err.Error())
	}
	engineRef := resClient.CreateResourceReference(createWorldResp.GetResourceId())
	engine, err := s4wave_world.NewEngine(resClient, engineRef)
	if err != nil {
		engineRef.Release()
		t.Fatal(err.Error())
	}
	defer engine.Release()

	const objectKey = "leak-probe/object"

	// Seed one committed object so the repeated lookup returns a real handle.
	writeTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	obj, err := writeTx.CreateObject(ctx, objectKey, nil)
	if err != nil {
		writeTx.Release()
		t.Fatal(err.Error())
	}
	world.ReleaseObjectState(obj)
	if err := writeTx.Commit(ctx); err != nil {
		writeTx.Release()
		t.Fatal(err.Error())
	}
	writeTx.Release()

	// One long-lived read transaction is the stable part of the baseline.
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readTx.Release()

	baseline := server.CountTrackedResources()

	const iterations = 25
	for i := range iterations {
		got, found, err := readTx.GetObject(ctx, objectKey)
		if err != nil {
			t.Fatalf("iteration %d: GetObject: %v", i, err)
		}
		if !found {
			t.Fatalf("iteration %d: committed object not found through remote lookup", i)
		}

		acquired := server.CountTrackedResources()
		if acquired != baseline+1 {
			t.Fatalf("iteration %d: server count after GetObject = %d, want baseline+1 = %d", i, acquired, baseline+1)
		}

		world.ReleaseObjectState(got)

		waitForTrackedResourceCount(t, server, baseline)
	}

	if final := server.CountTrackedResources(); final != baseline {
		t.Fatalf("final server count = %d, want baseline = %d", final, baseline)
	}
}

func waitForTrackedResourceCount(
	t *testing.T,
	server *resource_server.ResourceServer,
	want int,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if got := server.WaitTrackedResourceCount(ctx, want); got != want {
		t.Fatalf("server resource count after release = %d, want %d", got, want)
	}
}

// setupCountingResourceClient wires a real in-process resource client to a real
// ResourceServer over an in-memory pipe and returns the server so tests can
// observe its live handle count. It mirrors resource_testbed.SetupResourceClient
// but retains the owning ResourceServer and captures the accept-loop result so
// cleanup proves the server goroutine exits on transport close.
func setupCountingResourceClient(
	ctx context.Context,
	t *testing.T,
	tb *world_testbed.Testbed,
) (*resource_client.Client, *resource_server.ResourceServer, func()) {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	clientPipe, serverPipe := net.Pipe()
	closePipes := func() {
		if err := clientPipe.Close(); err != nil {
			t.Errorf("close client pipe: %v", err)
		}
		if err := serverPipe.Close(); err != nil {
			t.Errorf("close server pipe: %v", err)
		}
	}

	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		closePipes()
		t.Fatal(err.Error())
	}
	srpcClient := srpc.NewClientWithMuxedConn(clientMp)

	mux := srpc.NewMux()
	srpcServer := srpc.NewServer(mux)

	testbedResource := resource_testbed.NewTestbedResourceServer(ctx, le, tb.Bus, tb.Volume.GetID(), tb.BucketId)
	resourceServer := resource_server.NewResourceServer(testbedResource.GetMux())
	if err := resourceServer.Register(mux); err != nil {
		closePipes()
		t.Fatal(err.Error())
	}

	serverMp, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		closePipes()
		t.Fatal(err.Error())
	}

	// Capture the accept-loop result. AcceptMuxedConn returns a non-nil
	// shutdown error once the transport closes, so cleanup can prove the server
	// goroutine exited rather than trusting a timeout.
	acceptErrCh := make(chan error, 1)
	go func() {
		acceptErrCh <- srpcServer.AcceptMuxedConn(ctx, serverMp)
	}()

	resourceServiceClient := resource.NewSRPCResourceServiceClient(srpcClient)
	resClient, err := resource_client.NewClient(ctx, resourceServiceClient)
	if err != nil {
		closePipes()
		acceptErr := <-acceptErrCh
		if acceptErr == nil {
			t.Errorf("AcceptMuxedConn returned nil; want a shutdown error proving the accept goroutine exited")
		}
		t.Fatal(err.Error())
	}

	cleanup := func() {
		resClient.Release()
		closePipes()
		acceptErr := <-acceptErrCh
		if acceptErr == nil {
			t.Errorf("AcceptMuxedConn returned nil; want a shutdown error proving the accept goroutine exited")
		}
	}

	return resClient, resourceServer, cleanup
}
