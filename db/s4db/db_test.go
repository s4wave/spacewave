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
	"testing"

	"github.com/pkg/errors"
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
