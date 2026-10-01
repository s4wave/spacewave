//go:build linux

package fuse

import (
	"context"
	"io"
	"testing"

	"bazil.org/fuse"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	unixfs_world_testbed "github.com/s4wave/spacewave/db/unixfs/world/testbed"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

// TestInodeOpenUsesSynchronousDirectIO checks the existing-file open path.
func TestInodeOpenUsesSynchronousDirectIO(t *testing.T) {
	// Open an existing inode.
	inode := &Inode{}
	request := &fuse.OpenRequest{Flags: fuse.OpenReadWrite}
	response := &fuse.OpenResponse{}
	handle, err := inode.Open(context.Background(), request, response)
	if err != nil {
		t.Fatal(err)
	}

	// The open returns a Handle that bypasses the page cache.
	if _, ok := handle.(*Handle); !ok {
		t.Fatalf("expected *Handle, got %T", handle)
	}
	if response.Flags&fuse.OpenDirectIO == 0 {
		t.Fatal("expected OpenDirectIO response flag")
	}
}

// TestInodeCreateUsesSynchronousDirectIO checks the create-and-open path.
func TestInodeCreateUsesSynchronousDirectIO(t *testing.T) {
	// Build a store.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Initialize the filesystem in a World.
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

	// Create a file through the root inode.
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

	// Create returns the new Inode.
	createdInode, ok := created.(*Inode)
	if !ok {
		t.Fatalf("expected *Inode, got %T", created)
	}
	t.Cleanup(createdInode.h.Release)

	// The create returns a Handle that bypasses the page cache.
	opened, ok := handle.(*Handle)
	if !ok {
		t.Fatalf("expected *Handle, got %T", handle)
	}
	if response.Flags&fuse.OpenDirectIO == 0 {
		t.Fatal("expected OpenDirectIO response flag")
	}

	// The write is committed to the file before Write returns.
	data := []byte("created synchronously")
	writeResponse := &fuse.WriteResponse{}
	if err := opened.Write(ctx, &fuse.WriteRequest{Offset: 0, Data: data}, writeResponse); err != nil {
		t.Fatal(err)
	}
	if writeResponse.Size != len(data) {
		t.Fatalf("expected write size %d, got %d", len(data), writeResponse.Size)
	}
	got := make([]byte, len(data))
	if _, err := createdInode.h.ReadAt(ctx, 0, got); err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if string(got) != string(data) {
		t.Fatalf("expected %q, got %q", data, got)
	}
}
