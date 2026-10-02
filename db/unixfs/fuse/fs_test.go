//go:build linux

package fuse

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	unixfs_world_testbed "github.com/s4wave/spacewave/db/unixfs/world/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

// TestMountReadOnlyMapsFiles mounts a tree read-only through the kernel and
// checks that a file memory-maps with its contents and that writes fail.
func TestMountReadOnlyMapsFiles(t *testing.T) {
	// Build three distinct pages of data.
	data := bytes.Repeat([]byte("abcdefgh"), 3*4096/8)
	for i := range data {
		data[i] += byte(i / 4096)
	}

	// Mount a tree holding the data read-only.
	mountPath := mountTestFile(t, context.Background(), data)

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

// TestMountLogsBlockReads checks that reads through a mount log each block
// they reach, once, to the access logger of the tree's cursor context.
func TestMountLogsBlockReads(t *testing.T) {
	// Mount a tree holding a file of several chunks, logging block reads.
	data := make([]byte, 4<<20)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	logger := block.NewAccessLogger()
	mountPath := mountTestFile(t, block.WithAccessLogger(context.Background(), logger), data)

	// Read the file through the kernel.
	read, err := io.ReadAll(openMounted(t, filepath.Join(mountPath, "image.bin")))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(read, data) {
		t.Fatal("read contents differ from the written file")
	}

	// The log holds each block once, and its blocks hold at least the data.
	accesses := logger.Snapshot().GetAccesses()
	seen := make(map[string]bool, len(accesses))
	var total int
	for _, access := range accesses {
		key := access.GetRef().MarshalString()
		if seen[key] {
			t.Fatalf("block %s logged twice", key)
		}
		seen[key] = true
		total += int(access.GetSize())
	}
	if total < len(data) {
		t.Fatalf("logged %d blocks of %d bytes for a %d byte file", len(accesses), total, len(data))
	}
}

// mountTestFile mounts read-only a new tree holding image.bin with data,
// serving it until the test ends. The mounted cursor reads with readCtx, so its
// access logger, if any, sees only reads through the mount.
func mountTestFile(t *testing.T, readCtx context.Context, data []byte) string {
	// Require a kernel FUSE device.
	if _, err := os.Stat("/dev/fuse"); err != nil {
		t.Skip("no /dev/fuse")
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
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

	// Open a cursor that reads the tree with readCtx.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	readCursor := unixfs_world.NewFSCursorWithContext(
		readCtx,
		le,
		ws,
		"test/fuse-read-only",
		unixfs_world.FSType_FSType_FS_NODE,
		nil,
		false,
	)
	readHandle, err := unixfs.NewFSHandle(readCursor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(readHandle.Release)

	// Mount the tree read-only and serve it until the test ends.
	mountPath := filepath.Join(t.TempDir(), "mnt")
	if err := os.Mkdir(mountPath, 0o755); err != nil {
		t.Fatal(err)
	}
	rootFS, err := Mount(ctx, le, mountPath, readHandle, false, true, nil)
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
	return mountPath
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
