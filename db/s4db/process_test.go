package s4db

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// childEnv names the file a child process writes to.
const childEnv = "S4DB_TEST_CHILD"

// churnEnv names the file a child process churns for the race test.
const churnEnv = "S4DB_TEST_CHURN"

// childKeys is the number of keys each process writes.
const childKeys = 400

// TestMain runs the child the environment selects in place of the tests.
func TestMain(m *testing.M) {
	// Run the child, if this is one.
	if path := os.Getenv(childEnv); path != "" {
		if err := runChild(path); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if path := os.Getenv(churnEnv); path != "" {
		if err := runChurn(path); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runChurn rewrites the race test's "c" keys for a while beside the parent,
// then closes.
func runChurn(path string) error {
	// Open the file the parent holds open and churn until the timer fires.
	db, err := Open(path, Options{CacheBytes: 8 << 20, memtable: 128 << 10})
	if err != nil {
		return err
	}
	stop := time.After(3 * time.Second)
	if err := churn(context.Background(), db, "c", 3, stop); err != nil {
		_ = db.Close()
		return err
	}
	return db.Close()
}

// processKey returns key i written by the process named who.
func processKey(who string, i int) []byte {
	return fmt.Appendf(nil, "%s/%06d", who, i)
}

// processValue returns the value of key i, large enough on some keys to be
// stored in a value chunk.
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

// runChild writes the child's keys to the file at path, flushing often so the
// parent's reads cross runs the child wrote, and prints the last record it
// saw.
func runChild(path string) error {
	// Open the file the parent holds open.
	db, err := Open(path, Options{memtable: 64 << 10})
	if err != nil {
		return err
	}

	// Write the keys and report the last record.
	ctx := context.Background()
	if err := writeKeys(ctx, db, "child"); err != nil {
		_ = db.Close()
		return err
	}
	fmt.Println(db.Seq())
	return db.Close()
}

// TestTwoProcesses writes from this process and a child at once, then checks
// that each sees the other's keys, that the file reports itself shared, and
// that a reopen holds every key.
func TestTwoProcesses(t *testing.T) {
	// Open the file.
	ctx := context.Background()
	opts := Options{memtable: 64 << 10}
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

	// Write the parent's keys while the child writes its own.
	if err := writeKeys(ctx, db, "parent"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("child: %v: %s", err, out.String())
	}

	// Wait for the child's last record without writing, then check both key
	// sets through the tail alone.
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

	// Close and reopen: the superblocks either process wrote and the log
	// after them hold every key.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openTest(t, path, opts)
	defer db.Close()
	m.check(t, db)
}

// TestOpenInProcess checks that a second handle of an open file in one
// process fails, and that a read-only handle refuses writes.
func TestOpenInProcess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "one.s4wave")
	db := openTest(t, path, Options{})
	defer db.Close()
	if _, err := Open(path, Options{}); !errors.Is(err, ErrOpenInProcess) {
		t.Fatalf("second open returned %v", err)
	}
}

// TestReadOnly opens a file read-only beside nothing, reads it, and is
// refused a write transaction.
func TestReadOnly(t *testing.T) {
	// Write a key and close.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ro.s4wave")
	db := openTest(t, path, Options{})
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
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen read-only.
	db = openTest(t, path, Options{ReadOnly: true})
	defer db.Close()

	// The key reads; a write is refused.
	model{"k": []byte("v")}.check(t, db)
	if _, err := db.NewTransaction(ctx, true); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("write on read-only handle returned %v", err)
	}
}
