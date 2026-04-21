package unixfs_v86fs

import (
	"bytes"
	"context"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_block_fs "github.com/s4wave/spacewave/db/unixfs/block/fs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	iofs_mock "github.com/s4wave/spacewave/db/unixfs/iofs/mock"
	"github.com/sirupsen/logrus"
)

// buildTestServer creates an in-process v86fs server with a mock filesystem.
// Returns the SRPC client for the v86fs service and a cleanup function.
func buildTestServer(t *testing.T, ctx context.Context) SRPCV86FsServiceClient {
	t.Helper()

	ifs, _ := iofs_mock.NewMockIoFS()
	fsc, err := unixfs_iofs.NewFSCursor(ifs)
	if err != nil {
		t.Fatal(err.Error())
	}
	handle, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(handle.Release)

	resolver := func(_ context.Context, name string) (*unixfs.FSHandle, error) {
		if name == "" || name == "root" {
			return handle.Clone(ctx)
		}
		return nil, unixfs_errors.ErrNotExist
	}

	srv := NewServer(resolver)
	mux := srpc.NewMux()
	if err := SRPCRegisterV86FsService(mux, srv); err != nil {
		t.Fatal(err.Error())
	}
	server := srpc.NewServer(mux)
	pipe := srpc.NewServerPipe(server)
	client := srpc.NewClient(pipe)
	return NewSRPCV86FsServiceClient(client)
}

// TestRelayMountLookupRead tests the basic MOUNT + LOOKUP + READ flow.
func TestRelayMountLookupRead(t *testing.T) {
	ctx := context.Background()
	client := buildTestServer(t, ctx)

	strm, err := client.RelayV86Fs(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strm.Close()

	// MOUNT root
	err = strm.Send(&V86FsMessage{
		Tag:  1,
		Body: &V86FsMessage_MountRequest{MountRequest: &V86FsMountRequest{Name: ""}},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	reply, err := strm.Recv()
	if err != nil {
		t.Fatal(err.Error())
	}
	mountReply := reply.GetMountReply()
	if mountReply == nil {
		t.Fatalf("expected mount reply, got %T", reply.GetBody())
	}
	if mountReply.GetStatus() != 0 {
		t.Fatalf("mount failed with status %d", mountReply.GetStatus())
	}
	rootID := mountReply.GetRootInodeId()
	if rootID == 0 {
		t.Fatal("expected non-zero root inode ID")
	}

	// LOOKUP test.txt
	err = strm.Send(&V86FsMessage{
		Tag: 2,
		Body: &V86FsMessage_LookupRequest{LookupRequest: &V86FsLookupRequest{
			ParentId: rootID,
			Name:     "test.txt",
		}},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	reply, err = strm.Recv()
	if err != nil {
		t.Fatal(err.Error())
	}
	lookupReply := reply.GetLookupReply()
	if lookupReply == nil {
		t.Fatalf("expected lookup reply, got %T", reply.GetBody())
	}
	if lookupReply.GetStatus() != 0 {
		t.Fatalf("lookup failed with status %d", lookupReply.GetStatus())
	}
	fileID := lookupReply.GetInodeId()
	if fileID == 0 {
		t.Fatal("expected non-zero file inode ID")
	}
	if lookupReply.GetSize() != 11 { // "hello world"
		t.Fatalf("expected size 11, got %d", lookupReply.GetSize())
	}

	// OPEN file
	err = strm.Send(&V86FsMessage{
		Tag: 3,
		Body: &V86FsMessage_OpenRequest{OpenRequest: &V86FsOpenRequest{
			InodeId: fileID,
			Flags:   0,
		}},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	reply, err = strm.Recv()
	if err != nil {
		t.Fatal(err.Error())
	}
	openReply := reply.GetOpenReply()
	if openReply == nil {
		t.Fatalf("expected open reply, got %T", reply.GetBody())
	}
	handleID := openReply.GetHandleId()

	// READ file
	err = strm.Send(&V86FsMessage{
		Tag: 4,
		Body: &V86FsMessage_ReadRequest{ReadRequest: &V86FsReadRequest{
			HandleId: handleID,
			Offset:   0,
			Size:     1024,
		}},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	reply, err = strm.Recv()
	if err != nil {
		t.Fatal(err.Error())
	}
	readReply := reply.GetReadReply()
	if readReply == nil {
		t.Fatalf("expected read reply, got %T", reply.GetBody())
	}
	if readReply.GetStatus() != 0 {
		t.Fatalf("read failed with status %d", readReply.GetStatus())
	}
	expected := []byte("hello world")
	if !bytes.Equal(readReply.GetData(), expected) {
		t.Fatalf("expected %q, got %q", expected, readReply.GetData())
	}

	// CLOSE handle
	err = strm.Send(&V86FsMessage{
		Tag: 5,
		Body: &V86FsMessage_CloseRequest{CloseRequest: &V86FsCloseRequest{
			HandleId: handleID,
		}},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	reply, err = strm.Recv()
	if err != nil {
		t.Fatal(err.Error())
	}
	closeReply := reply.GetCloseReply()
	if closeReply == nil {
		t.Fatalf("expected close reply, got %T", reply.GetBody())
	}
}

// sendRecv sends a message and returns the reply.
func sendRecv(t *testing.T, strm SRPCV86FsService_RelayV86FsClient, msg *V86FsMessage) *V86FsMessage {
	t.Helper()
	if err := strm.Send(msg); err != nil {
		t.Fatal(err.Error())
	}
	reply, err := strm.Recv()
	if err != nil {
		t.Fatal(err.Error())
	}
	return reply
}

// newBillyHandle creates an in-memory writable FSHandle backed by go-billy memfs.
func newBillyHandle(t *testing.T) *unixfs.FSHandle {
	t.Helper()
	bfs := memfs.New()
	if err := bfs.MkdirAll("./", 0o755); err != nil {
		t.Fatal(err.Error())
	}
	fsc := unixfs_billy.NewBillyFSCursor(bfs, "")
	h, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(h.Release)
	return h
}

// buildMultiMountServer creates a server with "workspace" and "home" mounts.
func buildMultiMountServer(t *testing.T, ctx context.Context, workspace, home *unixfs.FSHandle) SRPCV86FsServiceClient {
	t.Helper()
	resolver := func(_ context.Context, name string) (*unixfs.FSHandle, error) {
		switch name {
		case "workspace":
			return workspace.Clone(ctx)
		case "home":
			return home.Clone(ctx)
		}
		return nil, unixfs_errors.ErrNotExist
	}
	srv := NewServer(resolver)
	mux := srpc.NewMux()
	if err := SRPCRegisterV86FsService(mux, srv); err != nil {
		t.Fatal(err.Error())
	}
	server := srpc.NewServer(mux)
	pipe := srpc.NewServerPipe(server)
	client := srpc.NewClient(pipe)
	return NewSRPCV86FsServiceClient(client)
}

// TestRelayMultiMountIsolation tests full file lifecycle with multi-mount isolation.
func TestRelayMultiMountIsolation(t *testing.T) {
	ctx := context.Background()
	wsHandle := newBillyHandle(t)
	homeHandle := newBillyHandle(t)
	client := buildMultiMountServer(t, ctx, wsHandle, homeHandle)

	strm, err := client.RelayV86Fs(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strm.Close()

	tag := uint32(0)
	nextTag := func() uint32 { tag++; return tag }

	// Mount workspace
	reply := sendRecv(t, strm, &V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_MountRequest{MountRequest: &V86FsMountRequest{Name: "workspace"}},
	})
	wsRootID := reply.GetMountReply().GetRootInodeId()
	if wsRootID == 0 {
		t.Fatal("expected workspace mount root")
	}

	// Mount home
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_MountRequest{MountRequest: &V86FsMountRequest{Name: "home"}},
	})
	homeRootID := reply.GetMountReply().GetRootInodeId()
	if homeRootID == 0 {
		t.Fatal("expected home mount root")
	}

	// CREATE file in workspace
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag: nextTag(),
		Body: &V86FsMessage_CreateRequest{CreateRequest: &V86FsCreateRequest{
			ParentId: wsRootID,
			Name:     "project.txt",
			Mode:     sIFREG | 0o644,
		}},
	})
	createReply := reply.GetCreateReply()
	if createReply == nil || createReply.GetStatus() != 0 {
		t.Fatalf("create failed: %v", reply.GetBody())
	}
	fileID := createReply.GetInodeId()

	// WRITE data to the created file
	fileData := []byte("workspace-only content")
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag: nextTag(),
		Body: &V86FsMessage_WriteRequest{WriteRequest: &V86FsWriteRequest{
			InodeId: fileID,
			Offset:  0,
			Data:    fileData,
		}},
	})
	writeReply := reply.GetWriteReply()
	if writeReply == nil || writeReply.GetStatus() != 0 {
		t.Fatalf("write failed: %v", reply.GetBody())
	}
	if int(writeReply.GetBytesWritten()) != len(fileData) {
		t.Fatalf("expected %d bytes written, got %d", len(fileData), writeReply.GetBytesWritten())
	}

	// GETATTR the file
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag: nextTag(),
		Body: &V86FsMessage_GetattrRequest{GetattrRequest: &V86FsGetattrRequest{
			InodeId: fileID,
		}},
	})
	getattrReply := reply.GetGetattrReply()
	if getattrReply == nil || getattrReply.GetStatus() != 0 {
		t.Fatalf("getattr failed: %v", reply.GetBody())
	}
	if getattrReply.GetSize() != uint64(len(fileData)) {
		t.Fatalf("expected size %d, got %d", len(fileData), getattrReply.GetSize())
	}

	// READ back from workspace to verify
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag: nextTag(),
		Body: &V86FsMessage_LookupRequest{LookupRequest: &V86FsLookupRequest{
			ParentId: wsRootID,
			Name:     "project.txt",
		}},
	})
	lookupReply := reply.GetLookupReply()
	if lookupReply == nil || lookupReply.GetStatus() != 0 {
		t.Fatalf("lookup project.txt failed: %v", reply.GetBody())
	}

	// OPEN + READ the file via lookup inode
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag: nextTag(),
		Body: &V86FsMessage_OpenRequest{OpenRequest: &V86FsOpenRequest{
			InodeId: lookupReply.GetInodeId(),
		}},
	})
	handleID := reply.GetOpenReply().GetHandleId()

	reply = sendRecv(t, strm, &V86FsMessage{
		Tag: nextTag(),
		Body: &V86FsMessage_ReadRequest{ReadRequest: &V86FsReadRequest{
			HandleId: handleID,
			Offset:   0,
			Size:     1024,
		}},
	})
	if !bytes.Equal(reply.GetReadReply().GetData(), fileData) {
		t.Fatalf("read-back mismatch: got %q", reply.GetReadReply().GetData())
	}

	sendRecv(t, strm, &V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_CloseRequest{CloseRequest: &V86FsCloseRequest{HandleId: handleID}},
	})

	// READDIR on workspace should show the file
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag: nextTag(),
		Body: &V86FsMessage_ReaddirRequest{ReaddirRequest: &V86FsReaddirRequest{
			DirId: wsRootID,
		}},
	})
	readdirReply := reply.GetReaddirReply()
	if readdirReply == nil || readdirReply.GetStatus() != 0 {
		t.Fatalf("readdir workspace failed: %v", reply.GetBody())
	}
	found := false
	for _, ent := range readdirReply.GetEntries() {
		if ent.GetName() == "project.txt" {
			found = true
		}
	}
	if !found {
		t.Fatal("project.txt not found in workspace readdir")
	}

	// LOOKUP project.txt in HOME should fail (isolation)
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag: nextTag(),
		Body: &V86FsMessage_LookupRequest{LookupRequest: &V86FsLookupRequest{
			ParentId: homeRootID,
			Name:     "project.txt",
		}},
	})
	// Should get an error reply (ENOENT)
	if errReply := reply.GetErrorReply(); errReply != nil {
		if errReply.GetStatus() != enoent {
			t.Fatalf("expected ENOENT, got status %d", errReply.GetStatus())
		}
	} else if lr := reply.GetLookupReply(); lr != nil {
		if lr.GetStatus() == 0 {
			t.Fatal("project.txt should NOT be visible in home mount")
		}
	}

	// READDIR on home should be empty
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag: nextTag(),
		Body: &V86FsMessage_ReaddirRequest{ReaddirRequest: &V86FsReaddirRequest{
			DirId: homeRootID,
		}},
	})
	homeDir := reply.GetReaddirReply()
	if homeDir == nil || homeDir.GetStatus() != 0 {
		t.Fatalf("readdir home failed: %v", reply.GetBody())
	}
	for _, ent := range homeDir.GetEntries() {
		if ent.GetName() == "project.txt" {
			t.Fatal("project.txt should NOT appear in home readdir")
		}
	}

	// UNLINK the file from workspace
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag: nextTag(),
		Body: &V86FsMessage_UnlinkRequest{UnlinkRequest: &V86FsUnlinkRequest{
			ParentId: wsRootID,
			Name:     "project.txt",
		}},
	})
	unlinkReply := reply.GetUnlinkReply()
	if unlinkReply == nil || unlinkReply.GetStatus() != 0 {
		t.Fatalf("unlink failed: %v", reply.GetBody())
	}

	// Verify file is gone from workspace
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag: nextTag(),
		Body: &V86FsMessage_ReaddirRequest{ReaddirRequest: &V86FsReaddirRequest{
			DirId: wsRootID,
		}},
	})
	for _, ent := range reply.GetReaddirReply().GetEntries() {
		if ent.GetName() == "project.txt" {
			t.Fatal("project.txt should be gone after unlink")
		}
	}
}

// TestRelayPushInvalidation tests that change callbacks produce
// INVALIDATE messages on the stream.
func TestRelayPushInvalidation(t *testing.T) {
	ctx := context.Background()

	// Use the block-based testbed for a cursor that fires change callbacks.
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Initialize with a directory root.
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(unixfs_block.NewFSNode(unixfs_block.NodeType_NodeType_DIRECTORY, 0, nil), true)
	resRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc.SetRootRef(resRef)

	writer := unixfs_block_fs.NewFSWriter()
	blockFS := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, oc, writer)
	writer.SetFS(blockFS)
	writer.SetTimestamp(timestamp.Now())

	rootHandle, err := unixfs.NewFSHandle(blockFS)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rootHandle.Release()

	// Pre-create a file.
	now := time.Now()
	err = rootHandle.Mknod(ctx, true, []string{"data.txt"}, unixfs.NewFSCursorNodeType_File(), 0o644, now)
	if err != nil {
		t.Fatal(err.Error())
	}
	fileHandle, err := rootHandle.Lookup(ctx, "data.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fileHandle.Release()
	err = fileHandle.WriteAt(ctx, 0, []byte("initial"), now)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Build server with this block FS.
	resolver := func(_ context.Context, name string) (*unixfs.FSHandle, error) {
		if name == "" || name == "workspace" {
			return rootHandle.Clone(ctx)
		}
		return nil, unixfs_errors.ErrNotExist
	}
	srv := NewServer(resolver)
	mux := srpc.NewMux()
	if err := SRPCRegisterV86FsService(mux, srv); err != nil {
		t.Fatal(err.Error())
	}
	server := srpc.NewServer(mux)
	pipe := srpc.NewServerPipe(server)
	client := srpc.NewClient(pipe)
	v86Client := NewSRPCV86FsServiceClient(client)

	strm, err := v86Client.RelayV86Fs(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strm.Close()

	tag := uint32(0)
	nextTag := func() uint32 { tag++; return tag }

	// Mount to register root inode.
	reply := sendRecv(t, strm, &V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_MountRequest{MountRequest: &V86FsMountRequest{Name: "workspace"}},
	})
	wsRootID := reply.GetMountReply().GetRootInodeId()

	// Lookup data.txt to register its inode (which registers the change callback).
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag: nextTag(),
		Body: &V86FsMessage_LookupRequest{LookupRequest: &V86FsLookupRequest{
			ParentId: wsRootID,
			Name:     "data.txt",
		}},
	})
	fileInodeID := reply.GetLookupReply().GetInodeId()
	if fileInodeID == 0 {
		t.Fatal("expected non-zero inode for data.txt")
	}

	// Modify file externally via FSHandle (bypassing the relay).
	err = fileHandle.WriteAt(ctx, 0, []byte("modified content"), time.Now())
	if err != nil {
		t.Fatal(err.Error())
	}

	// Send a STATFS as a "ping" so the server loop has a chance to drain notifyCh.
	if err := strm.Send(&V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_StatfsRequest{StatfsRequest: &V86FsStatfsRequest{}},
	}); err != nil {
		t.Fatal(err.Error())
	}

	// Read messages. We should see INVALIDATE before or after the STATFS reply.
	gotInvalidate := false
	for range 10 {
		msg, err := strm.Recv()
		if err != nil {
			t.Fatal(err.Error())
		}
		if inv := msg.GetInvalidate(); inv != nil {
			if inv.GetInodeId() == fileInodeID {
				gotInvalidate = true
				break
			}
			// Skip invalidations for other inodes (e.g., parent dir).
			continue
		}
		if msg.GetStatfsReply() != nil && gotInvalidate {
			break
		}
	}
	if !gotInvalidate {
		t.Fatal("expected INVALIDATE message after external write, got none")
	}
}

// TestRelayMountManagement tests AddMount/RemoveMount with MOUNT_NOTIFY/UMOUNT_NOTIFY.
func TestRelayMountManagement(t *testing.T) {
	ctx := context.Background()
	wsHandle := newBillyHandle(t)

	// Pre-create a file in the workspace.
	err := wsHandle.Mknod(ctx, true, []string{"readme.md"}, unixfs.NewFSCursorNodeType_File(), 0o644, time.Now())
	if err != nil {
		t.Fatal(err.Error())
	}
	fh, err := wsHandle.Lookup(ctx, "readme.md")
	if err != nil {
		t.Fatal(err.Error())
	}
	err = fh.WriteAt(ctx, 0, []byte("# hello"), time.Now())
	fh.Release()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create server with no static resolver, only dynamic mounts.
	srv := NewServer(nil)
	mux := srpc.NewMux()
	if err := SRPCRegisterV86FsService(mux, srv); err != nil {
		t.Fatal(err.Error())
	}
	server := srpc.NewServer(mux)
	pipe := srpc.NewServerPipe(server)
	client := srpc.NewClient(pipe)
	v86Client := NewSRPCV86FsServiceClient(client)

	strm, err := v86Client.RelayV86Fs(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strm.Close()

	tag := uint32(0)
	nextTag := func() uint32 { tag++; return tag }

	// Sync: send a STATFS request to confirm the session is running
	// before calling AddMount. Without this, AddMount races with
	// session registration and the MOUNT_NOTIFY may be lost.
	sendRecv(t, strm, &V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_StatfsRequest{StatfsRequest: &V86FsStatfsRequest{}},
	})

	// AddMount dynamically.
	srv.AddMount("workspace", "/workspace", wsHandle)
	if err := strm.Send(&V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_MountRequest{MountRequest: &V86FsMountRequest{Name: "workspace"}},
	}); err != nil {
		t.Fatal(err.Error())
	}

	gotMountNotify := false
	var mountReplyMsg *V86FsMessage
	for range 5 {
		msg, err := strm.Recv()
		if err != nil {
			t.Fatal(err.Error())
		}
		if mn := msg.GetMountNotify(); mn != nil {
			gotMountNotify = true
			if mn.GetName() != "workspace" {
				t.Fatalf("expected mount name 'workspace', got %q", mn.GetName())
			}
			if mn.GetMountPath() != "/workspace" {
				t.Fatalf("expected mount path '/workspace', got %q", mn.GetMountPath())
			}
		}
		if msg.GetMountReply() != nil {
			mountReplyMsg = msg
		}
		if gotMountNotify && mountReplyMsg != nil {
			break
		}
	}
	if !gotMountNotify {
		t.Fatal("expected MOUNT_NOTIFY after AddMount")
	}

	// Use the MOUNT reply.
	reply := mountReplyMsg
	mountReply := reply.GetMountReply()
	if mountReply == nil || mountReply.GetStatus() != 0 {
		t.Fatalf("mount workspace failed: %v", reply.GetBody())
	}
	wsRootID := mountReply.GetRootInodeId()

	// READDIR to confirm readme.md is visible.
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_ReaddirRequest{ReaddirRequest: &V86FsReaddirRequest{DirId: wsRootID}},
	})
	found := false
	for _, ent := range reply.GetReaddirReply().GetEntries() {
		if ent.GetName() == "readme.md" {
			found = true
		}
	}
	if !found {
		t.Fatal("readme.md not found in dynamically mounted workspace")
	}

	// ListMounts returns the mount.
	mounts := srv.ListMounts()
	if len(mounts) != 1 || mounts[0].Name != "workspace" {
		t.Fatalf("expected 1 mount 'workspace', got %v", mounts)
	}

	// RemoveMount.
	srv.RemoveMount("workspace")

	// Verify UMOUNT_NOTIFY arrives.
	if err := strm.Send(&V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_StatfsRequest{StatfsRequest: &V86FsStatfsRequest{}},
	}); err != nil {
		t.Fatal(err.Error())
	}

	gotUmountNotify := false
	gotStatfs2 := false
	for range 5 {
		msg, err := strm.Recv()
		if err != nil {
			t.Fatal(err.Error())
		}
		if um := msg.GetUmountNotify(); um != nil {
			gotUmountNotify = true
			if um.GetMountPath() != "/workspace" {
				t.Fatalf("expected umount path '/workspace', got %q", um.GetMountPath())
			}
		}
		if msg.GetStatfsReply() != nil {
			gotStatfs2 = true
		}
		if gotUmountNotify && gotStatfs2 {
			break
		}
	}
	if !gotUmountNotify {
		t.Fatal("expected UMOUNT_NOTIFY after RemoveMount")
	}

	// ListMounts should be empty now.
	mounts = srv.ListMounts()
	if len(mounts) != 0 {
		t.Fatalf("expected 0 mounts after RemoveMount, got %d", len(mounts))
	}
}
