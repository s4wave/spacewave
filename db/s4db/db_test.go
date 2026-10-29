//go:build darwin || linux || windows

package s4db

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
)

// openTest opens a database in a temporary directory.
func openTest(t *testing.T, path string, opts Options) *DB {
	t.Helper()
	db, err := Open(path, opts)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// TestKvtx runs the shared store conformance suite.
func TestKvtx(t *testing.T) {
	db := openTest(t, filepath.Join(t.TempDir(), "kv.s4wave"), Options{})
	defer db.Close()
	if err := kvtest.TestAll(context.Background(), db); err != nil {
		t.Fatal(err)
	}
}

// TestModel checks random commits against a map across checkpoints,
// compaction, and reopening.
func TestModel(t *testing.T) {
	// Commit, check, compact, and reopen in rounds.
	path := filepath.Join(t.TempDir(), "model.s4wave")
	opts := Options{CheckpointMin: 64 << 10, CheckpointMax: 256 << 10}
	r := rand.New(rand.NewPCG(1, 2))
	m := make(model)
	db := openTest(t, path, opts)
	for round := range 8 {
		for range 200 {
			randomCommit(t, r, db, m, 4000)
		}
		m.check(t, db)
		checkSpace(t, db)
		if round%3 == 2 {
			if err := db.Compact(context.Background()); err != nil {
				t.Fatal(err)
			}
			m.check(t, db)
		}
		for range 20 {
			randomCommit(t, r, db, m, 4000)
		}
		checkCrash(t, db, path, m)
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		db = openTest(t, path, opts)
		m.check(t, db)
	}

	// Delete everything and compact.
	ctx := context.Background()
	tx, _ := db.NewTransaction(ctx, true)
	for k := range m {
		_ = tx.Delete(ctx, []byte(k))
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	clear(m)

	// The emptied file shrinks to the fixed chunk and the log chunk.
	if err := db.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	m.check(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.NewTransaction(ctx, false); !errors.Is(err, ErrClosed) {
		t.Fatalf("NewTransaction after Close = %v, want ErrClosed", err)
	}
	used, size := usage(t, path)
	t.Logf("empty file: %d bytes long, %d allocated", size, used)
	if size > 2*chunkSize || used > size {
		t.Fatalf("empty database is %d bytes long with %d allocated", size, used)
	}
}

// TestTornTail cuts the file inside the last record and checks that reopening
// keeps every earlier commit.
func TestTornTail(t *testing.T) {
	// Commit a history.
	dir := t.TempDir()
	path := filepath.Join(dir, "torn.s4wave")
	r := rand.New(rand.NewPCG(3, 4))
	m := make(model)
	db := openTest(t, path, Options{})
	for range 50 {
		randomCommit(t, r, db, m, 500)
	}

	// Commit once more, noting where the last record lies.
	before := maps.Clone(m)
	pos := db.cur.Load().pos
	randomCommit(t, r, db, m, 500)
	end := db.cur.Load().pos

	// Copy the file as a crash would leave it: the last record half written.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	clear(b[pos+(end-pos)/2 : end])
	crashed := filepath.Join(dir, "crashed.s4wave")
	if err := os.WriteFile(crashed, b, 0o600); err != nil {
		t.Fatal(err)
	}

	// Reopening the copy keeps every earlier commit.
	_ = db.Close()
	db = openTest(t, crashed, Options{})
	defer db.Close()
	before.check(t, db)

	// The log continues after the torn record.
	randomCommit(t, r, db, before, 500)
	before.check(t, db)
}

// TestStaleRecord loses one record of two to a crash, then commits a record
// of the same length in its place, and checks that the record after it,
// left from before the crash, stays out of the log.
func TestStaleRecord(t *testing.T) {
	// Open a file and a helper committing one key.
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "stale.s4wave")
	db := openTest(t, path, Options{})
	set := func(db *DB, key string) {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, []byte(key), []byte("v")); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Commit two records of one length, noting where the first lies.
	pos := db.cur.Load().pos
	set(db, "a")
	end := db.cur.Load().pos
	set(db, "b")

	// Copy the file as a crash would leave it: the first record lost and
	// the second intact.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	clear(b[pos:end])
	crashed := filepath.Join(dir, "crashed.s4wave")
	if err := os.WriteFile(crashed, b, 0o600); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	// Commit a record of the same length where the lost one was, then
	// commit again: the writer tails the log first.
	db = openTest(t, crashed, Options{})
	defer db.Close()
	set(db, "c")
	if got := db.cur.Load().pos; got != end {
		t.Fatalf("new record ends at %d, the lost one at %d", got, end)
	}
	set(db, "d")
	model{"c": []byte("v"), "d": []byte("v")}.check(t, db)
}

// TestBulkOverlay commits large batches back to back, as a bulk import
// does, and checks that the overlay stays bounded while background
// checkpoints run behind the writer.
func TestBulkOverlay(t *testing.T) {
	// Open a file with small checkpoints.
	ctx := context.Background()
	opts := Options{CheckpointMin: 16 << 10, CheckpointMax: 64 << 10}
	db := openTest(t, filepath.Join(t.TempDir(), "bulk.s4wave"), opts)
	defer db.Close()

	// Commit batches of keys interleaved across streams, each batch about
	// one CheckpointMax.
	const streams, perStream, rounds = 64, 32, 200
	limit := 4 * opts.CheckpointMax
	for i := range rounds {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for s := range streams {
			for j := range perStream {
				key := fmt.Appendf(nil, "%02d/%08d", s, i*perStream+j)
				if err := tx.Set(ctx, key, key); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if got := db.cur.Load().overlayBytes; got > limit {
			t.Fatalf("commit %d: overlay holds %d bytes, over %d", i, got, limit)
		}
	}
}

// TestRewriteLog rewrites one key, which grows the log but not the overlay,
// and checks that checkpoints keep the log an open replays bounded.
func TestRewriteLog(t *testing.T) {
	// Open a file with small checkpoints.
	ctx := context.Background()
	opts := Options{CheckpointMin: 16 << 10, CheckpointMax: 64 << 10}
	db := openTest(t, filepath.Join(t.TempDir(), "rewrite.s4wave"), opts)
	defer db.Close()

	// Rewrite the key well past the bound, checking the log after each
	// commit. A commit may land past the bound before it waits.
	val := make([]byte, 400)
	limit := 4*opts.CheckpointMax + 4<<10
	for i := range 4000 {
		val[0] = byte(i)
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, []byte("key"), val); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if got := db.cur.Load().logBytes; got > limit {
			t.Fatalf("commit %d: log holds %d bytes, over %d", i, got, limit)
		}
	}
}

// childEnv names the file a child process of TestTwoProcesses writes.
const childEnv = "S4DB_TEST_CHILD"

// childKeys is the number of keys each process of TestTwoProcesses writes.
const childKeys = 400

// probeEnv names the file whose writer lock a child process of
// TestSingleProcessHoldsWriterLock probes.
const probeEnv = "S4DB_TEST_PROBE"

// burnEnv makes the test binary a child that keeps one core busy, for
// BenchmarkCommitBesideCleaner.
const burnEnv = "S4DB_TEST_BURN"

// TestMain runs the child of TestTwoProcesses, of
// TestSingleProcessHoldsWriterLock or of BenchmarkCommitBesideCleaner when
// the test binary is started as one, and else the tests.
func TestMain(m *testing.M) {
	// Spin until killed as a burner.
	if os.Getenv(burnEnv) != "" {
		for { //nolint:staticcheck // A burner spins on purpose.
		}
	}

	// Choose the child the environment names, if any.
	run := runChild
	path := os.Getenv(childEnv)
	if path == "" {
		run, path = runProbe, os.Getenv(probeEnv)
	}

	// Run it in place of the tests.
	if path != "" {
		if err := run(path); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// processKey returns key i written by the process named who.
func processKey(who string, i int) []byte {
	return fmt.Appendf(nil, "%s/%06d", who, i)
}

// processValue returns the value of key i, large enough on some keys to be
// stored out of the index.
func processValue(i int) []byte {
	return bytes.Repeat([]byte{byte(i)}, []int{16, 900, 20000}[i%3])
}

// processDeleted reports whether TestTwoProcesses deletes key i once
// written.
func processDeleted(i int) bool {
	return i%5 < 3
}

// deleteKeys deletes the keys named for who that processDeleted names,
// eight to a durable commit.
func deleteKeys(ctx context.Context, db *DB, who string) error {
	for i := 0; i < childKeys; i += 8 {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		for j := i; j < i+8; j++ {
			if !processDeleted(j) {
				continue
			}
			if err := tx.Delete(ctx, processKey(who, j)); err != nil {
				tx.Discard()
				return err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// writeKeys commits childKeys keys named for who, alternating ordered and
// durable commits of a few keys each.
func writeKeys(ctx context.Context, db *DB, who string) error {
	for i := 0; i < childKeys; i += 4 {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		for j := i; j < i+4; j++ {
			if err := tx.Set(ctx, processKey(who, j), processValue(j)); err != nil {
				tx.Discard()
				return err
			}
		}
		if i%8 == 0 {
			err = tx.(*Tx).CommitOrdered(ctx)
		} else {
			err = tx.Commit(ctx)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// runChild writes the child's keys to the file at path and prints the last
// commit it saw.
func runChild(path string) error {
	// Open the file the parent holds open, checkpointing often so the
	// parent's background checkpoints race this process's writes, and
	// cleaning any slack.
	cleanMinSlack = 0
	db, err := Open(path, Options{CheckpointMin: 8 << 10, CheckpointMax: 16 << 10})
	if err != nil {
		return err
	}

	// Write the keys, delete most, and report the last commit.
	ctx := context.Background()
	if err := writeKeys(ctx, db, "child"); err != nil {
		_ = db.Close()
		return err
	}
	if err := deleteKeys(ctx, db, "child"); err != nil {
		_ = db.Close()
		return err
	}
	fmt.Println(db.Seq())
	return db.Close()
}

// runProbe prints whether another process holds the writer lock of the
// file at path, taking and releasing it when free.
func runProbe(path string) error {
	// Open the file without registering it, as another process would.
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	// Try the lock without waiting, and release it when taken.
	file := &osFile{File: f, path: path}
	ok, err := file.lock(lockWriter, false)
	if err != nil {
		return err
	}
	if ok {
		fmt.Println("free")
		return file.unlock(lockWriter)
	}
	fmt.Println("held")
	return nil
}

// probeWriterLock reports what a child process finds probing the writer
// lock of the file at path.
func probeWriterLock(t *testing.T, path string) string {
	// Run the test binary as the probe.
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), probeEnv+"="+path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("probe: %v: %s", err, out)
	}
	return strings.TrimSpace(string(out))
}

// TestSingleProcessHoldsWriterLock checks that a single-process handle
// holds the file's writer lock from open to close, across commits, so no
// other process writes while its checkpoints write without the lock.
func TestSingleProcessHoldsWriterLock(t *testing.T) {
	// Open the file and commit.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "single.s4wave")
	db := openTest(t, path, Options{SingleProcess: true})
	if err := writeKeys(ctx, db, "single"); err != nil {
		t.Fatal(err)
	}
	if got := probeWriterLock(t, path); got != "held" {
		t.Fatalf("open handle: probe found the writer lock %s", got)
	}

	// Close, which releases it.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if got := probeWriterLock(t, path); got != "free" {
		t.Fatalf("closed handle: probe found the writer lock %s", got)
	}
}

// TestSingleProcessWriters commits from several writers of a single-process
// handle while background checkpoints write their pages without the writer
// lock, and checks the keys and the space before and after reopening.
func TestSingleProcessWriters(t *testing.T) {
	// Write from each writer at once, checkpointing often.
	ctx := context.Background()
	opts := Options{CheckpointMin: 8 << 10, CheckpointMax: 16 << 10, SingleProcess: true}
	path := filepath.Join(t.TempDir(), "writers.s4wave")
	db := openTest(t, path, opts)

	// Start the writers and wait for them all.
	writers := []string{"a", "b", "c", "d"}
	errs := make([]error, len(writers))
	var wg sync.WaitGroup
	for i, who := range writers {
		wg.Go(func() {
			errs[i] = writeKeys(ctx, db, who)
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	// Check the keys and the space, then reopen and check the keys.
	m := make(model)
	for i := range childKeys {
		for _, who := range writers {
			m[string(processKey(who, i))] = processValue(i)
		}
	}
	m.check(t, db)
	checkSpace(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openTest(t, path, opts)
	defer db.Close()
	m.check(t, db)
}

// TestTwoProcesses writes and then thins one file from two processes at
// once, each running the background cleaner, and checks that each commit
// lands, the space stays consistent, the parent sees the child's commits
// through the change watcher, Compact tightens the value chunks both wrote,
// and the file holds the remaining keys once both close.
func TestTwoProcesses(t *testing.T) {
	// Open the file, cleaning any slack.
	ctx := context.Background()
	least := cleanMinSlack
	cleanMinSlack = 0
	defer func() { cleanMinSlack = least }()
	opts := Options{CheckpointMin: 32 << 10, CheckpointMax: 64 << 10}
	path := filepath.Join(t.TempDir(), "two.s4wave")
	db := openTest(t, path, opts)

	// Start the child writing to it.
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), childEnv+"="+path)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Write and thin the parent's keys while the child does its own.
	if err := writeKeys(ctx, db, "parent"); err != nil {
		t.Fatal(err)
	}
	if err := deleteKeys(ctx, db, "parent"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("child: %v: %s", err, out.String())
	}

	// Wait for the child's last commit without writing, then check both
	// key sets.
	var seq uint64
	if _, err := fmt.Sscan(out.String(), &seq); err != nil {
		t.Fatalf("child output %q: %v", out.String(), err)
	}
	if err := db.WaitSeq(ctx, seq); err != nil {
		t.Fatal(err)
	}
	m := make(model)
	for i := range childKeys {
		for _, who := range []string{"child", "parent"} {
			if !processDeleted(i) {
				m[string(processKey(who, i))] = processValue(i)
			}
		}
	}
	m.check(t, db)
	checkSpace(t, db)

	// Compact moves the values both processes left into few chunks.
	if err := db.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	u, err := db.Usage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u.ValueChunkBytes > u.ValueBytes+u.ValueBytes/8+2*chunkSize {
		t.Fatalf("compact left %+v", u)
	}
	m.check(t, db)
	checkSpace(t, db)

	// Close, which checkpoints over the child's checkpoints, and check the
	// file.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openTest(t, path, opts)
	defer db.Close()
	m.check(t, db)
}

// TestOpenInProcess checks that a second handle of an open file in one
// process fails.
func TestOpenInProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "one.s4wave")
	db := openTest(t, path, Options{})
	defer db.Close()
	if _, err := Open(path, Options{}); !errors.Is(err, ErrOpenInProcess) {
		t.Fatalf("second open returned %v", err)
	}
}

// TestSingleProcessInBubble opens a single-process database inside a
// synctest bubble. Its goroutines must block durably, so fake time advances
// while it is open, and must all end with Close.
func TestSingleProcessInBubble(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Commit a key.
		ctx := t.Context()
		db := openTest(t, filepath.Join(t.TempDir(), "bubble.s4wave"), Options{SingleProcess: true})
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, []byte("k"), []byte("v")); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}

		// Wait out fake time, then read the key back.
		time.Sleep(time.Minute)
		tx, err = db.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		v, found, err := tx.Get(ctx, []byte("k"))
		tx.Discard()
		if err != nil || !found || string(v) != "v" {
			t.Fatalf("get returned %q, %v, %v", v, found, err)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

// checkCrash copies the file of an open database, as a crash would leave
// it, and checks that a process recovering the copy rebuilds the same
// contents and a consistent space.
func checkCrash(t *testing.T, live *DB, path string, m model) {
	// Copy the file under the writer lock. The background compactor and
	// checkpoint write it otherwise, and a copy read while they do can pair
	// a durable record with value bytes read before they were written.
	t.Helper()
	ctx := context.Background()
	if err := live.w.lock(ctx); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	live.w.unlock(nil)
	if err != nil {
		t.Fatal(err)
	}
	crashed := path + ".crashed"
	if err := os.WriteFile(crashed, b, 0o600); err != nil {
		t.Fatal(err)
	}

	// Recover it and check, then commit more so the rebuilt space is used.
	db := openTest(t, crashed, Options{})
	defer db.Close()
	m.check(t, db)
	checkSpace(t, db)
	c := maps.Clone(m)
	r := rand.New(rand.NewPCG(7, 8))
	for range 50 {
		randomCommit(t, r, db, c, 4000)
		checkSpace(t, db)
	}
	c.check(t, db)
}

// TestScanPrefixLens checks that the index-only scan reports the length of
// inline, packed, and buffered values.
func TestScanPrefixLens(t *testing.T) {
	// Commit an inline and a packed value.
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "lens.s4wave"), Options{})
	defer db.Close()
	want := map[string]int{"a": 10, "b": 64 << 10, "c": 3}
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"a", "b"} {
		if err := tx.Set(ctx, []byte(k), make([]byte, want[k])); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Buffer a third value and scan the lengths.
	tx, err = db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte("c"), make([]byte, want["c"])); err != nil {
		t.Fatal(err)
	}
	got := make(map[string]int)
	if err := tx.(*Tx).ScanPrefixLens(ctx, nil, func(k []byte, n int) error {
		got[string(k)] = n
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !maps.Equal(got, want) {
		t.Fatalf("lengths %v, want %v", got, want)
	}
}

// TestUsage checks that Usage counts live values, that the page classes fit
// in the file, and that Sizes agrees with it and bounds a collection.
func TestUsage(t *testing.T) {
	// Store values too large for the index.
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "usage.s4wave"), Options{})
	defer db.Close()
	tx, _ := db.NewTransaction(ctx, true)
	for i := range 100 {
		if err := tx.Set(ctx, []byte{byte(i)}, make([]byte, 3000)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// The live values and their chunks are counted, and the chunks of each
	// kind make up the file.
	u, err := db.Usage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if u.ValueBytes != 300000 {
		t.Fatalf("ValueBytes = %d, want 300000", u.ValueBytes)
	}
	if u.ValueChunkBytes < u.ValueBytes || u.LogBytes == 0 || u.IndexChunkBytes < u.IndexBytes {
		t.Fatalf("usage %+v lacks value chunks, index chunks, or log", u)
	}
	if sum := u.FreeBytes + u.ValueChunkBytes + u.IndexChunkBytes + u.LogBytes; sum != u.FileBytes {
		t.Fatalf("usage %+v sums to %d, not the file", u, sum)
	}

	// Sizes reports the same file and live value lengths, and a collection
	// cost of the index, the waiting log and the slack.
	sz, err := db.Sizes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if sz.FileBytes != u.FileBytes || sz.ValueBytes != u.ValueBytes {
		t.Fatalf("Sizes = %+v, want %d, %d", sz, u.FileBytes, u.ValueBytes)
	}
	if want := u.IndexBytes + 4*(64<<20) + collectSlackBytes; sz.CollectBytes != want {
		t.Fatalf("CollectBytes = %d, want %d", sz.CollectBytes, want)
	}
}

// TestCheckpointNothingToFold starts a background checkpoint on a state its
// checkpoint already holds, and checks that a snapshot of that state still
// reads every key once freed space is released and punched.
func TestCheckpointNothingToFold(t *testing.T) {
	// Fill a file, without the background cleaner, and checkpoint it.
	ctx := context.Background()
	db := openTest(t, filepath.Join(t.TempDir(), "fold.s4wave"), Options{SingleProcess: true})
	defer db.Close()
	if wait, _ := db.cleaner.SetRoutine(nil); wait != nil {
		<-wait
	}
	r := rand.New(rand.NewPCG(5, 6))
	m := make(model)
	for range 20 {
		randomCommit(t, r, db, m, 1000)
	}

	// Checkpoint, so no commit follows the tree, and read it.
	if err := db.w.lock(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.w.checkpointDurable(ctx); err != nil {
		t.Fatal(err)
	}
	db.w.unlock(nil)
	if st := db.cur.Load(); st.seq != st.ckpt {
		t.Fatalf("state at %d after its checkpoint at %d", st.seq, st.ckpt)
	}
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Start a background checkpoint on it, then release and punch.
	if err := db.w.lock(ctx); err != nil {
		t.Fatal(err)
	}
	db.w.startCheckpoint()
	db.w.unlock(nil)
	db.w.builds.Wait()
	if err := db.w.lock(ctx); err != nil {
		t.Fatal(err)
	}
	err = db.w.releaseNow(ctx)
	db.w.unlock(err)
	if err != nil {
		t.Fatal(err)
	}

	// A background pass over the snapshot reads every key from the file and
	// leaves no leaf in the cache.
	db.p.cache.clear()
	it := tx.Iterate(block.WithBackground(ctx), nil, true, false)
	var seen int
	for it.Next() {
		seen++
	}
	if err := it.Err(); err != nil || seen != len(m) {
		t.Fatalf("background pass saw %d of %d keys, err %v", seen, len(m), err)
	}
	it.Close()
	for i := range db.p.cache.shards {
		for j := range db.p.cache.shards[i].ring {
			if slot := &db.p.cache.shards[i].ring[j]; slot.n != nil && slot.n.leaf {
				t.Fatalf("background pass cached leaf page %d", slot.page)
			}
		}
	}

	// The snapshot reads every key from the file.
	for k, v := range m {
		got, ok, err := tx.Get(ctx, []byte(k))
		if err != nil || !ok || !bytes.Equal(got, v) {
			t.Fatalf("get %x: found %v err %v", k, ok, err)
		}
	}
}

// TestReopenAfterReleasedReplace commits a value and then a replacement,
// ordered and unflushed, closes, and reopens. Close flushes and returns the
// replaced value's pages to the file system, and the reopened file holds the
// replacement: recovery checks the values of records no later record marks
// durable, and the first record's value is gone.
func TestReopenAfterReleasedReplace(t *testing.T) {
	// Commit a value and replace it, without the background cleaner.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "replace.s4wave")
	db := openTest(t, path, Options{SingleProcess: true})
	if wait, _ := db.cleaner.SetRoutine(nil); wait != nil {
		<-wait
	}
	key, want := []byte("k"), bytes.Repeat([]byte{2}, 20000)
	for _, v := range [][]byte{bytes.Repeat([]byte{1}, 20000), want} {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, key, v); err != nil {
			t.Fatal(err)
		}
		if err := tx.(*Tx).CommitOrdered(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// The reopened file holds the replacement.
	db = openTest(t, path, Options{SingleProcess: true})
	defer db.Close()
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	got, ok, err := tx.Get(ctx, key)
	if err != nil || !ok || !bytes.Equal(got, want) {
		t.Fatalf("get: found %v err %v, len %d", ok, err, len(got))
	}
}

// TestCompactDuringBuild deletes every key while a background checkpoint
// builds the tree that holds them, compacts, and closes. The emptied file
// shrinks to a few pages: the build's snapshot pins the space the compaction
// frees, so the compaction waits for the build to end.
func TestCompactDuringBuild(t *testing.T) {
	// Commit enough values that the tree takes many reservations to build.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "build.s4wave")
	db := openTest(t, path, Options{})
	if wait, _ := db.cleaner.SetRoutine(nil); wait != nil {
		<-wait
	}
	val := bytes.Repeat([]byte{1}, 500)
	for b := range 100 {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for i := range 200 {
			if err := tx.Set(ctx, fmt.Appendf(nil, "k%05d", b*200+i), val); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Start a background checkpoint of the tree holding them.
	if err := db.w.lock(ctx); err != nil {
		t.Fatal(err)
	}
	db.w.startCheckpoint()
	db.w.unlock(nil)

	// Delete every key while it builds, and compact.
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 20000 {
		if err := tx.Delete(ctx, fmt.Appendf(nil, "k%05d", i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// The emptied file shrinks to the fixed chunk and the log chunk.
	used, size := usage(t, path)
	if size > 2*chunkSize || used > size {
		t.Fatalf("empty database is %d bytes long with %d allocated", size, used)
	}
}
