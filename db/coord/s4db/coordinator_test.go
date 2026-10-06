//go:build !js && !wasip1

package s4db

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/coord"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
	db_s4db "github.com/s4wave/spacewave/db/s4db"
)

// childEnv names the database file a child process of
// TestCoordinatorProcesses opens.
const childEnv = "SPACEWAVE_COORD_S4DB_CHILD"

// testScope is the ObjectStore scope the processes share.
var testScope = coord.Scope{VolumeID: "volume", ObjectStoreID: "objects"}

// TestMain runs the child of TestCoordinatorProcesses when the test binary
// is started as one.
func TestMain(m *testing.M) {
	if path := os.Getenv(childEnv); path != "" {
		if err := runChild(path, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runChild takes the write lease, commits, and reports "held <seq>". On
// "parent <seq>" it waits for its watch to report that commit and replies
// "saw". It holds the lease until killed or until in closes.
func runChild(path string, in io.Reader, out io.Writer) error {
	// Open the file and take the lease.
	ctx := context.Background()
	db, err := db_s4db.Open(path, db_s4db.Options{})
	if err != nil {
		return err
	}
	defer db.Close()
	c := NewCoordinator(db, coord_inmem.NewCoordinator())
	lease, err := c.WaitAcquireWriteLease(ctx, testScope)
	if err != nil {
		return err
	}
	defer lease.Release(ctx)

	// Watch, commit, and report the commit.
	watch, err := c.Watch(ctx, testScope, db.Seq())
	if err != nil {
		return err
	}
	defer watch.Close()
	if err := commit(ctx, db, "child"); err != nil {
		return err
	}
	fmt.Fprintln(out, "held", db.Seq())

	// Wait for the parent's commit through the watch.
	var seq uint64
	if _, err := fmt.Fscanln(in, new(string), &seq); err != nil {
		return err
	}
	if err := waitGeneration(ctx, watch, seq); err != nil {
		return err
	}
	fmt.Fprintln(out, "saw")

	// Hold the lease until killed.
	_, err = io.Copy(io.Discard, in)
	return err
}

// TestCoordinatorProcesses checks that a child process holding the write
// lease excludes the parent, that each process watches the other's commits,
// and that killing the child releases the lease.
func TestCoordinatorProcesses(t *testing.T) {
	// Open the file and watch it.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "volume.s4wave")
	db := openTestDB(t, path)
	c := NewCoordinator(db, coord_inmem.NewCoordinator())
	watch, err := c.Watch(ctx, testScope, db.Seq())
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()

	// Connect a child process through its standard input and output.
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), childEnv+"="+path)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	// Start it and wait until it holds the lease and committed.
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	out := bufio.NewReader(stdout)
	var childSeq uint64
	if _, err := fmt.Fscanln(out, new(string), &childSeq); err != nil {
		t.Fatalf("read child commit: %v", err)
	}

	// The lease is busy, and the watch reports the child's commit.
	if l, ok, err := c.TryAcquireWriteLease(ctx, testScope); err != nil || ok {
		t.Fatalf("TryAcquireWriteLease while the child holds it = %v, %v, %v", l, ok, err)
	}
	if err := waitGeneration(ctx, watch, childSeq); err != nil {
		t.Fatalf("watch the child's commit: %v", err)
	}

	// The child's watch reports the parent's commit.
	if err := commit(ctx, db, "parent"); err != nil {
		t.Fatal(err)
	}
	seq := db.Seq()
	if _, err := fmt.Fprintln(stdin, "parent", seq); err != nil {
		t.Fatal(err)
	}
	if line, err := out.ReadString('\n'); err != nil || line != "saw\n" {
		t.Fatalf("child reply = %q, %v", line, err)
	}

	// Killing the child releases the lease.
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	lease, err := c.WaitAcquireWriteLease(ctx, testScope)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := lease.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation != seq {
		t.Fatalf("Refresh generation = %d, want %d", snapshot.Generation, seq)
	}
	if err := lease.Release(ctx); err != nil {
		t.Fatal(err)
	}
}

// commit writes one key named for who.
func commit(ctx context.Context, db *db_s4db.DB, who string) error {
	// Open a write transaction, discarded unless it commits.
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Set the key and commit.
	if err := tx.Set(ctx, []byte(who), []byte(who)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// waitGeneration reads watch until an event reaches generation.
func waitGeneration(ctx context.Context, watch coord.Watch, generation uint64) error {
	for {
		select {
		case event, ok := <-watch.Events():
			if !ok {
				return io.ErrUnexpectedEOF
			}
			if event.Generation >= generation {
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
