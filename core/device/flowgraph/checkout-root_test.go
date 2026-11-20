//go:build !js

package device_flowgraph

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_rpc "github.com/s4wave/spacewave/db/unixfs/rpc"
	unixfs_rpc_client "github.com/s4wave/spacewave/db/unixfs/rpc/client"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_unixfs "github.com/s4wave/spacewave/sdk/unixfs"
	"github.com/sirupsen/logrus"
)

// checkCheckoutRootAccess reads and attempts a fully specified write to a host file.
func checkCheckoutRootAccess(t *testing.T, ctx context.Context, device peer.ID, viewer *transport.SessionTransport, id protocol.ID, dir string, writable bool) {
	// Open the advertised stream as a known member of the Device's account.
	t.Helper()
	ms := openFrameControl(t, ctx, device, viewer, id)
	mc, err := srpc.NewMuxedConnWithRwc(ctx, ms.GetStream(), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mc.Close()
	client := unixfs_rpc.NewSRPCFSCursorServiceClient(srpc.NewClientWithMuxedConn(mc))

	// Read the cursor client and obtain handles for the real root and file.
	events, err := client.FSCursorClient(ctx, &unixfs_rpc.FSCursorClientRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer events.Close()
	opened, err := events.Recv()
	if err != nil || opened.GetInit() == nil {
		t.Fatalf("checkout root opened with %v, error %v", opened, err)
	}
	init := opened.GetInit()
	root, err := client.GetCursorOps(ctx, &unixfs_rpc.GetCursorOpsRequest{CursorHandleId: init.GetCursorHandleId()})
	if err != nil || root.GetUnixfsError() != nil {
		t.Fatalf("root operations = %v, error %v", root, err)
	}
	entry, err := client.OpsLookup(ctx, &unixfs_rpc.OpsLookupRequest{
		CursorHandleId: init.GetCursorHandleId(), OpsHandleId: root.GetOpsHandleId(),
		ClientHandleId: init.GetClientHandleId(), Name: "file.txt",
	})
	if err != nil || entry.GetUnixfsError() != nil {
		t.Fatalf("file lookup = %v, error %v", entry, err)
	}
	file, err := client.GetCursorOps(ctx, &unixfs_rpc.GetCursorOpsRequest{CursorHandleId: entry.GetCursorHandleId()})
	if err != nil || file.GetOpsHandleId() == 0 {
		t.Fatalf("file operations = %v, error %v", file, err)
	}

	// Read a range and its version from the real host file before attempting the write.
	read, err := client.OpsReadAt(ctx, &unixfs_rpc.OpsReadAtRequest{
		OpsHandleId: file.GetOpsHandleId(), Offset: 1, Size: 2, WantVersion: true,
	})
	if err != nil || read.GetUnixfsError() != nil || string(read.GetData()) != "ri" {
		t.Fatalf("file range = %v, error %v, want ri", read, err)
	}
	if len(read.GetVersion()) == 0 {
		t.Fatal("host read returned an empty version")
	}

	// Populate every request field so refusal depends on the configured root.
	written, err := client.OpsWriteAt(ctx, &unixfs_rpc.OpsWriteAtRequest{
		OpsHandleId: file.GetOpsHandleId(), Offset: 1, Data: []byte("WAVE"), Timestamp: timestamppb.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	writeErr := written.GetUnixfsError().ToGoError()
	want := "original"
	if writable {
		if writeErr != nil {
			t.Fatal(writeErr)
		}
		want = "oWAVEnal"
	}
	if !writable && !errors.Is(writeErr, unixfs_errors.ErrReadOnly) {
		t.Fatalf("write error = %v, want read-only refusal", writeErr)
	}

	// A read-only root also refuses the streaming file-create method.
	if !writable {
		checkReadOnlyUpload(t, ctx, client, init.GetCursorHandleId(), root.GetOpsHandleId())
	}
	if writable {
		checkInterruptedUpload(t, ctx, client, init.GetCursorHandleId(), root.GetOpsHandleId(), dir)
	}

	// Check the host bytes independently of the RPC service.
	data, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	if err != nil || string(data) != want {
		t.Fatalf("host file = %q, error %v, want %q", data, err, want)
	}
}

// checkCheckoutRootWatch observes a direct edit through a remote child cursor.
func checkCheckoutRootWatch(t *testing.T, parent context.Context, device peer.ID, viewer *transport.SessionTransport, id protocol.ID, host string) {
	// Create the directory before opening its cursor so it has no prior changes.
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 15*time.Second)
	defer cancel()
	dir := filepath.Join(host, "watched")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Open and resolve the directory down the admitted RPC connection.
	ms := openFrameControl(t, ctx, device, viewer, id)
	mc, err := srpc.NewMuxedConnWithRwc(ctx, ms.GetStream(), true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mc.Close()
	rpc := unixfs_rpc.NewSRPCFSCursorServiceClient(srpc.NewClientWithMuxedConn(mc))
	client, err := unixfs_rpc_client.BuildFSCursorClient(ctx, rpc)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Release()
	root, ops, err := getCheckoutRootOps(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Release()
	child, err := ops.Lookup(ctx, "watched")
	if err != nil {
		t.Fatal(err)
	}
	childHandle, _, err := getCheckoutRootOps(ctx, child)
	if err != nil {
		child.Release()
		t.Fatal(err)
	}
	defer childHandle.Release()

	// Mutate the host through the OS and wait for the native cursor event.
	changed := make(chan *unixfs.FSCursorChange, 1)
	child.AddChangeCb(func(change *unixfs.FSCursorChange) bool {
		changed <- change
		return false
	})
	if err := os.WriteFile(filepath.Join(dir, "from-host.txt"), []byte("host"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case change := <-changed:
		if change.Released || change.Size != 0 || change.Offset != 0 {
			t.Fatalf("host edit produced %v, want directory invalidation", change)
		}
	case <-ctx.Done():
		t.Fatalf("host edit did not wake the open remote cursor: %v", ctx.Err())
	}

	// Moving the directory itself retires the cursor at its former path.
	removed := make(chan struct{}, 1)
	child.AddChangeCb(func(change *unixfs.FSCursorChange) bool {
		if !change.Released {
			return true
		}
		removed <- struct{}{}
		return false
	})
	if err := os.Rename(dir, dir+"-moved"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-removed:
	case <-ctx.Done():
		t.Fatalf("host move did not release the remote directory cursor: %v", ctx.Err())
	}
}

// getCheckoutRootOps resolves one cursor through the same handle path as a viewer.
func getCheckoutRootOps(ctx context.Context, cursor unixfs.FSCursor) (*unixfs.FSHandle, unixfs.FSCursorOps, error) {
	// Hold a handle to resolve the remote cursor and its operations.
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		return nil, nil, err
	}
	_, ops, err := handle.GetOps(ctx)
	if err != nil {
		handle.Release()
		return nil, nil, err
	}
	return handle, ops, nil
}

// checkCheckoutRootResource resolves a Device protocol as an ordinary FSHandle Resource.
func checkCheckoutRootResource(t *testing.T, ctx context.Context, le *logrus.Entry, b bus.Bus, session peer.ID, engine world.Engine, capabilityID, dir string) {
	// Serve the Device over the Resource RPC with a read-only World grant.
	t.Helper()
	device := s4wave_device.NewDeviceResource(le, b, session, engine, world.NewEngineWorldState(engine, false), testSelfKey)
	resources, cleanup := resource_testbed.SetupResourceClientWithRoot(ctx, t, device.GetMux())
	defer cleanup()
	deviceRef := resources.AccessRootResource()
	defer deviceRef.Release()
	deviceClient, err := deviceRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	deviceService := s4wave_device.NewSRPCDeviceResourceServiceClient(deviceClient)

	// Select the live protocol by name and require a readable filesystem resource.
	opened, err := deviceService.AccessCheckoutRoot(ctx, &s4wave_device.AccessCheckoutRootRequest{Name: "skiffos"})
	if err != nil {
		t.Fatal(err)
	}
	if opened.GetCapabilityId() != capabilityID || opened.GetWriteEnabled() || !opened.GetWriteAvailable() {
		t.Fatalf("opened checkout root = %v", opened)
	}
	rootRef := resources.CreateResourceReference(opened.GetResourceId())
	defer rootRef.Release()
	rootClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	root := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(rootClient)
	entry, err := root.LookupPath(ctx, &s4wave_unixfs.HandleLookupPathRequest{Path: "file.txt"})
	if err != nil {
		t.Fatal(err)
	}
	fileRef := resources.CreateResourceReference(entry.GetResourceId())
	defer fileRef.Release()
	fileClient, err := fileRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	file := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(fileClient)

	// Read metadata with the directory listing before accessing any file data.
	watchCtx, stopWatch := context.WithTimeout(ctx, 15*time.Second)
	defer stopWatch()
	listing, err := root.WatchReaddir(watchCtx, &s4wave_unixfs.HandleWatchReaddirRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer listing.Close()
	snapshot, err := listing.Recv()
	if err != nil {
		t.Fatal(err)
	}
	index := slices.IndexFunc(snapshot.GetEntries(), func(entry *s4wave_unixfs.DirEntry) bool {
		return entry.GetName() == "file.txt"
	})
	if index < 0 {
		t.Fatal("checkout-root listing omitted the host file")
	}
	info := snapshot.GetEntries()[index]
	if info.GetSize() != 8 || info.GetMode() != 0o600 || info.GetModTime() == 0 {
		t.Fatalf("checkout-root listing metadata = %v", info)
	}

	// A host edit should refresh the already-open Resource listing.
	if err := os.WriteFile(filepath.Join(dir, "created-on-host.txt"), []byte("host"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitCheckoutListingEntry(t, listing, "created-on-host.txt", 4)

	// A content edit must also invalidate the directory's listing metadata.
	if err := os.WriteFile(filepath.Join(dir, "created-on-host.txt"), []byte("host edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitCheckoutListingEntry(t, listing, "created-on-host.txt", 9)

	// Read the host bytes from the child and require it to inherit read-only access.
	want, err := os.ReadFile(filepath.Join(dir, "file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	read, err := file.ReadAt(ctx, &s4wave_unixfs.HandleReadAtRequest{Length: int64(len(want))})
	if err != nil || string(read.GetData()) != string(want) {
		t.Fatalf("live resource data = %v, error %v, want %q", read, err, want)
	}
	_, err = file.WriteAt(ctx, &s4wave_unixfs.HandleWriteAtRequest{Data: []byte("bad")})
	if err == nil || err.Error() != unixfs_errors.ErrReadOnly.Error() {
		t.Fatalf("write through read-only handle = %v", err)
	}

	// Open a second handle for writes using only the capability and access mode.
	opened, err = deviceService.AccessCheckoutRoot(ctx, &s4wave_device.AccessCheckoutRootRequest{Name: "skiffos", Write: true})
	if err != nil || !opened.GetWriteEnabled() {
		t.Fatalf("open writable checkout root = %v, error %v", opened, err)
	}
	writeRef := resources.CreateResourceReference(opened.GetResourceId())
	defer writeRef.Release()
	writeClient, err := writeRef.GetClient()
	if err != nil {
		t.Fatal(err)
	}
	writer := s4wave_unixfs.NewSRPCFSHandleResourceServiceClient(writeClient)

	// Create and rename a file through the Resource SDK, and check the host directory.
	if _, err := writer.Mknod(ctx, &s4wave_unixfs.HandleMknodRequest{
		Names: []string{"new.txt"}, NodeType: s4wave_unixfs.MknodType_MKNOD_TYPE_FILE, CheckExist: true,
	}); err != nil {
		t.Fatal(err)
	}
	waitCheckoutListingEntry(t, listing, "new.txt", 0)
	if _, err := writer.Rename(ctx, &s4wave_unixfs.HandleRenameRequest{SourceName: "new.txt", DestName: "renamed.txt"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "renamed.txt")); err != nil {
		t.Fatal(err)
	}

	waitCheckoutListingEntry(t, listing, "renamed.txt", 0)

	// Check streamed publication through the same Resource SDK.
	checkLiveUploadTree(t, ctx, writer, dir)
}

// waitCheckoutListingEntry waits for the next delta containing the named entry.
func waitCheckoutListingEntry(t *testing.T, listing s4wave_unixfs.SRPCFSHandleResourceService_WatchReaddirClient, name string, size uint64) {
	// Receive deltas until this edit is visible or the watch fails.
	t.Helper()
	for {
		delta, err := listing.Recv()
		if err != nil {
			t.Fatalf("checkout-root listing did not report %q with size %d: %v", name, size, err)
		}
		for _, entry := range delta.GetEntries() {
			if entry.GetName() == name && entry.GetSize() == size {
				return
			}
		}
	}
}

// checkReadOnlyUpload requires a refusal with all file upload start fields populated.
func checkReadOnlyUpload(t *testing.T, ctx context.Context, client unixfs_rpc.SRPCFSCursorServiceClient, cursorID, opsID uint64) {
	// Open the upload stream and populate every field before closing the sender.
	t.Helper()
	strm, err := client.OpsMknodWithContent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer strm.Close()
	// The Device may return its refusal before Send finishes.
	_ = strm.Send(&unixfs_rpc.OpsMknodWithContentRequest{
		Body: &unixfs_rpc.OpsMknodWithContentRequest_Start{Start: &unixfs_rpc.OpsMknodWithContentStart{
			CursorHandleId: cursorID, OpsHandleId: opsID, Name: "cannot-create",
			TotalSize: 1, Permissions: 0o600, Timestamp: timestamppb.Now(), Expect: &unixfs_rpc.OpsMknodWithContentStart_Exclusive{Exclusive: true},
		}},
	})

	// Refusal must come from the Device before the body starts.
	resp, err := strm.CloseAndRecv()
	if err != nil || !errors.Is(resp.GetUnixfsError().ToGoError(), unixfs_errors.ErrReadOnly) {
		t.Fatalf("read-only upload returned %v, error %v", resp, err)
	}
}

// checkInterruptedUpload verifies cleanup before an incomplete Device stream returns.
func checkInterruptedUpload(t *testing.T, ctx context.Context, client unixfs_rpc.SRPCFSCursorServiceClient, cursorID, opsID uint64, host string) {
	// Send a short body targeting an existing file.
	t.Helper()
	strm, err := client.OpsMknodWithContent(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer strm.Close()
	start := &unixfs_rpc.OpsMknodWithContentStart{
		CursorHandleId: cursorID, OpsHandleId: opsID, Name: "file.txt",
		TotalSize: 9, Permissions: 0o600, Timestamp: timestamppb.Now(),
	}
	if err := strm.Send(&unixfs_rpc.OpsMknodWithContentRequest{Body: &unixfs_rpc.OpsMknodWithContentRequest_Start{Start: start}}); err != nil {
		t.Fatal(err)
	}
	if err := strm.Send(&unixfs_rpc.OpsMknodWithContentRequest{Body: &unixfs_rpc.OpsMknodWithContentRequest_Data{Data: []byte("short")}}); err != nil {
		t.Fatal(err)
	}

	// Wait for publication to fail and its staging entry to be removed.
	res, err := strm.CloseAndRecv()
	if err != nil || res.GetUnixfsError() == nil {
		t.Fatalf("incomplete upload returned %v, error %v", res, err)
	}
	entries, err := os.ReadDir(host)
	if err != nil || len(entries) != 1 || entries[0].Name() != "file.txt" {
		t.Fatalf("host entries after incomplete upload = %v, error %v", entries, err)
	}
}

// checkLiveUploadTree checks batch completion and partial batch retention on disk.
func checkLiveUploadTree(t *testing.T, ctx context.Context, root s4wave_unixfs.SRPCFSHandleResourceServiceClient, host string) {
	// Publish a folder and two files in one batch through the Device stream.
	t.Helper()
	strm, err := root.UploadTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer strm.Close()
	dir := &s4wave_unixfs.HandleUploadTreeDirectory{Path: "upload", ExpectMissing: true}
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{Body: &s4wave_unixfs.HandleUploadTreeRequest_Directory{Directory: dir}}); err != nil {
		t.Fatal(err)
	}
	sendLiveUploadFile(t, strm, "upload/data.txt", 4, "data")
	sendLiveUploadFile(t, strm, "upload/empty.txt", 0, "")
	resp, err := strm.CloseAndRecv()
	written := resp.GetWritten()
	if err != nil || written.GetFilesWritten() != 2 || written.GetDirectoriesWritten() != 1 || written.GetBytesWritten() != 4 {
		t.Fatalf("folder upload returned %v, error %v", resp, err)
	}

	// Interrupt a later file after an earlier one in the batch was published.
	broken, err := root.UploadTree(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer broken.Close()
	sendLiveUploadFile(t, broken, "upload/retained.txt", 4, "keep")
	sendLiveUploadFile(t, broken, "upload/partial.txt", int64(unixfs_rpc.FileUploadChunkSize+1), strings.Repeat("x", unixfs_rpc.FileUploadChunkSize))
	if _, err := broken.CloseAndRecv(); err == nil {
		t.Fatal("short upload succeeded")
	}

	// Confirm what reached the host before the interrupted batch ended.
	for name, want := range map[string]string{"data.txt": "data", "empty.txt": "", "retained.txt": "keep"} {
		bytes, err := os.ReadFile(filepath.Join(host, "upload", name))
		if err != nil || string(bytes) != want {
			t.Fatalf("host upload %s = %q, error %v", name, bytes, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(host, "upload"))
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".spacewave-upload-") {
			names = append(names, entry.Name())
		}
	}
	if !slices.Equal(names, []string{"data.txt", "empty.txt", "retained.txt"}) {
		t.Fatalf("published names = %v", names)
	}
}

// sendLiveUploadFile sends the start and one data frame of a test file.
func sendLiveUploadFile(t *testing.T, strm s4wave_unixfs.SRPCFSHandleResourceService_UploadTreeClient, path string, size int64, data string) {
	// Start the file, then send its frame when it has content.
	t.Helper()
	start := &s4wave_unixfs.HandleUploadTreeFileStart{Path: path, TotalSize: size, Mode: 0o600}
	if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{Body: &s4wave_unixfs.HandleUploadTreeRequest_FileStart{FileStart: start}}); err != nil {
		t.Fatal(err)
	}
	if data != "" {
		if err := strm.Send(&s4wave_unixfs.HandleUploadTreeRequest{Body: &s4wave_unixfs.HandleUploadTreeRequest_Data{Data: []byte(data)}}); err != nil {
			t.Fatal(err)
		}
	}
}
