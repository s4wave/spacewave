//go:build !js

package resource_server_test

import (
	"context"
	"net"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/sirupsen/logrus"
)

// TestRemoteGetObjectReleaseReturnsServerCountToBaseline proves the real
// ResourceClient/ResourceServer value-only lookup seam left by the World
// object-state release fix. A repeated remote GetObject acquires exactly one
// server-side resource handle, and releasing the returned ObjectState returns
// the owner-side handle count to its baseline every iteration.
//
// It exercises the server-side object-state allocation and release owner:
// WorldStateResource.GetObject allocates a resource through
// resourceCtx.AddResource (core/resource/world/world-state.go GetObject), and
// ObjectState.Release triggers ResourceRefRelease, which removes the tracked
// handle (bldr/resource/server/server.go ResourceRefRelease). A regressed
// release path would leave CountTrackedResources at baseline+1 after an
// iteration.
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

		released := server.CountTrackedResources()
		if released != baseline {
			t.Fatalf("iteration %d: server count after release = %d, want baseline = %d", i, released, baseline)
		}
	}

	if final := server.CountTrackedResources(); final != baseline {
		t.Fatalf("final server count = %d, want baseline = %d", final, baseline)
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
