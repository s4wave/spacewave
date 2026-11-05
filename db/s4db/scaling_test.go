package s4db

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// scaleKeys is the smaller file's key count; the larger holds ten times as
// many.
const scaleKeys = 20_000

// scaleRunBytes closes the built runs small, so the larger file holds ten
// times the runs of the smaller.
const scaleRunBytes = 16 << 10

// scaleCalls is the calls of an entry point one measurement times.
const scaleCalls = 400

// scaleTrials is the measurements taken of each entry point; the fastest
// counts, since other work on the host only slows a trial.
const scaleTrials = 7

// readCounter counts the reads a database makes of its file.
type readCounter struct {
	storage
	// reads counts ReadAt calls.
	reads atomic.Int64
}

// ReadAt counts the read and passes it on.
func (r *readCounter) ReadAt(p []byte, off int64) (int, error) {
	r.reads.Add(1)
	return r.storage.ReadAt(p, off)
}

// scaled is a built file opened for measurement.
type scaled struct {
	// db is the open database.
	db *DB
	// rc counts its reads.
	rc *readCounter
	// keys is the number of keys.
	keys int
	// tailFrom is the state the tail measurement applies records after.
	tailFrom *state
}

// scaleKey returns key i of a scaled file.
func scaleKey(i int) []byte {
	return fmt.Appendf(nil, "k/%08d", i)
}

// openScaled builds a file of keys keys through build, every sixteenth with
// a value over inlineMax, and opens it with no block cache, so every lookup
// reads its blocks and the reads per call show.
func openScaled(t *testing.T, keys int) *scaled {
	// Build the file in key order.
	t.Helper()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "scale.s4wave")
	f, closeFile, err := OpenFile(path, func(name string) (*os.File, error) {
		return os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	small, large := make([]byte, 64), make([]byte, 1024)
	err = build(ctx, &osFile{File: f, path: path, closeFile: closeFile}, Options{}, scaleRunBytes, func(yield func(key, value []byte) error) error {
		for i := range keys {
			v := small
			if i%16 == 0 {
				v = large
			}
			if err := yield(scaleKey(i), v); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		err = closeFile()
	}
	if err != nil {
		t.Fatal(err)
	}

	// Open it through a read counter, and load every run's reader so the
	// measurements leave out the footers' first reads.
	f, closeFile, err = OpenFile(path, func(name string) (*os.File, error) {
		return os.OpenFile(name, os.O_RDWR, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	rc := &readCounter{storage: &osFile{File: f, path: path, closeFile: closeFile}}
	db, err := open(rc, Options{CacheBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for _, m := range db.cur.Load().m.runs {
		if _, err := db.reader(m); err != nil {
			t.Fatal(err)
		}
	}
	return &scaled{db: db, rc: rc, keys: keys}
}

// scaleCost is what one call of an entry point costs.
type scaleCost struct {
	// bytes and allocs are the heap bytes and objects allocated.
	bytes, allocs float64
	// reads is the file reads.
	reads float64
	// took is the wall time.
	took time.Duration
}

// String formats the cost.
func (c scaleCost) String() string {
	return fmt.Sprintf("%.0f B, %.1f allocs, %.2f reads, %v", c.bytes, c.allocs, c.reads, c.took)
}

// measure returns the cost of one call of op on each of two files: the
// allocation and reads averaged over the calls of the fastest trial, and that
// trial's time per call. The files' trials alternate, so load on the host
// slows both alike.
func measure(t *testing.T, files [2]*scaled, op func(s *scaled, i int) error) [2]scaleCost {
	// Warm each file's path.
	t.Helper()
	for _, s := range files {
		for i := range 64 {
			if err := op(s, i); err != nil {
				t.Fatal(err)
			}
		}
	}

	// Time the trials in turn, keeping each file's fastest.
	best := [2]scaleCost{{took: time.Hour}, {took: time.Hour}}
	for trial := range scaleTrials {
		for f, s := range files {
			if c := measureTrial(t, s, op, trial); c.took < best[f].took {
				best[f] = c
			}
		}
	}
	return best
}

// measureTrial runs scaleCalls calls of op on s between two readings of the
// counters, numbering them from trial's first call.
func measureTrial(t *testing.T, s *scaled, op func(s *scaled, i int) error, trial int) scaleCost {
	// Run the calls.
	t.Helper()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	reads := s.rc.reads.Load()
	start := time.Now()
	for i := range scaleCalls {
		if err := op(s, trial*scaleCalls+i); err != nil {
			t.Fatal(err)
		}
	}
	took := time.Since(start)
	runtime.ReadMemStats(&after)

	// Average the counters over the calls.
	return scaleCost{
		bytes:  float64(after.TotalAlloc-before.TotalAlloc) / scaleCalls,
		allocs: float64(after.Mallocs-before.Mallocs) / scaleCalls,
		reads:  float64(s.rc.reads.Load()-reads) / scaleCalls,
		took:   took / scaleCalls,
	}
}

// flat fails the test unless the cost at ten times the data stays within a
// small factor of the cost at one: allocation and reads near equal, and time
// under twice, where a walk of every run or chunk would take several times.
func flat(t *testing.T, name string, small, large scaleCost) {
	t.Helper()
	t.Logf("%s: n %v; 10n %v", name, small, large)
	switch {
	case large.bytes > small.bytes*1.25+512:
		t.Errorf("%s allocates %.0f B a call at 10n against %.0f at n", name, large.bytes, small.bytes)
	case large.allocs > small.allocs*1.25+1:
		t.Errorf("%s allocates %.1f objects a call at 10n against %.1f at n", name, large.allocs, small.allocs)
	case large.reads > small.reads*1.25+0.5:
		t.Errorf("%s reads %.2f times a call at 10n against %.2f at n", name, large.reads, small.reads)
	case large.took > 2*small.took+5*time.Microsecond:
		t.Errorf("%s takes %v a call at 10n against %v at n", name, large.took, small.took)
	}
}

// TestScaling runs each entry point the Volume and World layers call on a
// file of n keys and one of 10n, with ten times the runs, and checks that a
// call's allocation, reads and time stay flat: each should grow with the
// call's own arguments, never with the file. The scheduler's checks run on
// chunk tables of n and 10n chunks.
func TestScaling(t *testing.T) {
	// Build the two files.
	ctx := context.Background()
	files := [2]*scaled{openScaled(t, scaleKeys), openScaled(t, 10*scaleKeys)}
	if r0, r1 := len(files[0].db.cur.Load().m.runs), len(files[1].db.cur.Load().m.runs); r1 < 8*r0 {
		t.Fatalf("built %d and %d runs, want ten times as many", r0, r1)
	}

	// pick returns a key of s for call i, spread over the file. inline
	// returns a distinct key for each i of the fifteen in sixteen whose values
	// are inline, so a commit overwrites a key the memtable lacks, and frees no
	// value bytes the cleaner would then reclaim.
	pick := func(s *scaled, i int) int {
		return int(uint64(i) * 0x9e3779b97f4a7c15 % uint64(s.keys)) // #nosec G115 -- a test index.
	}
	inline := func(s *scaled, i int) []byte {
		j := i * 7919 % (s.keys / 16 * 15)
		return scaleKey(j/15*16 + j%15 + 1)
	}

	// Each entry point, as a call on one file.
	ops := []struct {
		name string
		op   func(s *scaled, i int) error
	}{
		{"read transaction", func(s *scaled, _ int) error {
			tx, err := s.db.NewTransaction(ctx, false)
			if err == nil {
				tx.Discard()
			}
			return err
		}},
		{"get", func(s *scaled, i int) error {
			tx := &Tx{db: s.db, st: s.db.cur.Load()}
			_, _, err := tx.Get(ctx, scaleKey(pick(s, i)))
			return err
		}},
		{"get absent", func(s *scaled, i int) error {
			tx := &Tx{db: s.db, st: s.db.cur.Load()}
			_, _, err := tx.Get(ctx, append(scaleKey(pick(s, i)), '~'))
			return err
		}},
		{"get batch of 16", func(s *scaled, i int) error {
			tx := &Tx{db: s.db, st: s.db.cur.Load()}
			keys := make([][]byte, 16)
			for j := range keys {
				keys[j] = scaleKey(pick(s, i*16+j))
			}
			_, _, err := tx.GetBatch(ctx, keys)
			return err
		}},
		{"iterate a prefix", func(s *scaled, i int) error {
			tx := &Tx{db: s.db, st: s.db.cur.Load()}
			it := tx.iterate(scaleKey(pick(s, i))[:9], false)
			defer it.Close()
			for it.Next() {
			}
			return it.Err()
		}},
		{"commit of 4 keys", func(s *scaled, i int) error {
			tx, err := s.db.NewTransaction(ctx, true)
			if err != nil {
				return err
			}
			for j := range 4 {
				if err := tx.Set(ctx, inline(s, i*4+j), []byte("v")); err != nil {
					tx.Discard()
					return err
				}
			}
			return tx.(*Tx).CommitOrdered(ctx)
		}},
		{"tail of one commit", func(s *scaled, i int) error {
			if i%scaleCalls == 0 {
				s.tailFrom = s.db.cur.Load()
				if err := commitOne(ctx, s.db, inline(s, i)); err != nil {
					return err
				}
			}
			_, _, err := s.db.tail(s.tailFrom, false)
			return err
		}},
	}
	for _, o := range ops {
		costs := measure(t, files, o.op)
		flat(t, o.name, costs[0], costs[1])
	}

	// The scheduler's checks on tables of n and 10n chunks, each table a
	// stand-in file whose manifest the checks read.
	var tables [2]*scaled
	manifests := map[*scaled]*manifest{}
	for f, chunks := range []uint64{scaleKeys, 10 * scaleKeys} {
		tables[f] = &scaled{db: &DB{}, rc: &readCounter{}}
		manifests[tables[f]] = scaleManifest(chunks)
	}
	sched := &scheduler{cleaner: &cleaner{}}
	var sink int
	checks := measure(t, tables, func(s *scaled, _ int) error {
		m := manifests[s]
		if _, ok := sched.cleaner.due(m); ok {
			sink++
		}
		if _, ok := (&mergeJob{}).pick(m); ok {
			sink++
		}
		if sched.hasPending(&state{m: m}) {
			sink++
		}
		return nil
	})
	if sink == 0 {
		t.Fatal("the checks found no work on a table that holds some")
	}
	flat(t, "scheduler checks", checks[0], checks[1])
}

// commitOne commits one key with a durable-ordered commit.
func commitOne(ctx context.Context, db *DB, key []byte) error {
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	if err := tx.Set(ctx, key, []byte("t")); err != nil {
		tx.Discard()
		return err
	}
	return tx.(*Tx).CommitOrdered(ctx)
}

// scaleManifest returns a manifest whose table holds chunks value chunks,
// most of them full, with a free chunk low in the file, a pending one, and
// a run holding the last chunk, so every check has work to find.
func scaleManifest(chunks uint64) *manifest {
	// Fill the table.
	m := newManifest()
	r := rand.New(rand.NewPCG(chunks, 1))
	for c := uint64(firstChunk); c < chunks; c++ {
		live := uint32(r.IntN(chunkSize))
		m.table.set(c, chunkEntry{kind: chunkValue, live: live, used: chunkSize, seq: c})
	}
	m.table.set(firstChunk+1, chunkEntry{})
	m.table.set(firstChunk+2, chunkEntry{kind: chunkPending, owner: chunkValue})

	// End it with a run, so the merge picker finds a run at the top.
	run := runMeta{id: 1, level: 1, chunks: []uint64{chunks}, smallest: []byte("a"), largest: []byte("z")}
	m.table.set(chunks, chunkEntry{kind: chunkRun, owner: run.id})
	m.runs = []runMeta{run}
	return m
}
