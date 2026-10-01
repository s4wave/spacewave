package unixfs_v86fs

import (
	"bytes"
	"context"
	"testing"
)

func TestLocalSessionMountWriteRead(t *testing.T) {
	// Set up the test context, a writable handle, and a server with the workspace mount.
	ctx := context.Background()
	handle := newBillyHandle(t)
	srv := NewServer(nil, nil)
	srv.AddMount("workspace", "/mnt/workspace", handle)

	// Open a local session against the server.
	sess := NewLocalSession(ctx, srv)
	defer sess.Close()

	// Drain the seeded mount notification.
	notifications := sess.DrainNotifications()
	if len(notifications) != 1 || notifications[0].GetMountNotify().GetName() != "workspace" {
		t.Fatalf("seed notifications = %#v", notifications)
	}

	// Mount the workspace and check the reply.
	mountReply := handleLocal(t, sess, &V86FsMessage{
		Tag: 1,
		Body: &V86FsMessage_MountRequest{
			MountRequest: &V86FsMountRequest{Name: "workspace"},
		},
	}).GetMountReply()
	if mountReply.GetStatus() != 0 || mountReply.GetRootInodeId() == 0 {
		t.Fatalf("mount reply = %#v", mountReply)
	}

	// Create a file and check the reply.
	createReply := handleLocal(t, sess, &V86FsMessage{
		Tag: 2,
		Body: &V86FsMessage_CreateRequest{
			CreateRequest: &V86FsCreateRequest{
				ParentId: mountReply.GetRootInodeId(),
				Name:     "out.txt",
				Mode:     0o644,
			},
		},
	}).GetCreateReply()
	if createReply.GetStatus() != 0 || createReply.GetInodeId() == 0 {
		t.Fatalf("create reply = %#v", createReply)
	}

	// Write data to the file and check the reply.
	writeReply := handleLocal(t, sess, &V86FsMessage{
		Tag: 3,
		Body: &V86FsMessage_WriteRequest{
			WriteRequest: &V86FsWriteRequest{
				InodeId: createReply.GetInodeId(),
				Data:    []byte("hello"),
			},
		},
	}).GetWriteReply()
	if writeReply.GetStatus() != 0 || writeReply.GetBytesWritten() != 5 {
		t.Fatalf("write reply = %#v", writeReply)
	}

	// Open the file and check the reply.
	openReply := handleLocal(t, sess, &V86FsMessage{
		Tag: 4,
		Body: &V86FsMessage_OpenRequest{
			OpenRequest: &V86FsOpenRequest{InodeId: createReply.GetInodeId()},
		},
	}).GetOpenReply()
	if openReply.GetStatus() != 0 || openReply.GetHandleId() == 0 {
		t.Fatalf("open reply = %#v", openReply)
	}

	// Read the file back and check the data.
	readReply := handleLocal(t, sess, &V86FsMessage{
		Tag: 5,
		Body: &V86FsMessage_ReadRequest{
			ReadRequest: &V86FsReadRequest{
				HandleId: openReply.GetHandleId(),
				Size:     5,
			},
		},
	}).GetReadReply()
	if readReply.GetStatus() != 0 || !bytes.Equal(readReply.GetData(), []byte("hello")) {
		t.Fatalf("read reply = %#v", readReply)
	}
}

func handleLocal(t *testing.T, sess *LocalSession, msg *V86FsMessage) *V86FsMessage {
	t.Helper()
	reply, err := sess.HandleMessage(context.Background(), msg)
	if err != nil {
		t.Fatal(err.Error())
	}
	return reply
}
