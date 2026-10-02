//go:build linux

package fuse

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/pkg/errors"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world_testbed "github.com/s4wave/spacewave/db/unixfs/world/testbed"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

// TestMountReadOnlyMapsFiles mounts a tree read-only through the kernel and
// checks that a file memory-maps with its contents and that writes fail.
func TestMountReadOnlyMapsFiles(t *testing.T) {
	// Require a kernel FUSE device.
	if _, err := os.Stat("/dev/fuse"); err != nil {
		t.Skip("no /dev/fuse")
	}
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
	rootHandle, err := unixfs_world_testbed.InitTestbed(wtb, "test/fuse-read-only", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rootHandle.Release)

	// Build three distinct pages of data.
	data := bytes.Repeat([]byte("abcdefgh"), 3*4096/8)
	for i := range data {
		data[i] += byte(i / 4096)
	}

	// Write the data to a new file.
	ts := time.Now()
	if err := rootHandle.Mknod(ctx, true, []string{"image.bin"}, unixfs.NewFSCursorNodeType_File(), 0o755, ts); err != nil {
		t.Fatal(err)
	}
	fileHandle, err := rootHandle.Lookup(ctx, "image.bin")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fileHandle.Release)
	if err := fileHandle.WriteAt(ctx, 0, data, ts); err != nil {
		t.Fatal(err)
	}
	if err := fileHandle.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	// Mount the tree read-only and serve it until the test ends.
	mountPath := filepath.Join(t.TempDir(), "mnt")
	if err := os.Mkdir(mountPath, 0o755); err != nil {
		t.Fatal(err)
	}
	rootFS, err := Mount(ctx, le, mountPath, rootHandle, false, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() {
		served <- rootFS.Serve()
	}()
	t.Cleanup(func() {
		if err := Unmount(mountPath); err != nil {
			t.Error(err)
		}
		<-served
		rootFS.Close()
	})

	// Map the file shared, which a direct IO open cannot serve. Populate the
	// mapping inside the mmap call: a page fault on this mount from a running
	// goroutine blocks a thread the Go scheduler cannot stop, so a garbage
	// collection during the fault would also stop the server answering it.
	file := openMounted(t, filepath.Join(mountPath, "image.bin"))
	mapped, err := syscall.Mmap(int(file.Fd()), 0, len(data), syscall.PROT_READ, syscall.MAP_SHARED|syscall.MAP_POPULATE)
	if err != nil {
		t.Fatalf("mmap: %v", err)
	}
	defer syscall.Munmap(mapped)
	if !bytes.Equal(mapped, data) {
		t.Fatal("mapped contents differ from the written file")
	}

	// The kernel rejects writes to the read-only mount.
	err = os.WriteFile(filepath.Join(mountPath, "image.bin"), []byte("x"), 0o644)
	if !errors.Is(err, syscall.EROFS) {
		t.Fatalf("expected EROFS, got %v", err)
	}
}

// openMounted opens a file on a mount this process serves, closing it when the
// test ends. os.Open would add the file to the Go netpoller, and the kernel
// asks the server to poll it from a thread the scheduler cannot stop, so a
// garbage collection during that request would also stop the server
// answering it. A blocking descriptor from os.NewFile stays out of the poller.
func openMounted(t *testing.T, path string) *os.File {
	// Open a blocking descriptor.
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}

	// Wrap it without the poller.
	file := os.NewFile(uintptr(fd), path)
	t.Cleanup(func() { file.Close() })
	return file
}
