//go:build linux

package boot

import (
	"bytes"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// TestRootPinsRevision tries to change the served image through its live World.
func TestRootPinsRevision(t *testing.T) {
	// Build a frozen image with a small input instead of an executable.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	r, err := NewRoot(ctx, le, bytes.NewBufferString("original"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := r.Close(); err != nil {
			t.Error(err)
		}
	})

	// Advance the live World through its ordinary writer after pinning.
	ws := world.NewEngineWorldState(r.engine, true)
	fsc, _ := unixfs_world.NewFSCursorWithWriterContext(ctx, le, ws, rootKey, unixfs_world.FSType_FSType_FS_NODE, r.volume.GetPeerID())
	writable, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err)
	}
	defer writable.Release()
	file, err := writable.Lookup(ctx, "workload")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Release()
	if err := file.WriteAt(ctx, 0, []byte("modified"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := file.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	// The snapshot must still read the original bytes after live publication.
	frozen, err := r.handle.Lookup(ctx, "workload")
	if err != nil {
		t.Fatal(err)
	}
	defer frozen.Release()
	data := make([]byte, 8)
	if n, err := frozen.ReadAt(ctx, 0, data); err != nil || n != 8 || string(data) != "original" {
		t.Fatalf("frozen read = %q, %d, %v", data, n, err)
	}
}
