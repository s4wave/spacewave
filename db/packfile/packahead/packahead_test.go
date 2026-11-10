package packahead

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/s4wave/spacewave/net/hash"
	"golang.org/x/sync/errgroup"
)

// testBlock returns block i: a value of size bytes derived from i, under the
// pack index key of its hash.
func testBlock(t testing.TB, i, size int) Block {
	value := make([]byte, size)
	for j := 0; j < size; j += 8 {
		var w [8]byte
		binary.LittleEndian.PutUint64(w[:], uint64(i)*0x9e3779b97f4a7c15+uint64(j)) //nolint:gosec // test values.
		copy(value[j:], w[:])
	}
	h, err := hash.Sum(hash.HashType_HashType_SHA256, value)
	if err != nil {
		t.Fatal(err)
	}
	return Block{Key: packfile.BlockKey(h), Value: value}
}

// waitFinalized waits until the Log has at least n finalized packs.
func waitFinalized(t testing.TB, l *Log, n int) []PackInfo {
	deadline := time.After(10 * time.Second)
	for {
		infos, waitCh := l.GetPacks()
		if len(infos) >= n {
			return infos
		}
		select {
		case <-waitCh:
		case <-deadline:
			t.Fatalf("waited for %d finalized packs, have %d: %v", n, len(infos), l.GetStats().MaintainError)
		}
	}
}

// checkPack reads a finalized pack as a remote pack reader would and checks
// that it holds want.
func checkPack(t testing.TB, info PackInfo, want map[string][]byte, opts Options) {
	t.Helper()

	// Check the pack's name, size and bloom filter.
	if err := identity.ValidatePackID(info.ID); err != nil {
		t.Fatalf("pack %d id %q: %v", info.Seq, info.ID, err)
	}
	if info.Size > opts.withDefaults().MaxPackBytes || info.Blocks > opts.withDefaults().MaxPackBlocks {
		t.Fatalf("pack %d: %d bytes, %d blocks past the bounds", info.Seq, info.Size, info.Blocks)
	}
	bf := &bloom.BloomFilter{}
	if err := bf.UnmarshalVT(info.BloomFilter); err != nil {
		t.Fatal(err)
	}

	// Read every value through kvfile and compare it.
	f, err := os.Open(info.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rd, err := kvfile.BuildReaderWithFile(f)
	if err != nil {
		t.Fatalf("pack %d is not a kvfile: %v", info.Seq, err)
	}
	var n int
	err = rd.ScanPrefix(nil, func(key, value []byte) error {
		n++
		if w, ok := want[string(key)]; ok && !bytes.Equal(w, value) {
			t.Fatalf("pack %d holds a wrong value", info.Seq)
		}
		return nil
	})
	if err != nil || n != info.Blocks {
		t.Fatalf("pack %d: scanned %d of %d: %v", info.Seq, n, info.Blocks, err)
	}
}

// TestPackAhead writes batches from concurrent writers through rotation and
// finalizing, reads every block back from buffers and files, checks each
// finalized pack as a remote reader would, skips a block already held,
// forgets a freed one, reopens, and rotates a lingering pack.
func TestPackAhead(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	opts := Options{MaxPackBlocks: 16}
	l, err := Open(ctx, dir, opts)
	if err != nil {
		t.Fatal(err)
	}

	// Write 4 writers x 20 batches x 3 blocks of about 3.3 KB.
	want := make(map[string][]byte)
	var all []Block
	for i := range 240 {
		b := testBlock(t, i, 3000+i)
		all = append(all, b)
		want[string(b.Key)] = b.Value
	}
	eg, egCtx := errgroup.WithContext(ctx)
	for w := range 4 {
		eg.Go(func() error {
			for i := w * 60; i < (w+1)*60; i += 3 {
				if err := l.Append(egCtx, all[i:i+3]); err != nil {
					return err
				}
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		t.Fatal(err)
	}

	// Every block reads back, with at most one sync per batch and one per
	// rotated pack.
	for _, b := range all {
		got, ok, err := l.Get(b.Key)
		if err != nil || !ok || !bytes.Equal(got, b.Value) {
			t.Fatalf("get after append: ok=%v err=%v", ok, err)
		}
	}
	stats := l.GetStats()
	if stats.Entries != len(all) || stats.Syncs > uint64(80+stats.Packs) {
		t.Fatalf("stats after append: %+v", stats)
	}

	// The full packs finalize into packfiles a remote reader accepts.
	infos := waitFinalized(t, l, 240/16-1)
	for _, info := range infos {
		checkPack(t, info, want, opts)
	}
	for _, b := range all {
		if got, ok, err := l.Get(b.Key); err != nil || !ok || !bytes.Equal(got, b.Value) {
			t.Fatalf("get after finalize: ok=%v err=%v", ok, err)
		}
	}

	// A block already held is not written again.
	before := l.GetStats().LocalBytes
	if err := l.Append(ctx, all[:3]); err != nil {
		t.Fatal(err)
	}
	if after := l.GetStats().LocalBytes; after != before {
		t.Fatalf("held blocks rewritten: %d to %d bytes", before, after)
	}

	// A removed block stays held while its pack is not uploaded, and once it
	// is, it is forgotten until the next open.
	l.Remove(all[0].Key)
	if !l.Has(all[0].Key) {
		t.Fatal("removed block forgotten before its pack uploaded")
	}
	var uploaded int
	for _, info := range infos {
		l.MarkUploaded(info.Seq)
		uploaded += info.Blocks
	}
	var forgotten int
	for _, b := range all {
		l.Remove(b.Key)
		if !l.Has(b.Key) {
			forgotten++
		}
	}
	if forgotten != uploaded {
		t.Fatalf("removed %d blocks of uploaded packs holding %d", forgotten, uploaded)
	}
	l.Close()
	if err := l.Append(ctx, all[:1]); err != ErrClosed {
		t.Fatalf("append after close: %v", err)
	}

	// Reopen: the open pack finalizes, and every block reads again.
	l, err = Open(ctx, dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	infos, _ = l.GetPacks()
	if len(infos) != 15 {
		t.Fatalf("reopened with %d packs", len(infos))
	}
	for _, info := range infos {
		checkPack(t, info, want, opts)
	}
	for _, b := range all {
		if got, ok, err := l.Get(b.Key); err != nil || !ok || !bytes.Equal(got, b.Value) {
			t.Fatalf("get after reopen: ok=%v err=%v", ok, err)
		}
	}
	l.Close()

	// A lingering pack rotates and finalizes on its own.
	l, err = Open(ctx, dir, Options{Linger: 20 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	extra := testBlock(t, 1000, 100)
	if err := l.Append(ctx, []Block{extra}); err != nil {
		t.Fatal(err)
	}
	infos = waitFinalized(t, l, 16)
	if last := infos[len(infos)-1]; last.Blocks != 1 {
		t.Fatalf("lingering pack holds %d blocks", last.Blocks)
	}
}

// TestPackAheadWriteFailure fails a batch's write with ENOSPC: the write is
// not acknowledged and its block is not readable, the next write rotates and
// succeeds, and the failed pack is removed.
func TestPackAheadWriteFailure(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("needs /dev/full")
	}
	ctx := t.Context()
	dir := t.TempDir()
	l, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// Point the empty open pack's handle at a device whose writes fail.
	full, err := os.OpenFile("/dev/full", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_ = l.open.f.Close()
		l.open.f = full
	})

	// The write fails and leaves nothing readable.
	a, b := testBlock(t, 1, 3000), testBlock(t, 2, 3000)
	if err := l.Append(ctx, []Block{a}); err == nil || !strings.Contains(err.Error(), "no space") {
		t.Fatalf("append to a full device: %v", err)
	}
	if l.Has(a.Key) {
		t.Fatal("failed block is readable")
	}

	// The next write rotates and succeeds; the failed pack goes.
	if err := l.Append(ctx, []Block{a, b}); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := l.Get(a.Key); err != nil || !ok || !bytes.Equal(got, a.Value) {
		t.Fatalf("get after retry: ok=%v err=%v", ok, err)
	}
	deadline := time.After(10 * time.Second)
	for {
		_, waitCh := l.GetPacks()
		if l.GetStats().Packs == 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("failed pack not removed: %+v", l.GetStats())
		case <-waitCh:
		}
	}
	if _, err := os.Stat(filepath.Join(dir, openName(1))); !os.IsNotExist(err) {
		t.Fatalf("failed pack file: %v", err)
	}
}
