//go:build linux

package boot

import (
	"bytes"
	"errors"
	"testing"
	"time"

	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/sirupsen/logrus"
)

// TestRootReadOnly tries to change the served image through its own handle.
func TestRootReadOnly(t *testing.T) {
	// Build an in-memory image with a small input instead of an executable.
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

	// A write through the served tree is refused, when written or at the
	// latest when synced, since the handle buffers small writes.
	file, err := r.Handle().Lookup(ctx, "workload")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Release()
	writeErr := file.WriteAt(ctx, 0, []byte("modified"), time.Now())
	if err := errors.Join(writeErr, file.Sync(ctx)); !errors.Is(err, unixfs_errors.ErrReadOnly) {
		t.Fatalf("write to the served image returned %v", err)
	}

	// The image still holds the original bytes.
	if err := r.VerifyImage(ctx, bytes.NewBufferString("original")); err != nil {
		t.Fatal(err)
	}
}
