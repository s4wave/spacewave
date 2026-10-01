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
	// Mark the function as a test helper.
	t.Helper()

	// Build a mock IO FS cursor and release the handle at cleanup.
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

	// Install a resolver that clones the root handle for known names.
	resolver := func(_ context.Context, name string) (*unixfs.FSHandle, error) {
		if name == "" || name == "root" {
			return handle.Clone(ctx)
		}
		return nil, unixfs_errors.ErrNotExist
	}

	// Register the server over an SRPC pipe and return the client.
	srv := NewServer(nil, resolver)
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
	// Set up the test context.
	ctx := context.Background()

	// Set up the context and the test server client.
	client := buildTestServer(t, ctx)

	// Open the relay stream.
	strm, err := client.RelayV86Fs(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strm.Close()

	// MOUNT root

	// Mount the root and check the reply.
	err = strm.Send(&V86FsMessage{
		Tag:  1,
		Body: &V86FsMessage_MountRequest{MountRequest: &V86FsMountRequest{Name: ""}},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Receive the mount reply and check it.
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

	// Look up test.txt and check the reply.
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

	// Receive the lookup reply and check it.
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

	// Open the file and capture the handle ID.
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

	// Receive the open reply.
	openReply := reply.GetOpenReply()
	if openReply == nil {
		t.Fatalf("expected open reply, got %T", reply.GetBody())
	}
	handleID := openReply.GetHandleId()

	// READ file

	// Read the file and check the data.
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

	// Receive the read reply and check it.
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

	// Close the handle and check the reply.
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

	// Receive the close reply and check it.
	closeReply := reply.GetCloseReply()
	if closeReply == nil {
		t.Fatalf("expected close reply, got %T", reply.GetBody())
	}
}

// sendRecv sends a message and returns the reply.
func sendRecv(t *testing.T, strm SRPCV86FsService_RelayV86FsClient, msg *V86FsMessage) *V86FsMessage {
	// Send the message and return the reply.
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
	// Mark the function as a test helper.
	t.Helper()

	// Create an in-memory billy filesystem and wrap it in an FSHandle.
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
	// Mark the function as a test helper.
	t.Helper()

	// Install a resolver that clones the named mount handle.
	resolver := func(_ context.Context, name string) (*unixfs.FSHandle, error) {
		switch name {
		case "workspace":
			return workspace.Clone(ctx)
		case "home":
			return home.Clone(ctx)
		}
		return nil, unixfs_errors.ErrNotExist
	}

	// Register the server over an SRPC pipe and return the client.
	srv := NewServer(nil, resolver)
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

	// Set up the context, two mount handles, the server client, and the relay stream.
	ctx := context.Background()
	wsHandle := newBillyHandle(t)
	homeHandle := newBillyHandle(t)
	client := buildMultiMountServer(t, ctx, wsHandle, homeHandle)

	// Open the relay stream and build a tag counter.
	strm, err := client.RelayV86Fs(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strm.Close()

	// Build a tag counter for the stream messages.
	tag := uint32(0)
	nextTag := func() uint32 { tag++; return tag }

	// Mount workspace

	// Mount workspace and check the root.
	reply := sendRecv(t, strm, &V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_MountRequest{MountRequest: &V86FsMountRequest{Name: "workspace"}},
	})
	wsRootID := reply.GetMountReply().GetRootInodeId()
	if wsRootID == 0 {
		t.Fatal("expected workspace mount root")
	}

	// Mount home and check the root.
	// Mount home
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_MountRequest{MountRequest: &V86FsMountRequest{Name: "home"}},
	})
	homeRootID := reply.GetMountReply().GetRootInodeId()
	if homeRootID == 0 {
		t.Fatal("expected home mount root")
	}

	// Create a file in workspace and check the reply.
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

	// Write data to the file and check the reply.
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

	// Read the file attributes and check the size.
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

	// Look up the file again through workspace.
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

	// Open the file and read its data back.
	// OPEN + READ the file via lookup inode
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag: nextTag(),
		Body: &V86FsMessage_OpenRequest{OpenRequest: &V86FsOpenRequest{
			InodeId: lookupReply.GetInodeId(),
		}},
	})
	handleID := reply.GetOpenReply().GetHandleId()

	// Read the file back through the open handle.
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

	// Close the handle.
	sendRecv(t, strm, &V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_CloseRequest{CloseRequest: &V86FsCloseRequest{HandleId: handleID}},
	})

	// Read the workspace directory and check the file appears.
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

	// Look up the file in home and check it is absent.
	// LOOKUP project.txt in HOME should fail (isolation)
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag: nextTag(),
		Body: &V86FsMessage_LookupRequest{LookupRequest: &V86FsLookupRequest{
			ParentId: homeRootID,
			Name:     "project.txt",
		}},
	})

	// None
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

	// Read the home directory and check it stays empty.
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

	// Unlink the file from workspace and check the reply.
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

	// Read the workspace directory and check the file is gone.
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

// TestRelaySetattrChmod proves the server handles SETATTR with the mode bit
// against a writable backing: create a file, chmod it, and confirm the new
// permission bits round-trip through GETATTR. apt's fchmod on cache files
// exercises this exact path; a guest-side fchmod EPERM is a guest v86fs driver
// gap, not a server gap, because errnoFromError never produces EPERM (it maps
// unknown errors to ENOSYS and read-only to EROFS).
// Set up the context, two mount handles, the server client, and the relay stream.
func TestRelaySetattrChmod(t *testing.T) {
	// None
	ctx := context.Background()
	wsHandle := newBillyHandle(t)
	homeHandle := newBillyHandle(t)
	client := buildMultiMountServer(t, ctx, wsHandle, homeHandle)

	// Open the relay stream and build a tag counter.
	strm, err := client.RelayV86Fs(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strm.Close()

	// Build a tag counter for the stream messages.
	tag := uint32(0)
	nextTag := func() uint32 { tag++; return tag }

	// Mount workspace and check the root.
	reply := sendRecv(t, strm, &V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_MountRequest{MountRequest: &V86FsMountRequest{Name: "workspace"}},
	})
	wsRootID := reply.GetMountReply().GetRootInodeId()
	if wsRootID == 0 {
		t.Fatal("expected workspace mount root")
	}

	// Create cache.bin and check the reply.
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag: nextTag(),
		Body: &V86FsMessage_CreateRequest{CreateRequest: &V86FsCreateRequest{
			ParentId: wsRootID,
			Name:     "cache.bin",
			Mode:     sIFREG | 0o644,
		}},
	})
	createReply := reply.GetCreateReply()
	if createReply == nil || createReply.GetStatus() != 0 {
		t.Fatalf("create failed: %v", reply.GetBody())
	}
	fileID := createReply.GetInodeId()

	// Set the file mode to 0600 and check the reply.
	reply = sendRecv(t, strm, &V86FsMessage{
		Tag: nextTag(),
		Body: &V86FsMessage_SetattrRequest{SetattrRequest: &V86FsSetattrRequest{
			InodeId: fileID,
			Valid:   attrMode,
			Mode:    0o600,
		}},
	})
	setattrReply := reply.GetSetattrReply()
	if setattrReply == nil || setattrReply.GetStatus() != 0 {
		t.Fatalf("setattr chmod failed: %v", reply.GetBody())
	}

	// Read the attributes back and check the mode applied.
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
	if perm := getattrReply.GetMode() & 0o777; perm != 0o600 {
		t.Fatalf("chmod did not apply: mode perm = %#o, want %#o", perm, 0o600)
	}
}

// TestRelayPushInvalidation tests that change callbacks produce
// INVALIDATE messages on the stream.
func TestRelayPushInvalidation(t *testing.T) {
	// Set up the test context.
	ctx := context.Background()

	// Use the block-based testbed for a cursor that fires change callbacks.

	// Start a testbed holding the block-backed World.
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

	// Initialize the cursor with a directory root and write it.
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(unixfs_block.NewFSNode(unixfs_block.NodeType_NodeType_DIRECTORY, 0, nil), true)
	resRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc.SetRootRef(resRef)

	// Build a block FS writer over the cursor and open the root handle.
	writer := unixfs_block_fs.NewFSWriter()
	blockFS := unixfs_block_fs.NewFS(ctx, unixfs_block.NodeType_NodeType_DIRECTORY, oc, writer)
	writer.SetFS(blockFS)
	writer.SetTimestamp(timestamp.Now())

	// Open the root handle for the block FS.
	rootHandle, err := unixfs.NewFSHandle(blockFS)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rootHandle.Release()

	// Pre-create a file.

	// Pre-create data.txt and write its initial contents.
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

	// Build a server over the block FS and open the relay stream.
	resolver := func(_ context.Context, name string) (*unixfs.FSHandle, error) {
		if name == "" || name == "workspace" {
			return rootHandle.Clone(ctx)
		}
		return nil, unixfs_errors.ErrNotExist
	}
	srv := NewServer(nil, resolver)
	mux := srpc.NewMux()
	if err := SRPCRegisterV86FsService(mux, srv); err != nil {
		t.Fatal(err.Error())
	}
	server := srpc.NewServer(mux)
	pipe := srpc.NewServerPipe(server)
	client := srpc.NewClient(pipe)

	// Open the relay stream and build a tag counter.
	v86Client := NewSRPCV86FsServiceClient(client)

	// Open the relay stream and build a tag counter.
	strm, err := v86Client.RelayV86Fs(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strm.Close()

	// Build a tag counter for the stream messages.
	tag := uint32(0)
	nextTag := func() uint32 { tag++; return tag }

	// Mount workspace and look up data.txt to register the callback.
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

	// Modify the file externally through the FSHandle.
	// Modify file externally via FSHandle (bypassing the relay).
	err = fileHandle.WriteAt(ctx, 0, []byte("modified content"), time.Now())
	if err != nil {
		t.Fatal(err.Error())
	}

	// Send a STATFS ping so the server loop drains notifications.
	// Send a STATFS as a "ping" so the server loop has a chance to drain notifyCh.

	// Send a STATFS ping so the UMOUNT_NOTIFY drains.
	if err := strm.Send(&V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_StatfsRequest{StatfsRequest: &V86FsStatfsRequest{}},
	}); err != nil {
		t.Fatal(err.Error())
	}

	// Read messages until the INVALIDATE for the file arrives.
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

	// Open the relay stream and build a tag counter.
	ctx := context.Background()

	// Set up the context and a writable workspace handle.
	wsHandle := newBillyHandle(t)

	// Pre-create readme.md in the workspace and write its contents.
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

	// Create a server with only dynamic mounts and open the relay stream.
	srv := NewServer(nil, nil)
	mux := srpc.NewMux()
	if err := SRPCRegisterV86FsService(mux, srv); err != nil {
		t.Fatal(err.Error())
	}
	server := srpc.NewServer(mux)
	pipe := srpc.NewServerPipe(server)
	client := srpc.NewClient(pipe)
	v86Client := NewSRPCV86FsServiceClient(client)

	// Open the relay stream and build a tag counter.
	strm, err := v86Client.RelayV86Fs(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strm.Close()

	// Build a tag counter for the stream messages.
	tag := uint32(0)
	nextTag := func() uint32 { tag++; return tag }

	// Sync: send a STATFS request to confirm the session is running
	// before calling AddMount. Without this, AddMount races with
	// session registration and the MOUNT_NOTIFY may be lost.

	// Send a STATFS request to confirm the session is running.
	sendRecv(t, strm, &V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_StatfsRequest{StatfsRequest: &V86FsStatfsRequest{}},
	})

	// Add the workspace mount dynamically and request it.
	// AddMount dynamically.
	srv.AddMount("workspace", "/workspace", wsHandle)
	if err := strm.Send(&V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_MountRequest{MountRequest: &V86FsMountRequest{Name: "workspace"}},
	}); err != nil {
		t.Fatal(err.Error())
	}

	// Read messages until the MOUNT_NOTIFY and MOUNT reply arrive.
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

	// Check the MOUNT reply and capture the root inode.
	reply := mountReplyMsg
	mountReply := reply.GetMountReply()
	if mountReply == nil || mountReply.GetStatus() != 0 {
		t.Fatalf("mount workspace failed: %v", reply.GetBody())
	}
	wsRootID := mountReply.GetRootInodeId()

	// Read the workspace directory and check readme.md is visible.
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

	// Check ListMounts returns the workspace mount.
	mounts := srv.ListMounts()
	if len(mounts) != 1 || mounts[0].Name != "workspace" {
		t.Fatalf("expected 1 mount 'workspace', got %v", mounts)
	}

	// RemoveMount.

	// Remove the workspace mount.
	srv.RemoveMount("workspace")

	// Verify UMOUNT_NOTIFY arrives.
	if err := strm.Send(&V86FsMessage{
		Tag:  nextTag(),
		Body: &V86FsMessage_StatfsRequest{StatfsRequest: &V86FsStatfsRequest{}},
	}); err != nil {
		t.Fatal(err.Error())
	}

	// Read messages until the UMOUNT_NOTIFY arrives.
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

	// Check ListMounts is empty after the removal.
	mounts = srv.ListMounts()
	if len(mounts) != 0 {
		t.Fatalf("expected 0 mounts after RemoveMount, got %d", len(mounts))
	}
}

// TestRelayReadCapsSize tests a read larger than maxReadSize returns a short
// read instead of allocating the guest-requested size.
func TestRelayReadCapsSize(t *testing.T) {

	// Open the relay stream and build a tag counter.
	ctx := context.Background()

	// Set up the context and a large file in a billy filesystem.
	bfs := memfs.New()
	data := bytes.Repeat([]byte("0123456789abcdef"), (maxReadSize*2)/16)
	f, err := bfs.Create("big.bin")
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err.Error())
	}
	if err := f.Close(); err != nil {
		t.Fatal(err.Error())
	}

	// Open the FSHandle, the server client, and the relay stream.
	h, err := unixfs.NewFSHandle(unixfs_billy.NewBillyFSCursor(bfs, ""))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(h.Release)
	client := buildMultiMountServer(t, ctx, h, h)
	strm, err := client.RelayV86Fs(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strm.Close()

	// Mount the workspace, look up the file, and open it.
	rootID := sendRecv(t, strm, &V86FsMessage{
		Tag:  1,
		Body: &V86FsMessage_MountRequest{MountRequest: &V86FsMountRequest{Name: "workspace"}},
	}).GetMountReply().GetRootInodeId()
	fileID := sendRecv(t, strm, &V86FsMessage{
		Tag:  2,
		Body: &V86FsMessage_LookupRequest{LookupRequest: &V86FsLookupRequest{ParentId: rootID, Name: "big.bin"}},
	}).GetLookupReply().GetInodeId()
	handleID := sendRecv(t, strm, &V86FsMessage{
		Tag:  3,
		Body: &V86FsMessage_OpenRequest{OpenRequest: &V86FsOpenRequest{InodeId: fileID}},
	}).GetOpenReply().GetHandleId()

	// Read with oversized sizes and check each returns a capped short read.
	for i, size := range []uint32{maxReadSize + 1, ^uint32(0)} {
		readReply := sendRecv(t, strm, &V86FsMessage{
			Tag: uint32(4 + i), //nolint:gosec
			Body: &V86FsMessage_ReadRequest{ReadRequest: &V86FsReadRequest{
				HandleId: handleID,
				Offset:   16,
				Size:     size,
			}},
		}).GetReadReply()
		if readReply == nil || readReply.GetStatus() != 0 {
			t.Fatalf("size %d: read failed: %v", size, readReply)
		}
		if !bytes.Equal(readReply.GetData(), data[16:16+maxReadSize]) {
			t.Fatalf("size %d: read %d bytes, want the first %d", size, len(readReply.GetData()), maxReadSize)
		}
	}
}
