package rangemerge

import (
	"bytes"
	"context"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/pkg/errors"
)

// testCosts approximates storage4000: an open and a random disk read sum to
// the 210 ms first byte logged from be-5, and a further packet moves 32 KiB
// at the 130 MiB/s logged on one connection. The split between open and
// seek is assumed, not measured.
var testCosts = costs{
	open:   100 * time.Millisecond,
	close:  time.Millisecond,
	seek:   110 * time.Millisecond,
	packet: 250 * time.Microsecond,
}

// randomPack returns n random bytes.
func randomPack(r *rand.Rand, n int) []byte {
	b := make([]byte, n)
	_, _ = r.Read(b)
	return b
}

// span is one range read.
type span struct {
	off int64
	n   int
}

// readAll reads spans of name through s at once and checks each against obj.
func readAll(t *testing.T, s *Store, name string, obj []byte, spans []span) {
	t.Helper()
	var wg sync.WaitGroup
	errs := make([]error, len(spans))
	got := make([][]byte, len(spans))
	for i, sp := range spans {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got[i], errs[i] = s.GetObjectRange(context.Background(), "b", name, sp.off, sp.n)
		}()
	}
	wg.Wait()
	for i, sp := range spans {
		if errs[i] != nil {
			t.Fatalf("read %d: %v", i, errs[i])
		}
		end := min(int64(len(obj)), sp.off+int64(sp.n))
		var want []byte
		if sp.off < int64(len(obj)) {
			want = obj[sp.off:end]
		}
		if !bytes.Equal(got[i], want) {
			t.Fatalf("read %d at %d+%d: got %d bytes, want %d", i, sp.off, sp.n, len(got[i]), len(want))
		}
	}
}

// TestMergeAdjacent checks that concurrent reads of nearby ranges of one pack
// cost one read of the server, and each gets its own bytes.
func TestMergeAdjacent(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	obj := randomPack(r, 1<<20)
	f := newFakeSFTP(map[string][]byte{"b/p1": obj}, 1, 256, testCosts)
	s := New(f, Config{Window: 5 * time.Millisecond, MaxGap: 64 << 10, MaxSpan: 1 << 20})
	spans := []span{{0, 4096}, {8192, 4096}, {4096, 4096}, {70000, 1000}, {65536, 4096}, {8192, 100}}
	readAll(t, s, "p1", obj, spans)
	if _, _, reads := f.totals(); reads != 1 {
		t.Fatalf("server reads = %d, want 1", reads)
	}
	if st := s.Stats(); st.Reads != uint64(len(spans)) || st.Spans != 1 {
		t.Fatalf("stats = %+v", st)
	}
}

// TestMergeSplits checks that a gap past MaxGap and a span past MaxSpan each
// start a new read, and reads of other packs never merge.
func TestMergeSplits(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	a, b := randomPack(r, 1<<20), randomPack(r, 1<<20)
	f := newFakeSFTP(map[string][]byte{"b/a": a, "b/b": b}, 2, 256, testCosts)
	s := New(f, Config{Window: 5 * time.Millisecond, MaxGap: 1024, MaxSpan: 64 << 10})

	// Two clusters by gap, and the second split by span.
	spans := []span{{0, 1000}, {1500, 1000}, {500000, 40000}, {540000, 40000}}
	readAll(t, s, "a", a, spans)
	if _, _, reads := f.totals(); reads != 3 {
		t.Fatalf("server reads = %d, want 3", reads)
	}

	// Another pack read at once merges only with its own ranges.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); readAll(t, s, "a", a, []span{{0, 10}}) }()
	go func() { defer wg.Done(); readAll(t, s, "b", b, []span{{0, 10}}) }()
	wg.Wait()
	if _, _, reads := f.totals(); reads != 5 {
		t.Fatalf("server reads = %d, want 5", reads)
	}
}

// TestMergeShortAndMissing checks reads at the end of a pack come back short,
// past it empty, and a missing pack's error reaches every reader.
func TestMergeShortAndMissing(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	obj := randomPack(r, 10000)
	f := newFakeSFTP(map[string][]byte{"b/p": obj}, 1, 256, testCosts)
	s := New(f, Config{Window: 5 * time.Millisecond, MaxGap: 64 << 10, MaxSpan: 1 << 20})
	readAll(t, s, "p", obj, []span{{9000, 4096}, {12000, 100}, {0, 10}})

	// Every reader of a missing pack gets the error.
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = s.GetObjectRange(context.Background(), "b", "gone", int64(i)*10, 10)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err == nil {
			t.Fatalf("reader %d of a missing pack: no error", i)
		}
	}
}

// TestMergeFailedRead checks a failed server read fails each merged reader.
func TestMergeFailedRead(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	obj := randomPack(r, 1<<16)
	f := newFakeSFTP(map[string][]byte{"b/p": obj}, 1, 256, testCosts)
	f.fail = errors.New("connection lost")
	s := New(f, Config{Window: 5 * time.Millisecond, MaxGap: 64 << 10, MaxSpan: 1 << 20})
	var wg sync.WaitGroup
	errs := make([]error, 3)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = s.GetObjectRange(context.Background(), "b", "p", int64(i)*100, 10)
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err == nil || err.Error() != "connection lost" {
			t.Fatalf("reader %d: err = %v", i, err)
		}
	}
}

// TestMergeCanceledReader checks a reader whose context ends returns at once
// with its cause while the others still get their bytes.
func TestMergeCanceledReader(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	obj := randomPack(r, 1<<16)
	f := newFakeSFTP(map[string][]byte{"b/p": obj}, 1, 256, testCosts)
	s := New(f, Config{Window: 50 * time.Millisecond, MaxGap: 64 << 10, MaxSpan: 1 << 20})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := s.GetObjectRange(ctx, "b", "p", 0, 10)
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled reader: err = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled reader did not return")
	}
	readAll(t, s, "p", obj, []span{{100, 10}, {200, 10}})
}

// workload is a walk's asks: packs holding 1, 2 or 3 of the asked blocks, a
// mean of about 1.63 as be-5's sample of 605 asks showed, each block blockBytes
// at a random offset, asked in walk order, inFlight at once.
type workload struct {
	objects map[string][]byte
	asks    []ask
}

// ask is one block read.
type ask struct {
	name string
	off  int64
	n    int
}

// newWorkload builds packs with the asked blocks in a random cluster each.
func newWorkload(r *rand.Rand, packs, blockBytes int) workload {
	w := workload{objects: map[string][]byte{}}
	for p := range packs {
		name := "pack" + string(rune('A'+p%26)) + string(rune('a'+p/26%26)) + string(rune('0'+p/676))
		obj := randomPack(r, 1<<20)
		w.objects["b/"+name] = obj
		k := 1
		switch x := r.Float64(); {
		case x < 0.13:
			k = 3
		case x < 0.50:
			k = 2
		}
		// Blocks written together sit together in their pack.
		base := r.Int63n(int64(len(obj) - 4*blockBytes))
		for i := range k {
			w.asks = append(w.asks, ask{name: name, off: base + int64(i*blockBytes), n: blockBytes})
		}
	}
	return w
}

// run asks every block through get, inFlight at once, and fails on wrong
// bytes.
func (w workload) run(t *testing.T, inFlight int, get func(ask) ([]byte, error)) {
	t.Helper()
	for i := 0; i < len(w.asks); i += inFlight {
		batch := w.asks[i:min(len(w.asks), i+inFlight)]
		var wg sync.WaitGroup
		for _, a := range batch {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got, err := get(a)
				obj := w.objects["b/"+a.name]
				if err != nil || !bytes.Equal(got, obj[a.off:a.off+int64(a.n)]) {
					t.Errorf("ask %s+%d: err %v", a.name, a.off, err)
				}
			}()
		}
		wg.Wait()
	}
}

// perSession returns blocks per second of one session's server time.
func perSession(blocks int, f *fakeSFTP) float64 {
	clock, _, _ := f.totals()
	return float64(blocks) / clock.Seconds()
}

// TestProjection measures, on the fake's one-request-at-a-time sessions,
// blocks per session-second for the read path today and with merging.
//
//   - today, unindexed: each block costs an open, the pack's index tail and
//     the block (PackfileStore.getBlock through the pack readers);
//   - today, extent: the index row's extent, an open and one read
//     (PackStore.readExtent), the handle kept as the client keeps it;
//   - merged: the extent reads of one pack asked together merge into one.
func TestProjection(t *testing.T) {
	const packs, blockBytes, inFlight, sessions = 400, 32 << 10, 32, 8
	w := newWorkload(rand.New(rand.NewSource(6)), packs, blockBytes)
	blocks := len(w.asks)

	// Today through the pack readers: tail then block, on a cold handle.
	tail := newFakeSFTP(w.objects, sessions, 256, testCosts)
	w.run(t, inFlight, func(a ask) ([]byte, error) {
		obj := w.objects["b/"+a.name]
		if _, err := tail.GetObjectRange(context.Background(), "b", a.name, int64(len(obj)-16<<10), 16<<10); err != nil {
			return nil, err
		}
		return tail.GetObjectRange(context.Background(), "b", a.name, a.off, a.n)
	})

	// Today with extents: one read a block, the handle shared.
	direct := newFakeSFTP(w.objects, sessions, 256, testCosts)
	w.run(t, inFlight, func(a ask) ([]byte, error) {
		return direct.GetObjectRange(context.Background(), "b", a.name, a.off, a.n)
	})

	// Merged extent reads.
	merged := newFakeSFTP(w.objects, sessions, 256, testCosts)
	s := New(merged, Config{})
	w.run(t, inFlight, func(a ask) ([]byte, error) {
		return s.GetObjectRange(context.Background(), "b", a.name, a.off, a.n)
	})

	// Report and check the order.
	rt, rd, rm := perSession(blocks, tail), perSession(blocks, direct), perSession(blocks, merged)
	t.Logf("%d blocks in %d packs (%.2f a pack), %d sessions", blocks, packs, float64(blocks)/packs, sessions)
	t.Logf("blocks per session-second: unindexed %.2f, extent %.2f, extent merged %.2f", rt, rd, rm)
	t.Logf("all %d sessions: unindexed %.0f/h, extent %.0f/h, merged %.0f/h", sessions, rt*sessions*3600, rd*sessions*3600, rm*sessions*3600)
	if !(rt < rd && rd < rm) {
		t.Fatalf("want unindexed < extent < merged, got %.2f %.2f %.2f", rt, rd, rm)
	}
	if st := s.Stats(); st.Spans >= st.Reads {
		t.Fatalf("merging saved no reads: %+v", st)
	}
}
