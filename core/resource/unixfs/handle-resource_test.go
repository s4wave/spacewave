package resource_unixfs

import (
	"context"
	"net"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	billy_util "github.com/go-git/go-billy/v6/util"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	unixfs_sdk "github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
	"github.com/sirupsen/logrus"
)

func setupFSHandleResourceClient(
	t *testing.T,
) (
	context.Context,
	*resource_client.Client,
	*unixfs_sdk.FSHandle,
	func(),
) {
	t.Helper()

	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	opc := world.NewLookupOpController(
		"test-fs-ops",
		wtb.EngineID,
		unixfs_world.LookupFsOp,
	)
	if _, err := wtb.Bus.AddController(ctx, opc, nil); err != nil {
		t.Fatal(err)
	}
	<-time.After(time.Millisecond * 100)

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	fsType := unixfs_world.FSType_FSType_FS_NODE
	if _, _, err := unixfs_world.FsInit(
		ctx,
		ws,
		sender,
		"test-fs",
		fsType,
		nil,
		true,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}

	rootCursor, err := unixfs_world.FollowUnixfsRef(
		ctx,
		wtb.Logger,
		ws,
		&unixfs_world.UnixfsRef{ObjectKey: "test-fs"},
		sender,
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	rootHandle, err := unixfs_sdk.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}

	bfs := unixfs_billy.NewBillyFS(ctx, rootHandle, "", time.Now())
	if err := bfs.MkdirAll("src", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := bfs.MkdirAll("dest", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := billy_util.WriteFile(bfs, "src/file.txt", []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	clientPipe, serverPipe := net.Pipe()

	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	srpcClient := srpc.NewClientWithMuxedConn(clientMp)

	rootMux := NewFSHandleObjectResource(
		rootHandle,
		nil,
		ws,
		"test-fs",
		fsType,
		nil,
	).GetMux()
	resourceSrv := resource_server.NewResourceServer(rootMux)
	serverMux := srpc.NewMux()
	if err := resourceSrv.Register(serverMux); err != nil {
		t.Fatal(err)
	}
	server := srpc.NewServer(serverMux)

	serverMp, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = server.AcceptMuxedConn(ctx, serverMp)
	}()

	resourceSvc := resource.NewSRPCResourceServiceClient(srpcClient)
	resClient, err := resource_client.NewClient(ctx, resourceSvc)
	if err != nil {
		t.Fatal(err)
	}

	cleanup := func() {
		resClient.Release()
		rootHandle.Release()
		rootCursor.Release()
		wtb.Release()
		clientPipe.Close()
		serverPipe.Close()
	}

	return ctx, resClient, rootHandle, cleanup
}

func TestFSHandleResourceRenameCrossDirectory(t *testing.T) {
	ctx, resClient, rootHandle, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()

	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	srcResp, err := rootSvc.Lookup(ctx, &s4wave_unixfs.HandleLookupRequest{Name: "src"})
	if err != nil {
		t.Fatal(err)
	}
	srcRef := resClient.CreateResourceReference(srcResp.GetResourceId())
	defer srcRef.Release()

	srcClient, err := srcRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	srcSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(srcClient)

	destResp, err := rootSvc.Lookup(ctx, &s4wave_unixfs.HandleLookupRequest{Name: "dest"})
	if err != nil {
		t.Fatal(err)
	}
	destRef := resClient.CreateResourceReference(destResp.GetResourceId())
	defer destRef.Release()

	if _, err := srcSvc.Rename(ctx, &s4wave_unixfs.HandleRenameRequest{
		SourceName:           "file.txt",
		DestName:             "moved.txt",
		DestParentResourceId: destResp.GetResourceId(),
	}); err != nil {
		t.Fatal(err)
	}

	movedResp, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "dest/moved.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	movedRef := resClient.CreateResourceReference(movedResp.GetResourceId())
	movedRef.Release()

	if _, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "src/file.txt",
	}); err == nil {
		t.Fatal("expected old path lookup to fail after move")
	}

	bfs := unixfs_billy.NewBillyFS(ctx, rootHandle, "", time.Now())
	data, err := billy_util.ReadFile(bfs, "dest/moved.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("got data %q, want %q", string(data), "hello")
	}
}

func TestFSHandleResourceWatchReaddirSeesSiblingRename(t *testing.T) {
	ctx, resClient, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()

	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	srcResp, err := rootSvc.Lookup(ctx, &s4wave_unixfs.HandleLookupRequest{Name: "src"})
	if err != nil {
		t.Fatal(err)
	}
	srcRef := resClient.CreateResourceReference(srcResp.GetResourceId())
	defer srcRef.Release()

	srcClient, err := srcRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	srcSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(srcClient)

	destResp, err := rootSvc.Lookup(ctx, &s4wave_unixfs.HandleLookupRequest{Name: "dest"})
	if err != nil {
		t.Fatal(err)
	}
	destRef := resClient.CreateResourceReference(destResp.GetResourceId())
	defer destRef.Release()

	watchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	srcWatch, err := srcSvc.WatchReaddir(
		watchCtx,
		&s4wave_unixfs.HandleWatchReaddirRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}

	initial, err := srcWatch.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(extractEntryNames(initial.GetEntries()), []string{"file.txt"}) {
		t.Fatalf("unexpected initial entries: %v", extractEntryNames(initial.GetEntries()))
	}

	if _, err := srcSvc.Rename(ctx, &s4wave_unixfs.HandleRenameRequest{
		SourceName:           "file.txt",
		DestName:             "moved.txt",
		DestParentResourceId: destResp.GetResourceId(),
	}); err != nil {
		t.Fatal(err)
	}

	updated, err := srcWatch.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.GetEntries()) != 0 {
		t.Fatalf("expected src watch to become empty after sibling rename, got %v", extractEntryNames(updated.GetEntries()))
	}
}

func TestFSHandleResourceUploadTreeNested(t *testing.T) {
	ctx, resClient, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()

	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	strm, err := rootSvc.UploadTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_Directory{
			Directory: &s4wave_unixfs.HandleUploadTreeDirectory{
				Path: "nested/empty",
				Mode: 0o755,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
			FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
				Path:      "nested/child.txt",
				TotalSize: 5,
				Mode:      0o644,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_Data{
			Data: []byte("hello"),
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
			FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
				Path:      "top.txt",
				TotalSize: 3,
				Mode:      0o644,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_Data{
			Data: []byte("top"),
		},
	}); err != nil {
		t.Fatal(err)
	}
	resp, err := strm.CloseAndRecv()
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetBytesWritten() != 8 {
		t.Fatalf("bytes_written = %d, want %d", resp.GetBytesWritten(), 8)
	}
	if resp.GetFilesWritten() != 2 {
		t.Fatalf("files_written = %d, want %d", resp.GetFilesWritten(), 2)
	}
	if resp.GetDirectoriesWritten() != 1 {
		t.Fatalf("directories_written = %d, want %d", resp.GetDirectoriesWritten(), 1)
	}

	childResp, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "nested/child.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	childRef := resClient.CreateResourceReference(childResp.GetResourceId())
	defer childRef.Release()

	childClient, err := childRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	childSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(childClient)
	readResp, err := childSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{
		Length: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(readResp.GetData()) != "hello" {
		t.Fatalf("got data %q, want %q", string(readResp.GetData()), "hello")
	}

	topResp, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "top.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	topRef := resClient.CreateResourceReference(topResp.GetResourceId())
	defer topRef.Release()

	topClient, err := topRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	topSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(topClient)
	readResp, err = topSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{
		Length: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(readResp.GetData()) != "top" {
		t.Fatalf("got data %q, want %q", string(readResp.GetData()), "top")
	}

	if _, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "nested/empty",
	}); err != nil {
		t.Fatal(err)
	}
}

func extractEntryNames(entries []*s4wave_unixfs.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.GetName())
	}
	slices.Sort(names)
	return names
}
