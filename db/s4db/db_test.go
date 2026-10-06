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
	"slices"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
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

// model is the expected contents of a database.
type model map[string][]byte

// check compares every key and a full scan of db against m.
func (m model) check(t *testing.T, db *DB) {
	// Read every key in a read transaction.
	t.Helper()
	ctx := context.Background()
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	keys := make([]string, 0, len(m))
	for k, v := range m {
		keys = append(keys, k)
		got, ok, err := tx.Get(ctx, []byte(k))
		if err != nil || !ok || !bytes.Equal(got, v) {
			t.Fatalf("get %x: found %v err %v, len %d want %d", k, ok, err, len(got), len(v))
		}
	}

	// A forward scan returns the keys in order.
	slices.Sort(keys)
	var scanned []string
	if err := tx.ScanPrefixKeys(ctx, nil, func(k []byte) error {
		scanned = append(scanned, string(k))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(keys, scanned) {
		t.Fatalf("scan returned %d keys, want %d", len(scanned), len(keys))
	}

	// A reverse scan returns them backward, and Size counts them.
	it := tx.Iterate(ctx, nil, true, true)
	for i, key := range slices.Backward(keys) {
		if !it.Next() || string(it.Key()) != key {
			t.Fatalf("reverse scan diverged at %d: %v", i, it.Err())
		}
	}
	it.Close()
	if n, err := tx.Size(ctx); err != nil || n != uint64(len(m)) {
		t.Fatalf("size %d err %v, want %d", n, err, len(m))
	}
}

// randomCommit applies a random batch to db and m.
func randomCommit(t *testing.T, r *rand.Rand, db *DB, m model, keys int) {
	// Open a write transaction.
	t.Helper()
	ctx := context.Background()
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Set or delete up to 64 random keys.
	for range 1 + r.IntN(64) {
		k := fmt.Sprintf("k/%06d", r.IntN(keys))
		if r.IntN(4) == 0 {
			delete(m, k)
			if err := tx.Delete(ctx, []byte(k)); err != nil {
				t.Fatal(err)
			}
			continue
		}
		v := make([]byte, []int{8, 100, 600, 3000, 20000}[r.IntN(5)])
		for i := range v {
			v[i] = byte(r.Uint32())
		}
		m[k] = v
		if err := tx.Set(ctx, []byte(k), v); err != nil {
			t.Fatal(err)
		}
	}

	// Commit durably or ordered at random.
	if r.IntN(2) == 0 {
		err = tx.(kvtx.OrderedCommitTx).CommitOrdered(ctx)
	} else {
		err = tx.Commit(ctx)
	}
	if err != nil {
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
		checkCrash(t, path, m)
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

	// The emptied file shrinks to a few pages.
	if err := db.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	m.check(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	used, size := usage(t, path)
	t.Logf("empty file: %d bytes long, %d allocated", size, used)
	if used > 256<<10 || size > 2<<20 {
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

// childEnv names the file a child process of TestTwoProcesses writes.
const childEnv = "S4DB_TEST_CHILD"

// childKeys is the number of keys each process of TestTwoProcesses writes.
const childKeys = 400

// TestMain runs the child of TestTwoProcesses when the test binary is
// started as one.
func TestMain(m *testing.M) {
	if path := os.Getenv(childEnv); path != "" {
		if err := runChild(path); err != nil {
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
	return bytes.Repeat([]byte{byte(i)}, []int{16, 900, 5000}[i%3])
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
	// Open the file the parent holds open.
	db, err := Open(path, Options{CheckpointMin: 32 << 10, CheckpointMax: 64 << 10})
	if err != nil {
		return err
	}

	// Write the keys and report the last commit.
	ctx := context.Background()
	if err := writeKeys(ctx, db, "child"); err != nil {
		_ = db.Close()
		return err
	}
	fmt.Println(db.Seq())
	return db.Close()
}

// TestTwoProcesses writes one file from two processes at once and checks
// that each commit lands, the space stays consistent, and the parent sees
// the child's commits through the change watcher.
func TestTwoProcesses(t *testing.T) {
	// Open the file.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "two.s4wave")
	db := openTest(t, path, Options{CheckpointMin: 32 << 10, CheckpointMax: 64 << 10})
	defer db.Close()

	// Start the child writing to it.
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), childEnv+"="+path)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Write the parent's keys while the child writes its own.
	if err := writeKeys(ctx, db, "parent"); err != nil {
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
			m[string(processKey(who, i))] = processValue(i)
		}
	}
	m.check(t, db)
	checkSpace(t, db)
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

// checkCrash copies the file of an open database, as a crash would leave
// it, and checks that a process recovering the copy rebuilds the same
// contents and a consistent space.
func checkCrash(t *testing.T, path string, m model) {
	// Copy the file.
	t.Helper()
	b, err := os.ReadFile(path)
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

// checkSpace fails when two uses claim one page: a live value, a tree
// page, a log chunk, or the saved space against each other or against a
// free or pending run.
func checkSpace(t *testing.T, db *DB) {
	// Hold the writer lock so the space is stable.
	t.Helper()
	if err := db.w.lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer db.w.unlock()
	sp, st := db.w.sp, db.cur.Load()
	owner := make(map[uint64]string)
	claim := func(p uint64, who string, shared bool) {
		if prev, ok := owner[p]; ok && (!shared || prev != who) {
			t.Fatalf("page %d claimed by %s and %s", p, prev, who)
		}
		owner[p] = who
	}

	// Claim the pages under every stored value, the tree, the log chunks,
	// and the saved space.
	it := newIterator(db.p, st, nil, nil, false)
	for it.Next() {
		if v := it.cur.val; v.isRef {
			spans(v.ref.off, int(v.ref.n), func(p uint64, _ uint32) { claim(p, "value", true) })
		}
	}
	var walk func(p uint64)
	walk = func(p uint64) {
		claim(p, "tree", false)
		n, err := db.p.node(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range n.kids {
			walk(c)
		}
	}
	if st.root != 0 {
		walk(st.root)
	}
	for _, c := range sp.chunks {
		for _, p := range pageRange(c.start, c.n) {
			claim(p, "log", false)
		}
	}
	for _, p := range pageRange(st.sb.space.off/pageSize, pagesFor(int(st.sb.space.n))) {
		claim(p, "space", false)
	}

	// No claimed page may be free, pending, or past the end.
	for p, who := range owner {
		if p >= sp.end {
			t.Fatalf("%s page %d past the end %d", who, p, sp.end)
		}
		if r, ok := sp.before(p + 1); ok && p < r.end() {
			t.Fatalf("%s page %d in free run %+v", who, p, r)
		}
		for _, x := range slices.Concat(sp.values, sp.pages) {
			if p >= x.start && p < x.end() {
				t.Fatalf("%s page %d awaits release in %+v", who, p, x)
			}
		}
	}
}
