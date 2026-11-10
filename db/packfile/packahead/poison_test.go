package packahead

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// failingSync is a SyncData that fails the sync numbered fail, counting from
// one, and runs every other.
type failingSync struct {
	n    atomic.Int64
	fail int64
}

// sync is the SyncData hook.
func (s *failingSync) sync(f *os.File) error {
	if s.n.Add(1) == s.fail {
		return syscall.EIO
	}
	return f.Sync()
}

// TestPackAheadSyncPoisons fails the second sync of a pack, after a first
// batch synced: the pack is poisoned, so it is never finalized, none of its
// values reports durable, and a put of its durable value writes a new copy.
// The next Open recovers the poisoned pack from its file.
func TestPackAheadSyncPoisons(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	hook := &failingSync{fail: 2}
	l, err := Open(ctx, dir, Options{SyncData: hook.sync, Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// The first batch syncs; the second's sync fails and poisons the pack.
	a, b := testBlock(t, 1, 3000), testBlock(t, 2, 3000)
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		l.opts.Linger = 0
	})
	if err := l.Append(ctx, []Block{a}); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(ctx, []Block{b}); err == nil {
		t.Fatal("append succeeded on a failed sync")
	}
	if l.HasDurable(a.Key) || l.HasDurable(b.Key) {
		t.Fatal("a poisoned pack's value reports durable")
	}
	if got, ok, err := l.Get(a.Key); err != nil || !ok || !bytes.Equal(got, a.Value) {
		t.Fatalf("synced value of a poisoned pack does not read: %v %v", ok, err)
	}

	// A put of the durable value writes a new copy, which a later pack
	// finalizes; the poisoned pack stays unfinalized.
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		l.opts.Linger = 10 * time.Millisecond
	})
	if err := l.Append(ctx, []Block{a}); err != nil {
		t.Fatal(err)
	}
	if !l.HasDurable(a.Key) {
		t.Fatal("value stored again is not durable")
	}
	infos := waitFinalized(t, l, 1)
	if slices.ContainsFunc(infos, func(i PackInfo) bool { return i.Seq == 1 }) {
		t.Fatal("poisoned pack finalized")
	}
	if st := l.GetStats(); st.Poisoned != 1 || st.SyncFailures != 1 {
		t.Fatalf("stats: %+v", st)
	}
	if _, err := os.Stat(filepath.Join(dir, openName(1))); err != nil {
		t.Fatalf("poisoned pack file: %v", err)
	}

	// The next Open recovers the poisoned pack.
	l.Close()
	l2, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	if got, ok, err := l2.Get(a.Key); err != nil || !ok || !bytes.Equal(got, a.Value) {
		t.Fatalf("value after reopen: %v %v", ok, err)
	}
}

// TestPackAheadWriteBatchFailedPack writes a batch to a pack whose sync
// failed after the batch was planned: the batch is refused and not located.
func TestPackAheadWriteBatchFailedPack(t *testing.T) {
	ctx := t.Context()
	l, err := Open(ctx, t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	a := testBlock(t, 1, 3000)
	release, err := l.writeMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var p *pack
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		p = l.open
		p.poisoned = true
		l.failLocked(p, syscall.EIO)
	})
	_, werr := l.writeBatch(p, []Block{a})
	release()
	if werr == nil {
		t.Fatal("batch written to a failed pack")
	}
	if l.Has(a.Key) {
		t.Fatal("batch of a failed pack located")
	}
}
