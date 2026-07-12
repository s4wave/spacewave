package resource_unixfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
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
	if resp.GetBytesWritten() != bytesWritten {
		t.Fatalf("bytes_written = %d, want %d", resp.GetBytesWritten(), bytesWritten)
	}
	if resp.GetFilesWritten() != filesWritten {
		t.Fatalf("files_written = %d, want %d", resp.GetFilesWritten(), filesWritten)
	}
	if resp.GetDirectoriesWritten() != directoriesWritten {
		t.Fatalf("directories_written = %d, want %d", resp.GetDirectoriesWritten(), directoriesWritten)
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

	rootResource := NewFSHandleObjectResource(
		rootHandle,
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

	return ctx, resClient, rootHandle, rootResource, cleanup
}

func TestFSHandleResourceRenameCrossDirectory(t *testing.T) {
	ctx, resClient, rootHandle, _, cleanup := setupFSHandleResourceClient(t)
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
	ctx, resClient, _, rootResource, cleanup := setupFSHandleResourceClient(t)
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

func TestFSHandleResourceUploadTreeNested(t *testing.T) {
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
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
	assertUploadTreeCounters(t, resp, 8, 2, 1)

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

func TestFSHandleResourceUploadTreePublishesEachStream(t *testing.T) {
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

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
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

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
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	const iterations = 60
	for i := range iterations {
		victim := fmt.Sprintf("victim-%d.txt", i)
		uploaded := fmt.Sprintf("report-%d.txt", i)

		if _, err := rootSvc.Mknod(ctx, &s4wave_unixfs.HandleMknodRequest{
			Names:    []string{victim},
			NodeType: s4wave_unixfs.MknodType_MKNOD_TYPE_FILE,
			Mode:     0o644,
		}); err != nil {
			t.Fatalf("iteration %d: create victim: %v", i, err)
		}

		strm, err := rootSvc.UploadTree(ctx)
		if err != nil {
			t.Fatalf("iteration %d: open upload: %v", i, err)
		}
		sendUploadTreeFile(t, strm, uploaded, []byte(uploaded))

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
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	const iterations = 60
	for i := range iterations {
		target := fmt.Sprintf("target-%d", i)

		if _, err := rootSvc.MkdirAll(ctx, &s4wave_unixfs.HandleMkdirAllRequest{
			PathParts: []string{target},
			Mode:      0o755,
		}); err != nil {
			t.Fatalf("iteration %d: create target dir: %v", i, err)
		}

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

		strm, err := targetSvc.UploadTree(ctx)
		if err != nil {
			targetRef.Release()
			t.Fatalf("iteration %d: open upload: %v", i, err)
		}
		sendUploadTreeFile(t, strm, "up.txt", []byte("up"))

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
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

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
	close(stop)
	wg.Wait()
}

func TestFSHandleResourceUploadTreeMetrics(t *testing.T) {
	ctx, _, rootHandle, rootResource, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

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
		if got := recorder.countStage(want.stage); got != want.count {
			t.Fatalf("%s metric count = %d, want %d", want.stage, got, want.count)
		}
		if got := recorder.totalBytes(want.stage); got != want.bytes {
			t.Fatalf("%s metric bytes = %d, want %d", want.stage, got, want.bytes)
		}
	}

	bfs := unixfs_billy.NewBillyFS(ctx, rootResource.GetHandle(), "", time.Now())
	data, err := billy_util.ReadFile(bfs, "metric-dir/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("got data %q, want %q", string(data), "hello")
	}
	for _, metric := range recorder.metrics {
		t.Logf(
			"metric workload=resource-upload-tree file_class=resource-unixfs stage=%s bytes_written=%d files_written=%d directories_written=%d",
			metric.Stage,
			metric.Bytes,
			resp.GetFilesWritten(),
			resp.GetDirectoriesWritten(),
		)
	}
}

func TestFSHandleResourceUploadTreeMetricsAbortCleanup(t *testing.T) {
	ctx, _, _, rootResource, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

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
	if err == nil {
		t.Fatal("expected absolute upload path to fail")
	}
	if !slices.Equal(recorder.stages(), []string{"receive-directory", "abort-cleanup"}) {
		t.Fatalf("stages = %v, want [receive-directory abort-cleanup]", recorder.stages())
	}
}

func TestFSHandleResourceUploadTreeOverwriteReadback(t *testing.T) {
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()

	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	first := uploadTestPatternBytes(96 * 1024)
	second := uploadTestPatternBytes(160 * 1024)
	firstResp := uploadTreeFileViaResource(t, ctx, rootSvc, "overwrite.bin", first)
	assertUploadTreeCounters(t, firstResp, int64(len(first)), 1, 0)
	secondResp := uploadTreeFileViaResource(t, ctx, rootSvc, "overwrite.bin", second)
	assertUploadTreeCounters(t, secondResp, int64(len(second)), 1, 0)

	fileResp, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "overwrite.bin",
	})
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
	for offset := 0; offset < len(second); offset += fsHandleMaxReadSize {
		end := min(offset+fsHandleMaxReadSize, len(second))
		readResp, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{
			Offset: int64(offset),
			Length: int64(len(second)),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(readResp.GetData(), second[offset:end]) {
			t.Fatalf("overwritten file data mismatch at offset %d", offset)
		}
		if got, want := readResp.GetEof(), end == len(second); got != want {
			t.Fatalf("read eof at offset %d = %v, want %v", offset, got, want)
		}
	}
}

func TestFSHandleResourceReadAtCapsLargeResponse(t *testing.T) {
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()

	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

	data := make([]byte, fsHandleMaxReadSize+32*1024)
	for i := range data {
		data[i] = byte(i % 251)
	}

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

	fileResp, err := rootSvc.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{
		Path: "large.bin",
	})
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

	if _, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{}); err == nil {
		t.Fatal("expected large length=0 read to fail instead of returning a capped partial response")
	}

	first, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{
		Length: int64(len(data)),
	})
	if err != nil {
		t.Fatal(err)
	}
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

	second, err := fileSvc.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{
		Offset: fsHandleMaxReadSize,
		Length: int64(len(data)),
	})
	if err != nil {
		t.Fatal(err)
	}
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

func uploadTreeFileViaResource(
	t *testing.T,
	ctx context.Context,
	rootSvc s4wave_unixfs.SRPCFSHandleResourceServiceClient,
	name string,
	data []byte,
) *s4wave_unixfs.HandleUploadTreeResponse {
	t.Helper()

	strm, err := rootSvc.UploadTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{
		Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
			FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
				Path:      name,
				TotalSize: int64(len(data)),
				Mode:      0o644,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
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
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
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
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
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
		Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{
			FileStart: &s4wave_unixfs.HandleUploadTreeFileStart{
				Path:      "oversized.txt",
				TotalSize: uploadDataFrameMaxBytes + 1,
				Mode:      0o644,
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
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
	ctx, resClient, _, _, cleanup := setupFSHandleResourceClient(t)
	defer cleanup()

	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()

	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	rootSvc := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)

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

func extractEntryNames(entries []*s4wave_unixfs.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.GetName())
	}
	slices.Sort(names)
	return names
}
