package writelog

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	b58 "github.com/mr-tron/base58/base58"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// testHash returns the hash of block i.
func testHash(i int) *hash.Hash {
	h, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("block "+strconv.Itoa(i)))
	if err != nil {
		panic(err)
	}
	return h
}

// newLog builds a log for a test.
func newLog() *Log {
	return New(logrus.NewEntry(logrus.New()))
}

// readAll reads every line after from.
func readAll(t *testing.T, src interface {
	ReadLog(context.Context, Position, int, func(Line) error) (ReadResult, error)
}, from Position) ([]Line, ReadResult) {
	t.Helper()
	var lines []Line
	res, err := src.ReadLog(t.Context(), from, 0, func(l Line) error {
		lines = append(lines, l)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return lines, res
}

// TestAppendBase58 checks the in-place encoding matches the base58 package
// and a line's hash matches hash.MarshalString.
func TestAppendBase58(t *testing.T) {
	// Random values of each length, some with leading zeros.
	rng := rand.New(rand.NewPCG(1, 2))
	for n := range hashScratchBytes + 2 {
		for k := range (min(n, 3) + 1) * 16 {
			z := k % (min(n, 3) + 1)
			src := make([]byte, n)
			for i := z; i < n; i++ {
				src[i] = byte(rng.UintN(256))
			}
			if got, want := string(appendBase58(nil, src)), b58.Encode(src); got != want {
				t.Fatalf("len %d zeros %d: %q, want %q", n, z, got, want)
			}
		}
	}

	// A record line.
	var scratch [hashScratchBytes]byte
	h := testHash(7)
	if got, want := string(appendRecord(nil, 12, KindPut, h, &scratch)), "12 p "+h.MarshalString()+"\n"; got != want {
		t.Fatalf("line %q, want %q", got, want)
	}
}

// TestEpochs writes, rotates, reads and closes epochs.
func TestEpochs(t *testing.T) {
	dir := t.TempDir()
	l := newLog()
	if err := l.Set(&Options{Dir: dir, SegmentBytes: 1024}); err != nil {
		t.Fatal(err)
	}

	// Write enough lines to rotate several segments, of each kind.
	const n = 200
	for i := range n {
		switch i % 3 {
		case 0:
			l.Put(testHash(i))
		case 1:
			l.Root(testHash(i))
		default:
			l.Fill(testHash(i))
		}
	}
	pos, ok := l.Position()
	if !ok || pos.Seq != n {
		t.Fatalf("position %v %v", pos, ok)
	}
	if segs, _ := listSegments(dir); len(segs) < 5 {
		t.Fatalf("segments %d", len(segs))
	}

	// Read every line, then from the middle, then in bounded steps.
	start := Position{Epoch: pos.Epoch}
	lines, res := readAll(t, l, start)
	if len(lines) != n || res.State != ReadOpen || res.Next != pos {
		t.Fatalf("read %d lines, %+v", len(lines), res)
	}
	for i, line := range lines {
		want := []Kind{KindPut, KindRoot, KindFill}[i%3]
		if line.Seq != uint64(i+1) || line.Kind != want || !line.Hash.EqualVT(testHash(i)) {
			t.Fatalf("line %d = %+v", i, line)
		}
	}
	if lines, _ := readAll(t, l, Position{Epoch: pos.Epoch, Seq: 150}); len(lines) != 50 || lines[0].Seq != 151 {
		t.Fatalf("read from 150: %d lines", len(lines))
	}
	var steps int
	for from := start; ; steps++ {
		res, err := l.ReadLog(t.Context(), from, 64, func(Line) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		from = res.Next
		if res.State != ReadMore {
			if res.State != ReadOpen || from != pos {
				t.Fatalf("stepped read ended %+v", res)
			}
			break
		}
	}
	if steps != 3 {
		t.Fatalf("steps %d", steps)
	}

	// Trim to line 120: the read from 120 still works, the read from the
	// start is a gap.
	if err := l.TrimLog(t.Context(), Position{Epoch: pos.Epoch, Seq: 120}); err != nil {
		t.Fatal(err)
	}
	if lines, res := readAll(t, l, Position{Epoch: pos.Epoch, Seq: 120}); len(lines) != 80 || res.State != ReadOpen {
		t.Fatalf("read after trim: %d lines, %+v", len(lines), res)
	}
	if _, res := readAll(t, l, start); res.State != ReadGap {
		t.Fatalf("read of trimmed lines %+v", res)
	}
	if st := l.Status(); st.Bytes != dirBytes(t, dir) {
		t.Fatalf("status bytes %d, directory %d", st.Bytes, dirBytes(t, dir))
	}

	// Close: the directory reads to the close line. On again: a new epoch,
	// and the old one's segments are gone.
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if lines, res := readAll(t, Dir(dir), Position{Epoch: pos.Epoch, Seq: 190}); len(lines) != 10 || res.State != ReadClosed {
		t.Fatalf("closed read: %d lines, %+v", len(lines), res)
	}
	if err := l.Set(&Options{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	next, _ := l.Position()
	if next.Epoch == pos.Epoch || next.Seq != 0 {
		t.Fatalf("new epoch %v", next)
	}
	if _, res := readAll(t, l, Position{Epoch: pos.Epoch, Seq: 190}); res.State != ReadGap {
		t.Fatalf("old epoch read %+v", res)
	}
	_ = l.Close()
}

// failAfter fails every write after n bytes.
type failAfter struct {
	n int
}

func (f *failAfter) Write(p []byte) (int, error) {
	if f.n < len(p) {
		return 0, errors.New("disk failed")
	}
	f.n -= len(p)
	return len(p), nil
}

func (f *failAfter) Close() error { return nil }

// TestBrokenEpochs checks a failed write and a full directory end the epoch
// with no close line and fail no store write, and the log stays off until
// it is turned off and on.
func TestBrokenEpochs(t *testing.T) {
	ctx := t.Context()
	store := block_store_writeback.NewMarkingStore(
		block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false),
		func(context.Context, []block_store_writeback.Mark) error { return nil },
	)
	l := newLog()
	store.SetObserver(l)
	put := func(i int) {
		t.Helper()
		if _, _, err := store.PutBlock(ctx, []byte("block "+strconv.Itoa(i)), nil); err != nil {
			t.Fatalf("put %d: %v", i, err)
		}
	}
	for _, tc := range []struct {
		name string
		opts Options
	}{
		{"write fails", Options{create: func(string) (io.WriteCloser, error) { return &failAfter{n: 4096}, nil }}},
		{"cap", Options{Cap: 2048}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Write past the failure; every put succeeds.
			dir := t.TempDir()
			opts := tc.opts
			opts.Dir = dir
			if err := l.Set(&opts); err != nil {
				t.Fatal(err)
			}
			pos, _ := l.Position()
			for i := range bufferBytes / 40 {
				put(i)
			}
			st := l.Status()
			if st.On || st.Broken == "" {
				t.Fatalf("status %+v", st)
			}

			// The epoch reads as a gap, and the same options keep it off.
			if _, res := readAll(t, l, pos); res.State != ReadGap {
				t.Fatalf("broken epoch read %+v", res)
			}
			if err := l.Set(&opts); err != nil || l.Status().On {
				t.Fatalf("same options turned the log on: %v", err)
			}

			// Off and on starts a new epoch.
			_ = l.Set(nil)
			opts.create = nil
			opts.Cap = 0
			if err := l.Set(&opts); err != nil {
				t.Fatal(err)
			}
			put(0)
			if st := l.Status(); !st.On || st.Epoch == pos.Epoch || st.Seq != 1 {
				t.Fatalf("status after restart %+v", st)
			}
			_ = l.Set(nil)
		})
	}
}

// TestAppendScales checks the allocations of the append stay flat between n
// and 10n lines: none per line but a segment's rotation, which opens the
// next file and recounts the directory.
func TestAppendScales(t *testing.T) {
	hashes := make([]*hash.Hash, 1000)
	for i := range hashes {
		hashes[i] = testHash(i)
	}
	run := func(n int) (allocs uint64, segments int) {
		// Turn on a log in a fresh directory.
		dir := t.TempDir()
		l := newLog()
		if err := l.Set(&Options{Dir: dir, Cap: 1 << 40}); err != nil {
			t.Fatal(err)
		}

		// Count the allocations of n puts.
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		for i := range n {
			l.Put(hashes[i%len(hashes)])
		}
		runtime.ReadMemStats(&after)
		segs, _ := listSegments(dir)
		_ = l.Close()
		return after.Mallocs - before.Mallocs, len(segs)
	}
	const n = 20_000
	small, _ := run(n)
	large, segs := run(10 * n)
	t.Logf("%d lines: %d allocations; %d lines: %d allocations in %d segments", n, small, 10*n, large, segs)
	// A rotation opens a file and lists the directory, at most C / G files.
	if large > small+uint64(segs)*32+16 {
		t.Fatalf("allocations grew from %d to %d", small, large)
	}
}

// TestDirTrimFreesCap checks a reader trimming the directory directly, as the
// command does until the backend serves its log, keeps the log under its cap:
// the log recounts the directory at each rotation.
func TestDirTrimFreesCap(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	log := newLog()
	if err := log.Set(&Options{Dir: dir, Cap: 4096, SegmentBytes: 1024}); err != nil {
		t.Fatal(err)
	}
	defer log.Close()

	// Write five caps of lines, trimming through the directory as they go.
	for i := range 400 {
		log.Put(testHash(i))
		if i%10 == 0 {
			pos, ok := log.Position()
			if !ok {
				t.Fatalf("epoch broke at line %d", i)
			}
			if err := Dir(dir).TrimLog(ctx, pos); err != nil {
				t.Fatal(err)
			}
		}
	}
	// The count runs ahead of the directory between rotations, never behind.
	if st := log.Status(); !st.On || st.Broken != "" || st.Bytes > 4096 || st.Bytes < dirBytes(t, dir) {
		t.Fatalf("status %+v, directory holds %d bytes", st, dirBytes(t, dir))
	}
}

// dirBytes sums the sizes of the segments in dir.
func dirBytes(t *testing.T, dir string) int64 {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, e := range entries {
		info, err := os.Stat(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		total += info.Size()
	}
	return total
}

// BenchmarkPut measures one logged put, and one with the log off.
func BenchmarkPut(b *testing.B) {
	h := testHash(1)
	for _, on := range []bool{false, true} {
		b.Run("on="+strconv.FormatBool(on), func(b *testing.B) {
			l := newLog()
			if on {
				if err := l.Set(&Options{Dir: b.TempDir(), Cap: 1 << 40}); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				l.Put(h)
			}
			_ = l.Close()
		})
	}
}
