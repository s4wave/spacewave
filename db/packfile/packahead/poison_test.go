package packahead

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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

// TestPackAheadSyncSalvages fails the second sync of a pack, after a first
// batch synced: the pack is poisoned, and the maintainer salvages its synced
// batch into a fresh file, which takes its place finalized. The failed batch
// is gone, the file whose sync failed is never finalized, and once the copy
// uploads nothing holds the poisoned pack's values back.
func TestPackAheadSyncSalvages(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	hook := &failingSync{fail: 2}
	l, err := Open(ctx, dir, Options{SyncData: hook.sync})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// The first batch syncs; the second's sync fails and poisons the pack.
	a, b := testBlock(t, 1, 3000), testBlock(t, 2, 3000)
	if err := l.Append(ctx, []Block{a}); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(filepath.Join(dir, openName(1)))
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Append(ctx, []Block{b}); err == nil {
		t.Fatal("append succeeded on a failed sync")
	}

	// The salvaged copy is finalized under the same seq, in a new file.
	infos := waitFinalized(t, l, 1)
	if infos[0].Seq != 1 || infos[0].Blocks != 1 {
		t.Fatalf("salvaged pack: %+v", infos[0])
	}
	after, err := os.Stat(infos[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("the pack whose sync failed was finalized in place")
	}
	if _, err := os.Stat(filepath.Join(dir, openName(1))); !os.IsNotExist(err) {
		t.Fatalf("poisoned pack file: %v", err)
	}
	if !l.HasDurable(a.Key) || l.Has(b.Key) {
		t.Fatal("salvage kept the wrong batches")
	}
	if got, ok, err := l.Get(a.Key); err != nil || !ok || !bytes.Equal(got, a.Value) {
		t.Fatalf("salvaged value does not read: %v %v", ok, err)
	}
	if st := l.GetStats(); st.Poisoned != 0 || st.Salvaged != 1 || st.SyncFailures != 1 {
		t.Fatalf("stats: %+v", st)
	}

	// Once the copy uploads, no pack holds a value back.
	l.MarkUploaded(1)
	if l.HoldsBefore(time.Now()) {
		t.Fatal("the salvaged pack still holds values back after its upload")
	}
}

// TestPackAheadOpenSalvages reopens a Log whose open pack was left
// unfinalized: Open copies its good batches into a fresh file and finalizes
// that, never the file a failed sync may have left in the page cache only.
func TestPackAheadOpenSalvages(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	l, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	a := testBlock(t, 1, 3000)
	if err := l.Append(ctx, []Block{a}); err != nil {
		t.Fatal(err)
	}
	l.Close()
	before, err := os.Stat(filepath.Join(dir, openName(1)))
	if err != nil {
		t.Fatal(err)
	}

	// Open finalizes a copy and removes the unfinalized file.
	l2, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	infos, _ := l2.GetPacks()
	if len(infos) != 1 || infos[0].Seq != 1 {
		t.Fatalf("packs after open: %+v", infos)
	}
	after, err := os.Stat(infos[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("Open finalized an unfinalized pack in place")
	}
	if _, err := os.Stat(filepath.Join(dir, openName(1))); !os.IsNotExist(err) {
		t.Fatalf("unfinalized pack file: %v", err)
	}
	if got, ok, err := l2.Get(a.Key); err != nil || !ok || !bytes.Equal(got, a.Value) {
		t.Fatalf("value after open: %v %v", ok, err)
	}
}

// TestPackAheadFinalizeFailSalvages fails the index sync of a finalize: the
// pack is poisoned and salvaged into a fresh file, not finalized in place by
// a retry.
func TestPackAheadFinalizeFailSalvages(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	hook := &failingSync{fail: 2}
	l, err := Open(ctx, dir, Options{SyncData: hook.sync, Linger: 200 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// The batch syncs, and the index sync of its finalize fails.
	a := testBlock(t, 1, 3000)
	if err := l.Append(ctx, []Block{a}); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(filepath.Join(dir, openName(1)))
	if err != nil {
		t.Fatal(err)
	}
	infos := waitFinalized(t, l, 1)
	after, err := os.Stat(infos[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(before, after) {
		t.Fatal("a pack whose finalize failed was finalized in place")
	}
	if !l.HasDurable(a.Key) {
		t.Fatal("acknowledged value lost by salvage")
	}
}

// TestPackAheadSalvageFailsClosed fails a batch sync and then the sync of
// the salvaged copy: the Log fails closed, refusing every Append, and the
// poisoned pack's values report no durable copy.
func TestPackAheadSalvageFailsClosed(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	var n atomic.Int64
	hook := func(f *os.File) error {
		if n.Add(1) == 2 || strings.HasSuffix(f.Name(), ".salvage") {
			return syscall.EIO
		}
		return f.Sync()
	}
	l, err := Open(ctx, dir, Options{SyncData: hook})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// The second batch's sync fails, and so does its salvage.
	a, b, c := testBlock(t, 1, 3000), testBlock(t, 2, 3000), testBlock(t, 3, 3000)
	if err := l.Append(ctx, []Block{a}); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(ctx, []Block{b}); err == nil {
		t.Fatal("append succeeded on a failed sync")
	}
	deadline := time.Now().Add(10 * time.Second)
	for l.GetStats().SalvageFailures == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("salvage never failed: %+v", l.GetStats())
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Every Append now fails, and the poisoned values are not durable.
	if err := l.Append(ctx, []Block{c}); err == nil {
		t.Fatal("append succeeded after a failed salvage")
	}
	if l.HasDurable(a.Key) {
		t.Fatal("a value of the unsalvaged pack reports durable")
	}
	if got, ok, err := l.Get(a.Key); err != nil || !ok || !bytes.Equal(got, a.Value) {
		t.Fatalf("value of the unsalvaged pack does not read: %v %v", ok, err)
	}
}

// TestPackAheadUploadDropsRemoved removes a value while its pack is not
// uploaded, then marks the pack uploaded: the value is forgotten then.
func TestPackAheadUploadDropsRemoved(t *testing.T) {
	ctx := t.Context()
	l, err := Open(ctx, t.TempDir(), Options{Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	a := testBlock(t, 1, 3000)
	if err := l.Append(ctx, []Block{a}); err != nil {
		t.Fatal(err)
	}
	infos := waitFinalized(t, l, 1)
	l.Remove(a.Key)
	if !l.Has(a.Key) {
		t.Fatal("removed value forgotten before its pack uploaded")
	}
	l.MarkUploaded(infos[0].Seq)
	if l.Has(a.Key) {
		t.Fatal("removed value held after its pack uploaded")
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

// TestPackAheadFinalizeSyncHook finalizes a pack with a SyncData that counts
// its calls: the index sync goes through the hook too, so a test that fails
// syncs reaches every one.
func TestPackAheadFinalizeSyncHook(t *testing.T) {
	ctx := t.Context()
	var calls atomic.Int64
	hook := func(f *os.File) error {
		calls.Add(1)
		return f.Sync()
	}
	l, err := Open(ctx, t.TempDir(), Options{SyncData: hook, Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := l.Append(ctx, []Block{testBlock(t, 1, 3000)}); err != nil {
		t.Fatal(err)
	}
	waitFinalized(t, l, 1)
	if n := calls.Load(); n < 2 {
		t.Fatalf("finalize bypassed SyncData: %d calls", n)
	}
}
