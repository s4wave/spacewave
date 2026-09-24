package git_lfs

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_unixfs "github.com/s4wave/spacewave/core/resource/unixfs"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	unixfs_sdk "github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
	"github.com/sirupsen/logrus"
)

// newTestUnixFSStore serves an empty UnixFS object through a resource server
// over an in-memory SRPC connection and returns a UnixFSStore on its root
// handle, along with the server for resource accounting.
func newTestUnixFSStore(t *testing.T) (*UnixFSStore, *resource_server.ResourceServer) {
	t.Helper()
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())

	// Create the UnixFS object in a test world.
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)
	opc := world.NewLookupOpController("test-fs-ops", wtb.EngineID, unixfs_world.LookupFsOp)
	if _, err := wtb.Bus.AddController(ctx, opc, nil); err != nil {
		t.Fatal(err)
	}
	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	fsType := unixfs_world.FSType_FSType_FS_NODE
	if _, _, err := unixfs_world.FsInit(ctx, ws, sender, "test-fs", fsType, nil, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	rootCursor, err := unixfs_world.FollowUnixfsRef(
		ctx,
		le,
		ws,
		&unixfs_world.UnixfsRef{ObjectKey: "test-fs"},
		sender,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rootCursor.Release)
	rootHandle, err := unixfs_sdk.NewFSHandle(rootCursor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rootHandle.Release)

	// Serve the root handle resource over an in-memory connection.
	rootResource := resource_unixfs.NewFSHandleObjectResource(le, rootHandle, nil, ws, "test-fs", fsType, nil)
	resourceSrv := resource_server.NewResourceServer(rootResource.GetMux())
	serverMux := srpc.NewMux()
	if err := resourceSrv.Register(serverMux); err != nil {
		t.Fatal(err)
	}
	clientPipe, serverPipe := net.Pipe()
	t.Cleanup(func() {
		_ = clientPipe.Close()
		_ = serverPipe.Close()
	})
	serverMp, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = srpc.NewServer(serverMux).AcceptMuxedConn(ctx, serverMp)
	}()

	// Connect a resource client and open the root handle.
	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	resClient, err := resource_client.NewClient(
		ctx,
		resource.NewSRPCResourceServiceClient(srpc.NewClientWithMuxedConn(clientMp)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resClient.Release)
	rootRef := resClient.AccessRootResource()
	t.Cleanup(rootRef.Release)
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	root := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)
	return NewUnixFSStore(root, resClient), resourceSrv
}

// TestUnixFSStore stores, replaces, and fetches objects spanning several
// chunks, and releases every handle it opens.
func TestUnixFSStore(t *testing.T) {
	ctx := t.Context()
	store, resourceSrv := newTestUnixFSStore(t)
	baseline := resourceSrv.CountTrackedResources()

	// A missing object is absent and cannot be fetched.
	data := bytes.Repeat([]byte("0123456789abcdef"), (3*chunkSize+1000)/16)
	_, oid := testObject(string(data))
	if has, err := store.Has(ctx, oid, int64(len(data))); err != nil || has {
		t.Fatalf("Has(missing) = %v, %v; want false", has, err)
	}
	if err := store.Get(ctx, oid, int64(len(data)), &bytes.Buffer{}); err == nil {
		t.Fatal("Get(missing) succeeded")
	}

	// A wrong-size entry is replaced by the correct object.
	if err := store.Put(ctx, oid, 5, bytes.NewReader([]byte("stale"))); err != nil {
		t.Fatal(err)
	}
	if has, err := store.Has(ctx, oid, int64(len(data))); err != nil || has {
		t.Fatalf("Has(wrong size) = %v, %v; want false", has, err)
	}
	if err := store.Put(ctx, oid, int64(len(data)), bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
	if has, err := store.Has(ctx, oid, int64(len(data))); err != nil || !has {
		t.Fatalf("Has(stored) = %v, %v; want true", has, err)
	}

	// The object reads back in order across the read window.
	var got bytes.Buffer
	if err := store.Get(ctx, oid, int64(len(data)), &got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), data) {
		t.Fatalf("Get returned %d bytes that differ from the %d stored", got.Len(), len(data))
	}

	// A reader shorter than the declared size fails the Put.
	_, shortOid := testObject("short")
	if err := store.Put(ctx, shortOid, 100, bytes.NewReader([]byte("short"))); err == nil {
		t.Fatal("Put(short reader) succeeded")
	}

	// Every lookup handle was released.
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if count := resourceSrv.WaitTrackedResourceCount(waitCtx, baseline); count != baseline {
		t.Fatalf("tracked resources = %d, want %d", count, baseline)
	}
}
