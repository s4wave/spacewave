//go:build linux

package fuse

import (
	"context"
	"testing"

	"bazil.org/fuse"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	unixfs_world_testbed "github.com/s4wave/spacewave/db/unixfs/world/testbed"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

// TestInodeOpenUsesSynchronousDirectIO checks the existing-file open path.
func TestInodeOpenUsesSynchronousDirectIO(t *testing.T) {
	inode := &Inode{}
	request := &fuse.OpenRequest{Flags: fuse.OpenReadWrite}
	response := &fuse.OpenResponse{}

	handle, err := inode.Open(context.Background(), request, response)
	if err != nil {
		t.Fatal(err)
	}
	opened, ok := handle.(*Handle)
	if !ok {
		t.Fatalf("expected *Handle, got %T", handle)
	}
	if response.Flags&fuse.OpenDirectIO == 0 {
		t.Fatal("expected OpenDirectIO response flag")
	}
	if opened.openFlags&fuse.OpenSync == 0 {
		t.Fatal("expected synchronous Spacewave writes")
	}
}

// TestInodeCreateUsesSynchronousDirectIO checks the create-and-open path.
func TestInodeCreateUsesSynchronousDirectIO(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err)
	}
	rootHandle, err := unixfs_world_testbed.InitTestbed(wtb, "test/fuse-create", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rootHandle.Release)
	rootFS := &RootFS{ctx: ctx, ctxCancel: cancel, le: le}
	inode := NewInode(rootFS, nil, rootHandle)
	request := &fuse.CreateRequest{
		Name:  "created.txt",
		Flags: fuse.OpenReadWrite,
		Mode:  0o644,
	}
	response := &fuse.CreateResponse{}

	created, handle, err := inode.Create(ctx, request, response)
	if err != nil {
		t.Fatal(err)
	}
	createdInode, ok := created.(*Inode)
	if !ok {
		t.Fatalf("expected *Inode, got %T", created)
	}
	t.Cleanup(createdInode.h.Release)
	opened, ok := handle.(*Handle)
	if !ok {
		t.Fatalf("expected *Handle, got %T", handle)
	}
	t.Cleanup(func() {
		_ = opened.Release(context.Background(), &fuse.ReleaseRequest{})
	})
	if response.OpenResponse.Flags&fuse.OpenDirectIO == 0 {
		t.Fatal("expected OpenDirectIO response flag")
	}
	if opened.openFlags&fuse.OpenSync == 0 {
		t.Fatal("expected synchronous Spacewave writes")
	}

	writeResponse := &fuse.WriteResponse{}
	if err := opened.Write(ctx, &fuse.WriteRequest{Offset: 0, Data: []byte("created synchronously")}, writeResponse); err != nil {
		t.Fatal(err)
	}
	if writeResponse.Size != len("created synchronously") {
		t.Fatalf("expected synchronous write size %d, got %d", len("created synchronously"), writeResponse.Size)
	}
	if err := opened.Release(ctx, &fuse.ReleaseRequest{}); err != nil {
		t.Fatal(err)
	}
}
