package resource_unixfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"path"
	"slices"
	"sync"
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
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
	"github.com/sirupsen/logrus"
)

type uploadMetricRecorder struct {
	metrics []UploadMetric
}

func (r *uploadMetricRecorder) RecordUploadMetric(metric UploadMetric) {
	r.metrics = append(r.metrics, metric)
}

func (r *uploadMetricRecorder) stages() []string {
	stages := make([]string, 0, len(r.metrics))
	for _, metric := range r.metrics {
		stages = append(stages, metric.Stage)
	}
	return stages
}

func (r *uploadMetricRecorder) totalBytes(stage string) int {
	var total int
	for _, metric := range r.metrics {
		if metric.Stage == stage {
			total += metric.Bytes
		}
	}
	return total
}

func (r *uploadMetricRecorder) countStage(stage string) int {
	var count int
	for _, metric := range r.metrics {
		if metric.Stage == stage {
			count++
		}
	}
	return count
}

type uploadTreeMetricStream struct {
	ctx      context.Context
	messages []*s4wave_unixfs.HandleUploadTreeRequest
}

func (s *uploadTreeMetricStream) Context() context.Context { return s.ctx }

func (s *uploadTreeMetricStream) MsgSend(srpc.Message) error { return nil }

func (s *uploadTreeMetricStream) MsgRecv(msg srpc.Message) error {
	out, ok := msg.(*s4wave_unixfs.HandleUploadTreeRequest)
	if !ok {
		return errors.New("unexpected upload tree message type")
	}
	return s.RecvTo(out)
}

func (s *uploadTreeMetricStream) CloseSend() error { return nil }

func (s *uploadTreeMetricStream) Close() error { return nil }

func (s *uploadTreeMetricStream) Recv() (*s4wave_unixfs.HandleUploadTreeRequest, error) {
	if len(s.messages) == 0 {
		return nil, io.EOF
	}
	msg := s.messages[0]
	s.messages = s.messages[1:]
	return msg, nil
}

func (s *uploadTreeMetricStream) RecvTo(out *s4wave_unixfs.HandleUploadTreeRequest) error {
	msg, err := s.Recv()
	if err != nil {
		return err
	}
	*out = *msg
	return nil
}

func assertUploadTreeCounters(
	t *testing.T,
	resp *s4wave_unixfs.HandleUploadTreeResponse,
	bytesWritten int64,
	filesWritten int64,
	directoriesWritten int64,
) {
	t.Helper()
	written := resp.GetWritten()
	if written == nil {
		t.Fatalf("upload did not commit: %v", resp.GetConflict())
	}
	if written.GetBytesWritten() != bytesWritten {
		t.Fatalf("bytes_written = %d, want %d", written.GetBytesWritten(), bytesWritten)
	}
	if written.GetFilesWritten() != filesWritten {
		t.Fatalf("files_written = %d, want %d", written.GetFilesWritten(), filesWritten)
	}
	if written.GetDirectoriesWritten() != directoriesWritten {
		t.Fatalf("directories_written = %d, want %d", written.GetDirectoriesWritten(), directoriesWritten)
	}
}

func sendUploadTreeFile(
	t *testing.T,
	strm s4wave_unixfs.SRPCFSHandleResourceService_UploadTreeClient,
	path string,
	data []byte,
) {
	t.Helper()
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
			FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
				Path:      path,
				TotalSize: int64(len(data)),
				Mode:      0o644,
				Expect:    &s4wave_unixfs.HandleUploadTreeFileStart_Overwrite{Overwrite: true},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_Data{Data: data},
	}); err != nil {
		t.Fatal(err)
	}
}

func setupFSHandleResourceClient(
	t *testing.T,
) (
	context.Context,
	*resource_client.Client,
	*unixfs_sdk.FSHandle,
	*FSHandleResource,
	func(),
) {
	// Create the logger and context for the filesystem testbed.
	t.Helper()
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the block storage testbed for the filesystem World.
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	// Start the World engine on the block storage testbed.
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	// Register the filesystem operations with the World engine.
	opc := world.NewLookupOpController(
		"test-fs-ops",
		wtb.EngineID,
		world.NewSchema(unixfs_world.UnixfsTypes).LookupOp,
	)
	if _, err := wtb.Bus.AddController(ctx, opc, nil); err != nil {
		t.Fatal(err)
	}
	<-time.After(time.Millisecond * 100)

	// Initialize the test filesystem object in the World.
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

	// Open the initialized filesystem root and its handle.
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

	// Populate the source and destination directories used by the resource tests.
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

	// Connect the resource client to the server through an in-memory pipe.
	clientPipe, serverPipe := net.Pipe()
	clientMp, err := srpc.NewMuxedConn(clientPipe, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	srpcClient := srpc.NewClientWithMuxedConn(clientMp)

	// Register the filesystem root service with the resource server.
	rootResource := NewFSHandleObjectResource(
		logrus.NewEntry(log),
		rootHandle,
		rootCursor,
		nil,
		ws,
		"test-fs",
		fsType,
		nil,
	)
	rootMux := rootResource.GetMux()
	resourceSrv := resource_server.NewResourceServer(rootMux)
	serverMux := srpc.NewMux()
	if err := resourceSrv.Register(serverMux); err != nil {
		t.Fatal(err)
	}
	server := srpc.NewServer(serverMux)

	// Start the server RPC connection on the other end of the pipe.
	serverMp, err := srpc.NewMuxedConn(serverPipe, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		_ = server.AcceptMuxedConn(ctx, serverMp)
	}()

	// Create the resource client for the connected server.
	resourceSvc := resource.NewSRPCResourceServiceClient(srpcClient)
	resClient, err := resource_client.NewClient(ctx, resourceSvc)
	if err != nil {
		t.Fatal(err)
	}

	// Provide cleanup for the client, filesystem handles, World, and connections.
	cleanup := func() {
		// Release the filesystem testbed and both resource connections.
		resClient.Release()
		rootHandle.Release()
		rootCursor.Release()
		wtb.Release()
		clientPipe.Close()
		serverPipe.Close()
	}

	return ctx, resClient, rootHandle, rootResource, cleanup
}

func TestFSHandleResourceRenameCrossDirectory(t *testing.T) {
	// Start the filesystem resource testbed for a move between directories.
	ctx, resClient, rootHandle, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Retain the filesystem root resource for the test.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()

	// Open the filesystem root RPC service.
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Retain the source directory resource for the move.
	srcResp, err := rootSvc.Lookup(ctx, &s4wave_unixfs.HandleLookupRequest{Name: "src"})
	if err != nil {
		t.Fatal(err)
	}
	srcRef := resClient.CreateResourceReference(srcResp.GetResourceId())
	defer srcRef.Release()

	// Open the source directory RPC service.
	srcClient, err := srcRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	srcSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(srcClient)

	// Retain the destination directory resource for the move.
	destResp, err := rootSvc.Lookup(ctx, &s4wave_unixfs.HandleLookupRequest{Name: "dest"})
	if err != nil {
		t.Fatal(err)
	}
	destRef := resClient.CreateResourceReference(destResp.GetResourceId())
	defer destRef.Release()

	// Move the source file into the destination directory through RPC.
	if _, err := srcSvc.Rename(ctx, &s4wave_unixfs.HandleRenameRequest{
		SourceName:           "file.txt",
		DestName:             "moved.txt",
		DestParentResourceId: destResp.GetResourceId(),
	}); err != nil {
		t.Fatal(err)
	}

	// Require the moved file to be accessible at its destination path.
	movedResp, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "dest/moved.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	movedRef := resClient.CreateResourceReference(movedResp.GetResourceId())
	movedRef.Release()

	// Require the original file path to disappear after the move.
	if _, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "src/file.txt",
	}); err == nil {
		t.Fatal("expected old path lookup to fail after move")
	}

	// Read the moved file directly from the filesystem and verify its content.
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
	// Start the filesystem resource testbed for directory change notifications.
	ctx, resClient, _, rootResource, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Retain the filesystem root resource for the test.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()

	// Open the filesystem root RPC service.
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Retain the source directory resource for the watch.
	srcResp, err := rootSvc.Lookup(ctx, &s4wave_unixfs.HandleLookupRequest{Name: "src"})
	if err != nil {
		t.Fatal(err)
	}
	srcRef := resClient.CreateResourceReference(srcResp.GetResourceId())
	defer srcRef.Release()

	// Open the source directory RPC service.
	srcClient, err := srcRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	srcSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(srcClient)

	// Retain the destination directory resource for the sibling move.
	destResp, err := rootSvc.Lookup(ctx, &s4wave_unixfs.HandleLookupRequest{Name: "dest"})
	if err != nil {
		t.Fatal(err)
	}
	destRef := resClient.CreateResourceReference(destResp.GetResourceId())
	defer destRef.Release()

	// Watch source directory changes with a bounded test context.
	watchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	srcWatch, err := srcSvc.WatchReaddir(
		watchCtx,
		&s4wave_unixfs.HandleWatchReaddirRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the initial source directory listing contains the seeded file.
	initial, err := srcWatch.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(extractEntryNames(initial.GetEntries()), []string{"file.txt"}) {
		t.Fatalf("unexpected initial entries: %v", extractEntryNames(initial.GetEntries()))
	}
	if ent := initial.GetEntries()[0]; ent.GetSize() != 5 || ent.GetModTime() == 0 || ent.GetMode() == 0 {
		t.Fatalf("initial entry lacks its node info: %v", ent)
	}

	// Verify a local notification with unchanged entries emits no duplicate listing.
	dedupCtx, cancelDedup := context.WithCancel(ctx)
	defer cancelDedup()
	dedupWatch, err := srcSvc.WatchReaddir(
		dedupCtx,
		&s4wave_unixfs.HandleWatchReaddirRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dedupWatch.Recv(); err != nil {
		t.Fatal(err)
	}
	rootResource.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
	select {
	case dup := <-recvWatchReaddir(t, dedupWatch):
		t.Fatalf("unexpected duplicate watch emission: %v", extractEntryNames(dup.GetEntries()))
	case <-time.After(50 * time.Millisecond):
	}
	cancelDedup()

	// Move the source file to its sibling directory through RPC.
	if _, err := srcSvc.Rename(ctx, &s4wave_unixfs.HandleRenameRequest{
		SourceName:           "file.txt",
		DestName:             "moved.txt",
		DestParentResourceId: destResp.GetResourceId(),
	}); err != nil {
		t.Fatal(err)
	}

	// Require the source directory watch to remove the moved entry.
	updated, err := srcWatch.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.GetEntries()) != 0 || !slices.Equal(updated.GetRemoved(), []string{"file.txt"}) {
		t.Fatalf("expected src watch to remove file.txt, got entries %v removed %v", extractEntryNames(updated.GetEntries()), updated.GetRemoved())
	}

	// Require a write through another World cursor to reach the watch.
	bfs := unixfs_billy.NewBillyFS(ctx, rootResource.handle, "", time.Now())
	if err := billy_util.WriteFile(bfs, "src/new.txt", []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The write creates the file, then fills it, so its size arrives with the
	// last change.
	for {
		updated, err = srcWatch.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(extractEntryNames(updated.GetEntries()), []string{"new.txt"}) || len(updated.GetRemoved()) != 0 {
			t.Fatalf("unexpected change after external write: entries %v removed %v", extractEntryNames(updated.GetEntries()), updated.GetRemoved())
		}
		if ent := updated.GetEntries()[0]; ent.GetSize() == 2 && ent.GetModTime() != 0 {
			break
		}
	}

	// Fill a directory of 1,000 files.
	const bigCount = 1000
	if err := bfs.MkdirAll("big", 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range bigCount {
		if err := billy_util.WriteFile(bfs, fmt.Sprintf("big/f%04d", i), []byte("0123456789"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// readBig lists the directory at the current World state.
	var w readdirWatch
	readBig := func() int {
		root, _, err := rootResource.newObjectFSHandle(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer root.Release()
		big, err := root.Lookup(ctx, "big")
		if err != nil {
			t.Fatal(err)
		}
		defer big.Release()
		reads, err := w.read(ctx, big)
		if err != nil {
			t.Fatal(err)
		}
		return reads
	}

	// The first listing reads each entry's node once and sends them all.
	firstReads := readBig()
	if firstReads != bigCount {
		t.Fatalf("first listing read %d nodes, want %d", firstReads, bigCount)
	}
	full, ok := w.diff()
	if !ok || len(full.GetEntries()) != bigCount || full.GetEntries()[0].GetSize() != 10 {
		t.Fatalf("first listing sent %d entries", len(full.GetEntries()))
	}

	// A change to one file reads only that file's node and sends only it.
	if err := billy_util.WriteFile(bfs, "big/f0500", []byte("01234567890123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	changeReads := readBig()
	if changeReads != 1 {
		t.Fatalf("change read %d nodes, want 1", changeReads)
	}
	change, ok := w.diff()
	if !ok || len(change.GetEntries()) != 1 || change.GetEntries()[0].GetName() != "f0500" || change.GetEntries()[0].GetSize() != 20 {
		t.Fatalf("unexpected change: %v", change)
	}

	// Compare with the full listing of names and types each change sent before.
	bare := &s4wave_unixfs.HandleWatchReaddirResponse{}
	for _, ent := range full.GetEntries() {
		bare.Entries = append(bare.Entries, &s4wave_unixfs.DirEntry{Name: ent.GetName(), IsDir: ent.GetIsDir()})
	}
	t.Logf("%d entries: first listing %d node reads, %d bytes; one change %d node reads, %d bytes; bare full listing %d bytes",
		bigCount, firstReads, full.SizeVT(), changeReads, change.SizeVT(), bare.SizeVT())
}

func recvWatchReaddir(
	t *testing.T,
	watch s4wave_unixfs.SRPCFSHandleResourceService_WatchReaddirClient,
) <-chan *s4wave_unixfs.HandleWatchReaddirResponse {
	t.Helper()
	ch := make(chan *s4wave_unixfs.HandleWatchReaddirResponse, 1)
	go func() {
		resp, err := watch.Recv()
		if err != nil {
			return
		}
		ch <- resp
	}()
	return ch
}

// TestFSHandleResourceWatchFiles streams the matching files below a directory,
// then the paths each change adds and removes.
func TestFSHandleResourceWatchFiles(t *testing.T) {
	// Seed matching files at several depths beside files that do not match.
	ctx, resClient, rootHandle, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()
	bfs := unixfs_billy.NewBillyFS(ctx, rootHandle, "", time.Now())
	for _, name := range []string{"src/a.jpg", "dest/deep/c.PNG", "dest/x.txt"} {
		if err := bfs.MkdirAll(path.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := billy_util.WriteFile(bfs, name, []byte("img"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Open the filesystem root RPC service.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// A limited watch takes the first match in path order.
	watchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	limited, err := rootSvc.WatchFiles(watchCtx, &s4wave_unixfs.HandleWatchFilesRequest{
		Extensions: []string{".jpg", ".png"},
		Limit:      1,
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := limited.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(resp.GetAdded(), []string{"dest/deep/c.PNG"}) || !resp.GetTruncated() {
		t.Fatalf("unexpected limited response: %v", resp)
	}

	// The first response holds every match.
	watch, err := rootSvc.WatchFiles(watchCtx, &s4wave_unixfs.HandleWatchFilesRequest{
		Extensions: []string{".JPG", ".png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err = watch.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(resp.GetAdded(), []string{"dest/deep/c.PNG", "src/a.jpg"}) || resp.GetTruncated() {
		t.Fatalf("unexpected initial response: %v", resp)
	}

	// A new match arrives as an addition.
	if err := billy_util.WriteFile(bfs, "src/b.png", []byte("img"), 0o644); err != nil {
		t.Fatal(err)
	}
	resp, err = watch.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(resp.GetAdded(), []string{"src/b.png"}) || len(resp.GetRemoved()) != 0 {
		t.Fatalf("unexpected response after write: %v", resp)
	}

	// A removed match arrives as a removal.
	if err := bfs.Remove("src/a.jpg"); err != nil {
		t.Fatal(err)
	}
	resp, err = watch.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(resp.GetRemoved(), []string{"src/a.jpg"}) || len(resp.GetAdded()) != 0 {
		t.Fatalf("unexpected response after remove: %v", resp)
	}
}

func TestFSHandleResourceUploadTreeNested(t *testing.T) {
	// Start the filesystem resource testbed for a nested tree upload.
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Retain the filesystem root resource for the test.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()

	// Open the filesystem root RPC service.
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Start an upload stream for the nested filesystem tree.
	strm, err := rootSvc.UploadTree(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Upload an empty directory beneath the nested path.
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

	// Upload the nested child file and its contents.
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
			FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
				Path:      "nested/child.txt",
				TotalSize: 5,
				Mode:      0o644,
				Expect:    &s4wave_unixfs.HandleUploadTreeFileStart_Overwrite{Overwrite: true},
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

	// Upload the top-level file and its contents.
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
			FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
				Path:      "top.txt",
				TotalSize: 3,
				Mode:      0o644,
				Expect:    &s4wave_unixfs.HandleUploadTreeFileStart_Overwrite{Overwrite: true},
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

	// Commit the tree upload and verify its aggregate counters.
	resp, err := strm.CloseAndRecv()
	if err != nil {
		t.Fatal(err)
	}
	assertUploadTreeCounters(t, resp, 8, 2, 1)

	// Retain the uploaded nested child resource for readback.
	childResp, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "nested/child.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	childRef := resClient.CreateResourceReference(childResp.GetResourceId())
	defer childRef.Release()

	// Read the uploaded nested child through its RPC service.
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

	// Verify the nested child contains the uploaded bytes.
	if string(readResp.GetData()) != "hello" {
		t.Fatalf("got data %q, want %q", string(readResp.GetData()), "hello")
	}

	// Retain the uploaded top-level file resource for readback.
	topResp, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "top.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	topRef := resClient.CreateResourceReference(topResp.GetResourceId())
	defer topRef.Release()

	// Read the uploaded top-level file through its RPC service.
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

	// Verify the top-level file contains the uploaded bytes.
	if string(readResp.GetData()) != "top" {
		t.Fatalf("got data %q, want %q", string(readResp.GetData()), "top")
	}

	// Require the uploaded empty directory to be accessible.
	if _, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "nested/empty",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestFSHandleResourceUploadTreePublishesEachStream(t *testing.T) {
	// Start the filesystem resource testbed for separate upload publications.
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Retain the root resource and open its filesystem RPC service.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Stage two independent upload streams before committing either.
	first, err := rootSvc.UploadTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	second, err := rootSvc.UploadTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sendUploadTreeFile(t, first, "first.txt", []byte("first"))
	sendUploadTreeFile(t, second, "second.txt", []byte("second"))

	// Commit the first upload and verify only its file is published.
	firstResp, err := first.CloseAndRecv()
	if err != nil {
		t.Fatal(err)
	}
	assertUploadTreeCounters(t, firstResp, 5, 1, 0)
	if _, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "first.txt",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "second.txt",
	}); err == nil {
		t.Fatal("second file published before its upload stream committed")
	}

	// Commit the second upload and verify both files remain accessible.
	secondResp, err := second.CloseAndRecv()
	if err != nil {
		t.Fatal(err)
	}
	assertUploadTreeCounters(t, secondResp, 6, 1, 0)
	for _, path := range []string{"first.txt", "second.txt"} {
		if _, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
			Path: path,
		}); err != nil {
			t.Fatalf("lookup %q: %v", path, err)
		}
	}
}

func TestFSHandleResourceConcurrentUploadTreeCommitsPreserveSiblings(t *testing.T) {
	// Start the filesystem resource testbed for concurrent upload commits.
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Retain the root resource and open its filesystem RPC service.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Stage one upload stream for each sibling file.
	paths := []string{"first.txt", "second.txt", "third.txt"}
	streams := make([]s4wave_unixfs.SRPCFSHandleResourceService_UploadTreeClient, 0, len(paths))
	for _, path := range paths {
		strm, err := rootSvc.UploadTree(ctx)
		if err != nil {
			t.Fatal(err)
		}
		sendUploadTreeFile(t, strm, path, []byte(path))
		streams = append(streams, strm)
	}

	// Commit all staged upload streams concurrently and require success.
	start := make(chan struct{})
	results := make(chan error, len(streams))
	for _, strm := range streams {
		go func() {
			<-start
			_, err := strm.CloseAndRecv()
			results <- err
		}()
	}
	close(start)
	for range streams {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}

	// Require every sibling file to survive the concurrent commits.
	for _, path := range paths {
		if _, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
			Path: path,
		}); err != nil {
			t.Fatalf("lookup %q: %v", path, err)
		}
	}
}

// TestFSHandleResourceUploadTreePreservesConcurrentSiblingRemove races a batch
// tree upload against a per-op Remove of a disjoint sibling on the same world
// object. The two writers touch different names, so a correct serialization of
// the read-merge-publish yields one deterministic result regardless of commit
// order: the removed sibling stays gone and the uploaded file is present. The
// pre-fix lost update (each writer merges onto its own stale root snapshot and
// the last publisher wins) would either resurrect the removed sibling or drop
// the upload.
func TestFSHandleResourceUploadTreePreservesConcurrentSiblingRemove(t *testing.T) {
	// Start the filesystem resource testbed for concurrent upload and removal.
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Retain the root resource and open its filesystem RPC service.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Repeat the sibling upload and removal race with distinct file names.
	const iterations = 60
	for i := range iterations {
		// Choose disjoint sibling names for this upload and removal race.
		victim := fmt.Sprintf("victim-%d.txt", i)
		uploaded := fmt.Sprintf("report-%d.txt", i)

		// Create the sibling file that the concurrent removal will target.
		if _, err := rootSvc.Mknod(ctx, &s4wave_unixfs.HandleMknodRequest{
			Names:    []string{victim},
			NodeType: s4wave_unixfs.MknodType_MKNOD_TYPE_FILE,
			Mode:     0o644,
		}); err != nil {
			t.Fatalf("iteration %d: create victim: %v", i, err)
		}

		// Stage the disjoint file upload before starting the race.
		strm, err := rootSvc.UploadTree(ctx)
		if err != nil {
			t.Fatalf("iteration %d: open upload: %v", i, err)
		}
		sendUploadTreeFile(t, strm, uploaded, []byte(uploaded))

		// Race the staged upload commit against removal of the existing sibling.
		start := make(chan struct{})
		errs := make(chan error, 2)
		go func() {
			<-start
			_, err := strm.CloseAndRecv()
			errs <- err
		}()
		go func() {
			<-start
			_, err := rootSvc.Remove(ctx, &s4wave_unixfs.HandleRemoveRequest{
				Names: []string{victim},
			})
			errs <- err
		}()
		close(start)
		for range 2 {
			if err := <-errs; err != nil {
				t.Fatalf("iteration %d: concurrent op failed: %v", i, err)
			}
		}

		// Verify the removed sibling stays absent and the uploaded siblings remain.
		if _, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
			Path: victim,
		}); err == nil {
			t.Fatalf("iteration %d: removed sibling %q resurrected by concurrent upload", i, victim)
		}
		if _, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
			Path: uploaded,
		}); err != nil {
			t.Fatalf("iteration %d: uploaded file %q lost to concurrent remove: %v", i, uploaded, err)
		}
		if _, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
			Path: "src/file.txt",
		}); err != nil {
			t.Fatalf("iteration %d: baseline sibling lost: %v", i, err)
		}
	}
}

// TestFSHandleResourceUploadTreeDoesNotResurrectDeletedParent races a tree
// upload targeting a directory against a Remove of that same directory. The
// upload commits its file entries against the parent it targets, so once the
// parent is gone the commit must fail rather than merge onto a stale snapshot
// that still holds the parent. The invariant: whenever the Remove reports
// success, the parent directory is absent afterward. The pre-fix lost update
// let the upload resurrect the deleted directory even though Remove succeeded.
func TestFSHandleResourceUploadTreeDoesNotResurrectDeletedParent(t *testing.T) {
	// Start the filesystem resource testbed for upload and parent removal.
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Retain the root resource and open its filesystem RPC service.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Repeat the parent removal race with a fresh target directory.
	const iterations = 60
	for i := range iterations {
		// Create the directory that the concurrent removal will target.
		target := fmt.Sprintf("target-%d", i)
		if _, err := rootSvc.MkdirAll(ctx, &s4wave_unixfs.HandleMkdirAllRequest{
			PathParts: []string{target},
			Mode:      0o755,
		}); err != nil {
			t.Fatalf("iteration %d: create target dir: %v", i, err)
		}

		// Open the target directory resource and its filesystem RPC service.
		targetResp, err := rootSvc.Lookup(ctx, &s4wave_unixfs.HandleLookupRequest{Name: target})
		if err != nil {
			t.Fatalf("iteration %d: lookup target: %v", i, err)
		}
		targetRef := resClient.CreateResourceReference(targetResp.GetResourceId())
		targetClient, err := targetRef.GetClient()
		if err != nil {
			targetRef.Release()
			t.Fatalf("iteration %d: target client: %v", i, err)
		}
		targetSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(targetClient)

		// Stage a file upload into the target directory.
		strm, err := targetSvc.UploadTree(ctx)
		if err != nil {
			targetRef.Release()
			t.Fatalf("iteration %d: open upload: %v", i, err)
		}
		sendUploadTreeFile(t, strm, "up.txt", []byte("up"))

		// Race removal of the target directory against its staged upload commit.
		start := make(chan struct{})
		removeErr := make(chan error, 1)
		go func() {
			<-start
			_, err := rootSvc.Remove(ctx, &s4wave_unixfs.HandleRemoveRequest{
				Names: []string{target},
			})
			removeErr <- err
		}()
		close(start)
		// The upload may legitimately fail when the parent it targets is
		// removed before its commit; that is the delete winning the race.
		_, _ = strm.CloseAndRecv()
		rmErr := <-removeErr
		targetRef.Release()

		// Require a successfully removed parent directory to stay absent.
		if rmErr == nil {
			if _, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
				Path: target,
			}); err == nil {
				t.Fatalf("iteration %d: deleted parent %q resurrected by concurrent upload", i, target)
			}
		}
	}
}

// TestFSHandleResourceReadDuringUploadReloadRaceFree drives read RPCs on the
// same directory resource that a tree upload reloads its handle on, exercising
// the reloadHandle pointer swap against unsynchronized readers. Run under -race
// this fails if a read observes a torn handle pointer or uses a handle after
// its cursor is released.
func TestFSHandleResourceReadDuringUploadReloadRaceFree(t *testing.T) {
	// Start the filesystem resource testbed for reads during handle reloads.
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Retain the root resource and open its filesystem RPC service.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Continuously read the root resource while uploads replace its handle.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		for {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := rootSvc.GetFileInfo(ctx, &s4wave_unixfs.HandleGetFileInfoRequest{}); err != nil {
				select {
				case <-stop:
				default:
					t.Errorf("GetFileInfo during reload: %v", err)
				}
				return
			}
			if _, err := rootSvc.Lookup(ctx, &s4wave_unixfs.HandleLookupRequest{Name: "src"}); err != nil {
				select {
				case <-stop:
				default:
					t.Errorf("Lookup during reload: %v", err)
				}
				return
			}
		}
	})

	// Publish successive uploads while the root reader remains active.
	for i := range 25 {
		strm, err := rootSvc.UploadTree(ctx)
		if err != nil {
			close(stop)
			wg.Wait()
			t.Fatalf("open upload %d: %v", i, err)
		}
		sendUploadTreeFile(t, strm, fmt.Sprintf("reload-%d.txt", i), []byte("x"))
		if _, err := strm.CloseAndRecv(); err != nil {
			close(stop)
			wg.Wait()
			t.Fatalf("upload %d: %v", i, err)
		}
	}

	// Stop the root reader and wait for its completion.
	close(stop)
	wg.Wait()
}

func TestFSHandleResourceUploadTreeMetrics(t *testing.T) {
	// Start the filesystem resource testbed for upload metric recording.
	ctx, _, rootHandle, rootResource, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Upload a directory and file while recording every upload stage.
	recorder := &uploadMetricRecorder{}
	metricCtx := WithUploadMetricsRecorder(ctx, recorder)
	resp, err := rootResource.UploadTree(&uploadTreeMetricStream{
		ctx: metricCtx,
		messages: []*s4wave_unixfs.HandleUploadTreeRequest{
			{
				Body: &s4wave_unixfs.HandleUploadTreeRequest_Directory{
					Directory: &s4wave_unixfs.HandleUploadTreeDirectory{
						Path: "metric-dir",
						Mode: 0o755,
					},
				},
			},
			{
				Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
					FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
						Path:      "metric-dir/file.txt",
						TotalSize: 5,
						Mode:      0o644,
						Expect:    &s4wave_unixfs.HandleUploadTreeFileStart_Overwrite{Overwrite: true},
					},
				},
			},
			{
				Body: &s4wave_unixfs.HandleUploadTreeRequest_Data{
					Data: []byte("hello"),
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rootResource.GetHandle() != rootHandle {
		defer rootResource.GetHandle().Release()
	}
	assertUploadTreeCounters(t, resp, 5, 1, 1)

	// Verify the upload stages and their per-stage counts and byte totals.
	wantStages := []string{
		"receive-directory",
		"receive-file-start",
		"receive-data",
		"commit-start",
		"commit-complete",
		"reload-start",
		"reload-complete",
		"broadcast",
	}
	if !slices.Equal(recorder.stages(), wantStages) {
		t.Fatalf("stages = %v, want %v", recorder.stages(), wantStages)
	}
	for _, want := range []struct {
		stage string
		count int
		bytes int
	}{
		{stage: "receive-directory", count: 1},
		{stage: "receive-file-start", count: 1},
		{stage: "receive-data", count: 1, bytes: 5},
		{stage: "commit-start", count: 1},
		{stage: "commit-complete", count: 1},
		{stage: "reload-start", count: 1},
		{stage: "reload-complete", count: 1},
		{stage: "broadcast", count: 1},
	} {
		// Verify the expected event count and byte total for this upload stage.
		if got := recorder.countStage(want.stage); got != want.count {
			t.Fatalf("%s metric count = %d, want %d", want.stage, got, want.count)
		}
		if got := recorder.totalBytes(want.stage); got != want.bytes {
			t.Fatalf("%s metric bytes = %d, want %d", want.stage, got, want.bytes)
		}
	}

	// Read the uploaded file directly from the published filesystem root.
	bfs := unixfs_billy.NewBillyFS(ctx, rootResource.GetHandle(), "", time.Now())
	data, err := billy_util.ReadFile(bfs, "metric-dir/file.txt")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the uploaded bytes and report the recorded stage measurements.
	if string(data) != "hello" {
		t.Fatalf("got data %q, want %q", string(data), "hello")
	}
	for _, metric := range recorder.metrics {
		t.Logf(
			"metric workload=resource-upload-tree file_class=resource-unixfs stage=%s bytes_written=%d files_written=%d directories_written=%d",
			metric.Stage,
			metric.Bytes,
			resp.GetWritten().GetFilesWritten(),
			resp.GetWritten().GetDirectoriesWritten(),
		)
	}
}

func TestFSHandleResourceUploadTreeMetricsAbortCleanup(t *testing.T) {
	// Start the filesystem resource testbed for failed upload metrics.
	ctx, _, _, rootResource, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Upload an invalid absolute path while recording its cleanup stages.
	recorder := &uploadMetricRecorder{}
	metricCtx := WithUploadMetricsRecorder(ctx, recorder)
	_, err := rootResource.UploadTree(&uploadTreeMetricStream{
		ctx: metricCtx,
		messages: []*s4wave_unixfs.HandleUploadTreeRequest{
			{
				Body: &s4wave_unixfs.HandleUploadTreeRequest_Directory{
					Directory: &s4wave_unixfs.HandleUploadTreeDirectory{
						Path: "/metric-dir",
						Mode: 0o755,
					},
				},
			},
		},
	})

	// Verify the upload fails and records its abort cleanup.
	if err == nil {
		t.Fatal("expected absolute upload path to fail")
	}
	if !slices.Equal(recorder.stages(), []string{"receive-directory", "abort-cleanup"}) {
		t.Fatalf("stages = %v, want [receive-directory abort-cleanup]", recorder.stages())
	}
}

func TestFSHandleResourceUploadTreeOverwriteReadback(t *testing.T) {
	// Start the filesystem resource testbed for overwrite readback.
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Retain the filesystem root resource for the test.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()

	// Open the filesystem root RPC service.
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Upload two contents to the same file and verify each commit counter.
	first := uploadTestPatternBytes(96 * 1024)
	second := uploadTestPatternBytes(160 * 1024)
	firstResp := uploadTreeFileViaResource(t, ctx, rootSvc, "overwrite.bin", first)
	assertUploadTreeCounters(t, firstResp, int64(len(first)), 1, 0)
	secondResp := uploadTreeFileViaResource(t, ctx, rootSvc, "overwrite.bin", second)
	assertUploadTreeCounters(t, secondResp, int64(len(second)), 1, 0)

	// Retain the overwritten file resource for readback.
	fileResp, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "overwrite.bin",
	})
	if err != nil {
		t.Fatal(err)
	}
	fileRef := resClient.CreateResourceReference(fileResp.GetResourceId())
	defer fileRef.Release()

	// Read every bounded response from the overwritten file through RPC.
	fileClient, err := fileRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	fileSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(fileClient)
	for offset := 0; offset < len(second); offset += fsHandleMaxReadSize {
		// Read the next bounded range of the overwritten file.
		end := min(offset+fsHandleMaxReadSize, len(second))
		readResp, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{
			Offset: int64(offset),
			Length: int64(len(second)),
		})
		if err != nil {
			t.Fatal(err)
		}

		// Verify the range bytes and EOF against the replacement contents.
		if !slices.Equal(readResp.GetData(), second[offset:end]) {
			t.Fatalf("overwritten file data mismatch at offset %d", offset)
		}
		if got, want := readResp.GetEof(), end == len(second); got != want {
			t.Fatalf("read eof at offset %d = %v, want %v", offset, got, want)
		}
	}
}

func TestFSHandleResourceReadAtCapsLargeResponse(t *testing.T) {
	// Start the filesystem resource testbed for bounded read responses.
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Retain the filesystem root resource for the test.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()

	// Open the filesystem root RPC service.
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Prepare file contents larger than one bounded read response.
	data := make([]byte, fsHandleMaxReadSize+32*1024)
	for i := range data {
		data[i] = byte(i % 251)
	}

	// Upload the large file through bounded data frames and verify its counters.
	strm, err := rootSvc.UploadTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
			FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
				Path:      "large.bin",
				TotalSize: int64(len(data)),
				Mode:      0o644,
				Expect:    &s4wave_unixfs.HandleUploadTreeFileStart_Overwrite{Overwrite: true},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	for offset := 0; offset < len(data); offset += 32 * 1024 {
		end := min(offset+32*1024, len(data))
		if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
			Body: &s4wave_unixfs.HandleUploadTreeRequest_Data{
				Data: data[offset:end],
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	uploadResp, err := strm.CloseAndRecv()
	if err != nil {
		t.Fatal(err)
	}
	assertUploadTreeCounters(t, uploadResp, int64(len(data)), 1, 0)

	// Retain the large file resource for readback.
	fileResp, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "large.bin",
	})
	if err != nil {
		t.Fatal(err)
	}
	fileRef := resClient.CreateResourceReference(fileResp.GetResourceId())
	defer fileRef.Release()

	// Open the large file RPC service.
	fileClient, err := fileRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	fileSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(fileClient)

	// Require an unbounded read request for the large file to fail.
	if _, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{}); err == nil {
		t.Fatal("expected large length=0 read to fail instead of returning a capped partial response")
	}

	// Read the large file with an explicit length through RPC.
	first, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{
		Length: int64(len(data)),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the first response contains one full chunk without EOF.
	if first.GetBytesRead() != fsHandleMaxReadSize {
		t.Fatalf("first bytes_read = %d, want %d", first.GetBytesRead(), fsHandleMaxReadSize)
	}
	if len(first.GetData()) != fsHandleMaxReadSize {
		t.Fatalf("first data len = %d, want %d", len(first.GetData()), fsHandleMaxReadSize)
	}
	if !slices.Equal(first.GetData(), data[:fsHandleMaxReadSize]) {
		t.Fatal("first read data mismatch")
	}
	if first.GetEof() {
		t.Fatal("first capped read unexpectedly reported EOF")
	}

	// Read the remaining file range through RPC.
	second, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{
		Offset: fsHandleMaxReadSize,
		Length: int64(len(data)),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the tail bytes, byte count, and EOF in the final response.
	wantTail := data[fsHandleMaxReadSize:]
	if second.GetBytesRead() != int64(len(wantTail)) {
		t.Fatalf("second bytes_read = %d, want %d", second.GetBytesRead(), len(wantTail))
	}
	if !slices.Equal(second.GetData(), wantTail) {
		t.Fatal("second read data mismatch")
	}
	if !second.GetEof() {
		t.Fatal("second read did not report EOF")
	}
}

func TestFSHandleResourceReadStream(t *testing.T) {
	// Start the filesystem resource testbed for streamed read responses.
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Retain the root resource and open its filesystem RPC service.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Upload file contents spanning several bounded stream frames.
	data := uploadTestPatternBytes(3*fsHandleMaxReadSize + 1000)
	uploadTreeFileViaResource(t, ctx, rootSvc, "stream.bin", data)

	// Retain the uploaded file resource and open its RPC service.
	fileResp, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{Path: "stream.bin"})
	if err != nil {
		t.Fatal(err)
	}
	fileRef := resClient.CreateResourceReference(fileResp.GetResourceId())
	defer fileRef.Release()
	fileClient, err := fileRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	fileSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(fileClient)

	// readStream collects one stream, checking every frame fits a response.
	readStream := func(offset, length int64) []byte {
		// Open a file read stream for the requested range.
		t.Helper()
		strm, err := fileSvc.ReadStream(ctx, &s4wave_unixfs.HandleReadStreamRequest{
			Offset: offset,
			Length: length,
		})
		if err != nil {
			t.Fatal(err)
		}
		defer strm.Close()

		// Collect the file stream and require each frame to fit a bounded response.
		var out []byte
		for {
			frame, err := strm.Recv()
			if err == io.EOF {
				return out
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(frame.GetData()) > fsHandleMaxReadSize {
				t.Fatalf("frame of %d bytes exceeds %d", len(frame.GetData()), fsHandleMaxReadSize)
			}
			out = append(out, frame.GetData()...)
		}
	}

	// Verify full-file, bounded-range, and past-end read streams.
	if got := readStream(0, 0); !slices.Equal(got, data) {
		t.Fatalf("whole file: read %d bytes, want %d matching", len(got), len(data))
	}
	if got := readStream(1000, 2*fsHandleMaxReadSize); !slices.Equal(got, data[1000:1000+2*fsHandleMaxReadSize]) {
		t.Fatalf("range: read %d bytes, want %d matching", len(got), 2*fsHandleMaxReadSize)
	}
	if got := readStream(int64(len(data))-10, 100); !slices.Equal(got, data[len(data)-10:]) {
		t.Fatalf("past end: read %d bytes, want the last 10", len(got))
	}
}

func uploadTreeFileViaResource(
	t *testing.T,
	ctx context.Context,
	rootSvc s4wave_unixfs.SRPCFSHandleResourceServiceClient,
	name string,
	data []byte,
) *s4wave_unixfs.HandleUploadTreeResponse {
	t.Helper()
	return uploadTreeFileExpect(t, ctx, rootSvc, name, data, &s4wave_unixfs.HandleUploadTreeFileStart{
		Expect: &s4wave_unixfs.HandleUploadTreeFileStart_Overwrite{Overwrite: true},
	})
}

// uploadTreeFileExpect uploads one file with the expectation start carries;
// it fills the path, size and mode. An unset expectation asks for the path
// to hold nothing.
func uploadTreeFileExpect(
	t *testing.T,
	ctx context.Context,
	rootSvc s4wave_unixfs.SRPCFSHandleResourceServiceClient,
	name string,
	data []byte,
	start *s4wave_unixfs.HandleUploadTreeFileStart,
) *s4wave_unixfs.HandleUploadTreeResponse {
	// Open an upload stream and send the file metadata.
	t.Helper()
	start.Path, start.TotalSize = name, int64(len(data))
	strm, err := rootSvc.UploadTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{FileStart: start},
	}); err != nil {
		t.Fatal(err)
	}

	// Send the file contents in bounded upload data frames.
	for offset := 0; offset < len(data); offset += uploadDataFrameMaxBytes {
		end := min(offset+uploadDataFrameMaxBytes, len(data))
		if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
			Body: &s4wave_unixfs.HandleUploadTreeRequest_Data{
				Data: data[offset:end],
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Commit the file upload and return its counters.
	resp, err := strm.CloseAndRecv()
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func uploadTestPatternBytes(size int) []byte {
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = byte((i * 31) ^ (i >> 7) ^ (i >> 15))
	}
	return buf
}

func TestFSHandleResourceUploadTreeRejectsAbsolutePath(t *testing.T) {
	// Start the filesystem resource testbed for absolute path rejection.
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Retain the filesystem root resource for the test.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()

	// Open the filesystem root RPC service.
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Upload an absolute directory path and require the commit to fail.
	strm, err := rootSvc.UploadTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_Directory{
			Directory: &s4wave_unixfs.HandleUploadTreeDirectory{
				Path: "/nested",
				Mode: 0o755,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := strm.CloseAndRecv(); err == nil {
		t.Fatal("expected absolute upload path to fail")
	}
}

func TestFSHandleResourceUploadTreeRejectsOversizedData(t *testing.T) {
	// Start the filesystem resource testbed for oversized upload frames.
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Retain the filesystem root resource for the test.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()

	// Open the filesystem root RPC service.
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Open a tree upload stream and send the declared file size.
	strm, err := rootSvc.UploadTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
			FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
				Path:      "oversized.txt",
				TotalSize: uploadDataFrameMaxBytes + 1,
				Mode:      0o644,
				Expect:    &s4wave_unixfs.HandleUploadTreeFileStart_Overwrite{Overwrite: true},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}

	// Send an oversized tree upload frame and require the commit to fail.
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_Data{
			Data: make([]byte, uploadDataFrameMaxBytes+1),
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := strm.CloseAndRecv(); err == nil {
		t.Fatal("expected oversized upload data to fail")
	}
}

func TestFSHandleResourceUploadFileRejectsOversizedData(t *testing.T) {
	// Start the filesystem resource testbed for oversized single-file uploads.
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Retain the filesystem root resource for the test.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()

	// Open the filesystem root RPC service.
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Send an oversized single-file upload and require the commit to fail.
	strm, err := rootSvc.UploadFile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := strm.Send(&s4wave_unixfs.HandleUploadFileRequest{
		Name:      "oversized.txt",
		TotalSize: uploadDataFrameMaxBytes + 1,
		Mode:      0o644,
		Data:      make([]byte, uploadDataFrameMaxBytes+1),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := strm.CloseAndRecv(); err == nil {
		t.Fatal("expected oversized upload data to fail")
	}
}

func TestFSHandleResourceUploadFileRejectsDataPastTotalSize(t *testing.T) {
	// Start the filesystem resource testbed for single-file uploads.
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	// Retain the filesystem root resource for the test.
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()

	// Open the filesystem root RPC service.
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// Send more bytes than the declared size and require an error, not a hang.
	strm, err := rootSvc.UploadFile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := strm.Send(&s4wave_unixfs.HandleUploadFileRequest{
		Name:      "short.txt",
		TotalSize: 4,
		Mode:      0o644,
		Data:      []byte("four"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := strm.Send(&s4wave_unixfs.HandleUploadFileRequest{
		Data: []byte("extra"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := strm.CloseAndRecv(); err == nil {
		t.Fatal("expected upload data past total_size to fail")
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

// openFileService looks up path from the root and opens a new file resource.
func openFileService(
	t *testing.T,
	ctx context.Context,
	resClient *resource_client.Client,
	rootSvc s4wave_unixfs.SRPCFSHandleResourceServiceClient,
	path string,
) (s4wave_unixfs.SRPCFSHandleResourceServiceClient, func()) {
	t.Helper()
	resp, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	ref := resClient.CreateResourceReference(resp.GetResourceId())
	client, err := ref.GetClient()
	if err != nil {
		ref.Release()
		t.Fatal(err)
	}
	return s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(client), ref.Release
}

// readFileVersion reads a whole file through ReadStream and returns its
// contents with the version the first frame carried.
func readFileVersion(
	t *testing.T,
	ctx context.Context,
	fileSvc s4wave_unixfs.SRPCFSHandleResourceServiceClient,
) ([]byte, []byte) {
	t.Helper()
	strm, err := fileSvc.ReadStream(ctx, &s4wave_unixfs.HandleReadStreamRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer strm.Close()
	var data, version []byte
	for first := true; ; first = false {
		frame, err := strm.Recv()
		if err == io.EOF {
			return data, version
		}
		if err != nil {
			t.Fatal(err)
		}
		if first {
			version = frame.GetVersion()
		} else if len(frame.GetVersion()) != 0 {
			t.Fatal("version set after the first read frame")
		}
		data = append(data, frame.GetData()...)
	}
}

func TestFSHandleResourceUploadTreeRefusesStaleVersion(t *testing.T) {
	// Start the filesystem resource testbed with one saved document.
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)
	uploadTreeFileViaResource(t, ctx, rootSvc, "doc.txt", []byte("base text"))

	// The first writer reads the document and the version it was read at.
	readerSvc, releaseReader := openFileService(t, ctx, resClient, rootSvc, "doc.txt")
	defer releaseReader()
	data, readVersion := readFileVersion(t, ctx, readerSvc)
	if string(data) != "base text" || len(readVersion) == 0 {
		t.Fatalf("first read = %q with %d version bytes", data, len(readVersion))
	}

	// A second writer changes the document through its own file resource.
	writerSvc, releaseWriter := openFileService(t, ctx, resClient, rootSvc, "doc.txt")
	defer releaseWriter()
	if _, err := writerSvc.WriteAt(ctx, &s4wave_unixfs.HandleWriteAtRequest{
		Data: []byte("SECOND"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := writerSvc.Sync(ctx, &s4wave_unixfs.HandleSyncRequest{}); err != nil {
		t.Fatal(err)
	}

	// The first writer's save at the stale version must be refused.
	resp := uploadTreeFileExpect(t, ctx, rootSvc, "doc.txt", []byte("first draft"),
		&s4wave_unixfs.HandleUploadTreeFileStart{
			Expect: &s4wave_unixfs.HandleUploadTreeFileStart_Version{Version: readVersion},
		})
	conflict := resp.GetConflict()
	if conflict == nil {
		t.Fatalf("stale save result = %v, want conflict", resp.GetResult())
	}
	if conflict.GetPath() != "doc.txt" {
		t.Fatalf("conflict path = %q", conflict.GetPath())
	}

	// The second writer's content survives and its version is the one reported.
	checkSvc, releaseCheck := openFileService(t, ctx, resClient, rootSvc, "doc.txt")
	data, currentVersion := readFileVersion(t, ctx, checkSvc)
	releaseCheck()
	if string(data) != "SECONDext" {
		t.Fatalf("after refused save = %q, want the second writer's content", data)
	}
	if !slices.Equal(conflict.GetVersion(), currentVersion) || slices.Equal(currentVersion, readVersion) {
		t.Fatal("conflict version does not match the current file version")
	}

	// A save at the current version succeeds and reports the version it
	// wrote, which a later read returns.
	resp = uploadTreeFileExpect(t, ctx, rootSvc, "doc.txt", []byte("merged text"),
		&s4wave_unixfs.HandleUploadTreeFileStart{
			Expect:      &s4wave_unixfs.HandleUploadTreeFileStart_Version{Version: currentVersion},
			WantVersion: true,
		})
	assertUploadTreeCounters(t, resp, int64(len("merged text")), 1, 0)
	written := resp.GetWritten().GetVersions()
	if len(written) != 1 || written[0].GetPath() != "doc.txt" || len(written[0].GetVersion()) == 0 {
		t.Fatalf("written versions = %v, want one for doc.txt", written)
	}
	savedVersion := written[0].GetVersion()
	checkSvc, releaseCheck = openFileService(t, ctx, resClient, rootSvc, "doc.txt")
	data, readBack := readFileVersion(t, ctx, checkSvc)
	releaseCheck()
	if string(data) != "merged text" {
		t.Fatalf("after current-version save = %q", data)
	}
	if !slices.Equal(readBack, savedVersion) {
		t.Fatal("the written version differs from the version a later read returns")
	}

	// The written version is accepted as the next save's expectation, with
	// no read between.
	resp = uploadTreeFileExpect(t, ctx, rootSvc, "doc.txt", []byte("next text"),
		&s4wave_unixfs.HandleUploadTreeFileStart{
			Expect: &s4wave_unixfs.HandleUploadTreeFileStart_Version{Version: savedVersion},
		})
	assertUploadTreeCounters(t, resp, int64(len("next text")), 1, 0)
	if len(resp.GetWritten().GetVersions()) != 0 {
		t.Fatal("a save that asked for no version reported one")
	}
}

func TestFSHandleResourceUploadTreeMovesOpenHandles(t *testing.T) {
	// Save a document and open a file resource on it before any write.
	ctx, resClient, _, rootRes, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)
	uploadTreeFileViaResource(t, ctx, rootSvc, "doc.txt", []byte("base text"))
	readerSvc, releaseReader := openFileService(t, ctx, resClient, rootSvc, "doc.txt")
	defer releaseReader()
	_, readVersion := readFileVersion(t, ctx, readerSvc)

	// Another writer changes the document through its own World cursor,
	// which the resource tree learns of only through its watch.
	otherCursor, err := unixfs_world.FollowUnixfsRef(
		ctx,
		logrus.NewEntry(logrus.New()),
		rootRes.ws,
		&unixfs_world.UnixfsRef{ObjectKey: "test-fs"},
		"",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer otherCursor.Release()
	otherRoot, err := unixfs_sdk.NewFSHandle(otherCursor)
	if err != nil {
		t.Fatal(err)
	}
	defer otherRoot.Release()
	otherDoc, err := otherRoot.Lookup(ctx, "doc.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := otherDoc.WriteAt(ctx, 0, []byte("OTHER"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := otherDoc.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	otherDoc.Release()

	// A save at the version read first is refused, and a read through the
	// file resource opened before it returns the other writer's bytes.
	resp := uploadTreeFileExpect(t, ctx, rootSvc, "doc.txt", []byte("first draft"),
		&s4wave_unixfs.HandleUploadTreeFileStart{
			Expect: &s4wave_unixfs.HandleUploadTreeFileStart_Version{Version: readVersion},
		})
	conflict := resp.GetConflict()
	if conflict == nil {
		t.Fatalf("stale save result = %v, want conflict", resp.GetResult())
	}
	data, currentVersion := readFileVersion(t, ctx, readerSvc)
	if string(data) != "OTHERtext" {
		t.Fatalf("read after refused save = %q, want the other writer's bytes", data)
	}
	if !slices.Equal(currentVersion, conflict.GetVersion()) {
		t.Fatal("read version differs from the version the refusal reported")
	}

	// A save at the current version is read back through the same resource.
	resp = uploadTreeFileExpect(t, ctx, rootSvc, "doc.txt", []byte("merged text"),
		&s4wave_unixfs.HandleUploadTreeFileStart{
			Expect: &s4wave_unixfs.HandleUploadTreeFileStart_Version{Version: currentVersion},
		})
	assertUploadTreeCounters(t, resp, int64(len("merged text")), 1, 0)
	if data, _ := readFileVersion(t, ctx, readerSvc); string(data) != "merged text" {
		t.Fatalf("read after save = %q, want the saved bytes", data)
	}
}

func TestFSHandleResourceUploadTreeMissingRefusesExisting(t *testing.T) {
	// Start the filesystem resource testbed for create-only uploads.
	ctx, resClient, _, rootRes, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()
	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	// A create-only upload of a new path writes the file.
	resp := uploadTreeFileExpect(t, ctx, rootSvc, "new.txt", []byte("new"), &s4wave_unixfs.HandleUploadTreeFileStart{})
	assertUploadTreeCounters(t, resp, 3, 1, 0)

	// A create-only upload over an existing file is refused and keeps it.
	resp = uploadTreeFileExpect(t, ctx, rootSvc, "src/file.txt", []byte("clobber"), &s4wave_unixfs.HandleUploadTreeFileStart{})
	if conflict := resp.GetConflict(); conflict == nil || len(conflict.GetVersion()) == 0 {
		t.Fatalf("create over existing file result = %v, want conflict with version", resp.GetResult())
	}
	checkSvc, releaseCheck := openFileService(t, ctx, resClient, rootSvc, "src/file.txt")
	data, _ := readFileVersion(t, ctx, checkSvc)
	releaseCheck()
	if string(data) != "hello" {
		t.Fatalf("existing file = %q after refused create", data)
	}

	// Another writer changes the file's mode.
	if err := rootRes.mutate(func() error {
		file, _, err := rootRes.handle.LookupPath(ctx, "src/file.txt")
		if err != nil {
			return err
		}
		defer file.Release()
		return file.SetPermissions(ctx, 0o600, time.Now())
	}); err != nil {
		t.Fatal(err)
	}

	// A save at the file's version with no mode keeps the changed mode.
	checkSvc, releaseCheck = openFileService(t, ctx, resClient, rootSvc, "src/file.txt")
	_, version := readFileVersion(t, ctx, checkSvc)
	releaseCheck()
	resp = uploadTreeFileExpect(t, ctx, rootSvc, "src/file.txt", []byte("saved"),
		&s4wave_unixfs.HandleUploadTreeFileStart{
			Expect: &s4wave_unixfs.HandleUploadTreeFileStart_Version{Version: version},
		})
	assertUploadTreeCounters(t, resp, int64(len("saved")), 1, 0)
	checkSvc, releaseCheck = openFileService(t, ctx, resClient, rootSvc, "src/file.txt")
	defer releaseCheck()
	info, err := checkSvc.GetFileInfo(ctx, &s4wave_unixfs.HandleGetFileInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if perm := fs.FileMode(info.GetInfo().GetMode()).Perm(); perm != 0o600 {
		t.Fatalf("mode after a save without one = %v, want the other writer's 0600", perm)
	}

	// A create-only directory over an existing one is refused, and nothing
	// under it is written into the existing directory.
	resp = uploadTreeDirFile(t, ctx, rootSvc, "src", true, "src/dropped.txt", []byte("dropped"))
	if conflict := resp.GetConflict(); conflict.GetPath() != "src" || len(conflict.GetVersion()) != 0 {
		t.Fatalf("create over existing directory result = %v, want conflict at src", resp.GetResult())
	}
	if _, _, err := rootRes.handle.LookupPath(ctx, "src/dropped.txt"); !errors.Is(err, unixfs_errors.ErrNotExist) {
		t.Fatalf("lookup of a file under a refused directory: %v, want not exist", err)
	}

	// A create-only directory at a new path is written with its contents.
	resp = uploadTreeDirFile(t, ctx, rootSvc, "src (2)", true, "src (2)/dropped.txt", []byte("dropped"))
	assertUploadTreeCounters(t, resp, int64(len("dropped")), 1, 1)

	// A directory with no expectation merges into the existing one.
	resp = uploadTreeDirFile(t, ctx, rootSvc, "src", false, "src/merged.txt", []byte("merged"))
	assertUploadTreeCounters(t, resp, int64(len("merged")), 1, 1)
	checkSvc, releaseMerged := openFileService(t, ctx, resClient, rootSvc, "src/merged.txt")
	defer releaseMerged()
	if data, _ := readFileVersion(t, ctx, checkSvc); string(data) != "merged" {
		t.Fatalf("merged file = %q", data)
	}
}

// uploadTreeDirFile sends one directory, create-only when expectMissing is
// set, and one overwriting file in a single UploadTree and returns the result.
func uploadTreeDirFile(
	t *testing.T,
	ctx context.Context,
	rootSvc s4wave_unixfs.SRPCFSHandleResourceServiceClient,
	dir string,
	expectMissing bool,
	name string,
	data []byte,
) *s4wave_unixfs.HandleUploadTreeResponse {
	t.Helper()
	strm, err := rootSvc.UploadTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	msgs := []*s4wave_unixfs.HandleUploadTreeRequest{
		{Body: &s4wave_unixfs.HandleUploadTreeRequest_Directory{
			Directory: &s4wave_unixfs.HandleUploadTreeDirectory{Path: dir, ExpectMissing: expectMissing},
		}},
		{Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
			FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
				Path:      name,
				TotalSize: int64(len(data)),
				Expect:    &s4wave_unixfs.HandleUploadTreeFileStart_Overwrite{Overwrite: true},
			},
		}},
		{Body: &s4wave_unixfs.HandleUploadTreeRequest_Data{Data: data}},
	}
	for _, msg := range msgs {
		if err := strm.Send(msg); err != nil {
			t.Fatal(err)
		}
	}
	resp, err := strm.CloseAndRecv()
	if err != nil {
		t.Fatal(err)
	}
	return resp
}
