package unixfs

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// Truncate resizes the committed bytes.
func (f *fakeFileOps) Truncate(ctx context.Context, nsize uint64, ts time.Time) error {
	// Grow or shrink the file to nsize.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if int(nsize) > len(f.data) {
		f.data = append(f.data, make([]byte, int(nsize)-len(f.data))...)
	}
	f.data = f.data[:nsize]
	f.writes++
	return nil
}

// Remove accepts the removal.
func (f *fakeFileOps) Remove(ctx context.Context, names []string, ts time.Time) error {
	return nil
}

// openBufferedFile opens a handle at a file whose commits are recorded and
// writes "abc" to the file, keeping the write buffered.
func openBufferedFile(t *testing.T) (*FSHandle, *fakeFileOps) {
	// Open the file.
	ops := &fakeFileOps{}
	h, err := NewFSHandle(&fakeCursor{ops: ops})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.Release)

	// Buffer a write and check it did not commit.
	if err := h.WriteAt(context.Background(), 0, []byte("abc"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if data, writes := ops.committed(); writes != 0 {
		t.Fatalf("committed %q in %d writes before the trigger", data, writes)
	}
	return h, ops
}

// TestWriteBehindCoalescesSmallWrites tests that many small sequential writes
// reach the file as a few writes of the optimal write size.
func TestWriteBehindCoalescesSmallWrites(t *testing.T) {
	// Write 160 bytes one byte at a time to a file with a 16 byte extent.
	ctx := context.Background()
	ops := &fakeFileOps{}
	h, err := NewFSHandle(&fakeCursor{ops: ops})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Release()
	want := bytes.Repeat([]byte("0123456789abcdef"), 10)
	for i := range want {
		if err := h.WriteAt(ctx, int64(i), want[i:i+1], time.Now()); err != nil {
			t.Fatal(err)
		}
	}

	// The file received ten full extents.
	data, writes := ops.committed()
	if writes != 10 || data != string(want) {
		t.Fatalf("committed %d writes of %d bytes, want 10 writes of %d", writes, len(data), len(want))
	}
}

// TestWriteBehindFlushTriggers tests that each operation that needs the
// buffered bytes commits them as one write.
func TestWriteBehindFlushTriggers(t *testing.T) {
	ctx := context.Background()
	triggers := []struct {
		name   string
		writes int
		run    func(t *testing.T, h *FSHandle)
	}{
		{"read", 1, func(t *testing.T, h *FSHandle) {
			buf := make([]byte, 3)
			if _, err := h.ReadAt(ctx, 0, buf); err != nil {
				t.Fatal(err)
			}
			if string(buf) != "abc" {
				t.Fatalf("read %q, want abc", buf)
			}
		}},
		{"discontiguous write", 1, func(t *testing.T, h *FSHandle) {
			if err := h.WriteAt(ctx, 8, []byte("xyz"), time.Now()); err != nil {
				t.Fatal(err)
			}
		}},
		{"truncate", 2, func(t *testing.T, h *FSHandle) {
			if err := h.Truncate(ctx, 2, time.Now()); err != nil {
				t.Fatal(err)
			}
		}},
		{"sync", 1, func(t *testing.T, h *FSHandle) {
			if err := h.Sync(ctx); err != nil {
				t.Fatal(err)
			}
		}},
		{"rename", 1, func(t *testing.T, h *FSHandle) {
			// Rename to another location, which the fake rejects after the sync.
			dest, err := NewFSHandle(&fakeCursor{ops: &fakeOps{}})
			if err != nil {
				t.Fatal(err)
			}
			defer dest.Release()
			err = h.Rename(ctx, dest, "moved.txt", time.Now())
			if !errors.Is(err, unixfs_errors.ErrCrossFsRename) {
				t.Fatalf("rename error %v, want %v", err, unixfs_errors.ErrCrossFsRename)
			}
		}},
		{"remove", 1, func(t *testing.T, h *FSHandle) {
			if err := h.Remove(ctx, []string{"gone.txt"}, time.Now()); err != nil {
				t.Fatal(err)
			}
		}},
		{"last release", 1, func(t *testing.T, h *FSHandle) {
			h.Release()
		}},
	}
	for _, trigger := range triggers {
		t.Run(trigger.name, func(t *testing.T) {
			// Fire the trigger on a buffered extent.
			h, ops := openBufferedFile(t)
			trigger.run(t, h)

			// The extent committed as one write, plus the trigger's own.
			if _, writes := ops.committed(); writes != trigger.writes {
				t.Fatalf("committed %d writes, want %d", writes, trigger.writes)
			}
		})
	}
}

// TestWriteBehindIdleDeadline tests that the deadline commits an extent once
// the writer pauses, and that each write moves the deadline.
func TestWriteBehindIdleDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Buffer a write, then keep writing inside the deadline.
		ctx := t.Context()
		h, ops := openBufferedFile(t)
		time.Sleep(fsInodeIdleFlush * 3 / 5)
		if err := h.WriteAt(ctx, 3, []byte("def"), time.Now()); err != nil {
			t.Fatal(err)
		}
		time.Sleep(fsInodeIdleFlush * 3 / 5)
		synctest.Wait()
		if data, writes := ops.committed(); writes != 0 {
			t.Fatalf("committed %q in %d writes before the deadline", data, writes)
		}

		// The pause commits both writes as one.
		time.Sleep(fsInodeIdleFlush * 2 / 5)
		synctest.Wait()
		if data, writes := ops.committed(); writes != 1 || data != "abcdef" {
			t.Fatalf("committed %q in %d writes, want abcdef in 1", data, writes)
		}

		// The deadline fired once: later time commits nothing more.
		time.Sleep(fsInodeIdleFlush * 3)
		synctest.Wait()
		if _, writes := ops.committed(); writes != 1 {
			t.Fatalf("committed %d writes, want 1", writes)
		}
	})
}
