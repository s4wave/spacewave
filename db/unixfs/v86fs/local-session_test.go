package unixfs_v86fs

import (
	"bytes"
	"context"
	"testing"
)

func TestLocalSessionMountWriteRead(t *testing.T) {
	ctx := context.Background()
	handle := newBillyHandle(t)
	srv := NewServer(nil)
	srv.AddMount("workspace", "/mnt/workspace", handle)

	sess := NewLocalSession(ctx, srv)
	defer sess.Close()

	notifications := sess.DrainNotifications()
	if len(notifications) != 1 || notifications[0].GetMountNotify().GetName() != "workspace" {
		t.Fatalf("seed notifications = %#v", notifications)
	}

	mountReply := handleLocal(t, sess, &V86FsMessage{
		Tag: 1,
		Body: &V86FsMessage_MountRequest{
			MountRequest: &V86FsMountRequest{Name: "workspace"},
		},
	}).GetMountReply()
	if mountReply.GetStatus() != 0 || mountReply.GetRootInodeId() == 0 {
		t.Fatalf("mount reply = %#v", mountReply)
	}

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

	openReply := handleLocal(t, sess, &V86FsMessage{
		Tag: 4,
		Body: &V86FsMessage_OpenRequest{
			OpenRequest: &V86FsOpenRequest{InodeId: createReply.GetInodeId()},
		},
	}).GetOpenReply()
	if openReply.GetStatus() != 0 || openReply.GetHandleId() == 0 {
		t.Fatalf("open reply = %#v", openReply)
	}

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
