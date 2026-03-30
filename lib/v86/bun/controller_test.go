package forge_lib_v86_bun

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_billy "github.com/aperturerobotics/hydra/unixfs/billy"
	v86fs "github.com/aperturerobotics/hydra/unixfs/v86fs"
	"github.com/aperturerobotics/starpc/srpc"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/sirupsen/logrus"
)

// sendRecv sends a message and returns the reply, skipping notifications (tag=0).
func sendRecv(t *testing.T, strm v86fs.SRPCV86FsService_RelayV86FsClient, msg *v86fs.V86FsMessage) *v86fs.V86FsMessage {
	t.Helper()
	if err := strm.Send(msg); err != nil {
		t.Fatal(err.Error())
	}
	for {
		reply, err := strm.Recv()
		if err != nil {
			t.Fatal(err.Error())
		}
		if reply.GetTag() == 0 {
			continue // skip notifications
		}
		return reply
	}
}

// TestInitOutputMount tests that initOutputMount creates a writable FSHandle
// backed by a block transaction. Writes persist to the block store and
// data is readable from a fresh BillyFS view on the same handle.
func TestInitOutputMount(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	handle, err := initOutputMount(ctx, oc)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer handle.Release()

	// Create a file with content.
	data := []byte("hello from output mount")
	err = handle.MknodWithContent(
		ctx,
		"test.txt",
		unixfs.NewFSCursorNodeType_File(),
		int64(len(data)),
		bytes.NewReader(data),
		0o644,
		time.Now(),
	)
	if err != nil {
		t.Fatalf("mknod with content: %v", err)
	}

	// Verify root ref is non-empty after write.
	ref := oc.GetRef()
	if ref.GetRootRef().GetEmpty() {
		t.Fatal("expected non-empty root ref after writes")
	}

	// Read back via BillyFS (established pattern from hydra block FS tests).
	bfs := unixfs_billy.NewBillyFS(ctx, handle, "", time.Now())
	readData, err := billy_util.ReadFile(bfs, "test.txt")
	if err != nil {
		t.Fatalf("read via billy: %v", err)
	}
	if !bytes.Equal(readData, data) {
		t.Fatalf("expected %q, got %q", string(data), string(readData))
	}
}

// TestOutputMountViaSRPC tests the output mount through the v86fs SRPC
// relay, verifying that writes through SRPC persist to the block store.
func TestOutputMountViaSRPC(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	handle, err := initOutputMount(ctx, oc)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer handle.Release()

	// Set up v86fs server with the output mount.
	srv := v86fs.NewServer(nil)
	srv.AddMount("output", "/output", handle)

	mux := srpc.NewMux()
	if err := v86fs.SRPCRegisterV86FsService(mux, srv); err != nil {
		t.Fatal(err.Error())
	}
	server := srpc.NewServer(mux)
	pipe := srpc.NewServerPipe(server)
	client := srpc.NewClient(pipe)
	v86fsClient := v86fs.NewSRPCV86FsServiceClient(client)

	strm, err := v86fsClient.RelayV86Fs(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer strm.Close()

	// MOUNT output
	reply := sendRecv(t, strm, &v86fs.V86FsMessage{
		Tag:  1,
		Body: &v86fs.V86FsMessage_MountRequest{MountRequest: &v86fs.V86FsMountRequest{Name: "output"}},
	})
	mountReply := reply.GetMountReply()
	if mountReply == nil || mountReply.GetStatus() != 0 {
		t.Fatalf("mount failed: %v", reply.GetBody())
	}
	rootID := mountReply.GetRootInodeId()

	// CREATE test.txt
	reply = sendRecv(t, strm, &v86fs.V86FsMessage{
		Tag: 2,
		Body: &v86fs.V86FsMessage_CreateRequest{CreateRequest: &v86fs.V86FsCreateRequest{
			ParentId: rootID,
			Name:     "test.txt",
			Mode:     0o100644, // S_IFREG | 0644
		}},
	})
	createReply := reply.GetCreateReply()
	if createReply == nil || createReply.GetStatus() != 0 {
		t.Fatalf("create failed: %v", reply.GetBody())
	}
	fileID := createReply.GetInodeId()

	// WRITE data
	fileData := []byte("hello\n")
	reply = sendRecv(t, strm, &v86fs.V86FsMessage{
		Tag: 3,
		Body: &v86fs.V86FsMessage_WriteRequest{WriteRequest: &v86fs.V86FsWriteRequest{
			InodeId: fileID,
			Offset:  0,
			Data:    fileData,
		}},
	})
	writeReply := reply.GetWriteReply()
	if writeReply == nil || writeReply.GetStatus() != 0 {
		t.Fatalf("write failed: %v", reply.GetBody())
	}
	if writeReply.GetBytesWritten() != uint32(len(fileData)) {
		t.Fatalf("expected %d bytes written, got %d", len(fileData), writeReply.GetBytesWritten())
	}

	// Verify root ref is non-empty (data persisted to blocks).
	ref := oc.GetRef()
	if ref.GetRootRef().GetEmpty() {
		t.Fatal("expected non-empty root ref after SRPC writes")
	}

	// Verify file content via BillyFS view on the handle.
	bfs := unixfs_billy.NewBillyFS(ctx, handle, "", time.Now())
	readData, err := billy_util.ReadFile(bfs, "test.txt")
	if err != nil {
		t.Fatalf("read via billy after SRPC write: %v", err)
	}
	if !bytes.Equal(readData, fileData) {
		t.Fatalf("expected %q, got %q", string(fileData), string(readData))
	}
}
