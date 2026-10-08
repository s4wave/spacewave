package main

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
)

// TestReadyMarker verifies that even a stale ready marker blocks the tool.
func TestReadyMarker(t *testing.T) {
	// Create an isolated stale marker without consulting any real state directory.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "spacewave.sock.ready"), []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := requireOffline(dir); err == nil {
		t.Fatal("ready marker was ignored")
	}
}

// TestLiveLocks proves refusal against real kernel locks held by another process.
func TestLiveLocks(t *testing.T) {
	// Launch the helper only when explicitly selected by the parent test.
	if dir := os.Getenv("SO_MIGRATION_LOCK_FIXTURE"); dir != "" {
		// Hold the owning runtime or database lock until the parent closes stdin.
		var file *os.File
		var err error
		if os.Getenv("SO_MIGRATION_LOCK_KIND") == "runtime" {
			file, err = acquireLease(dir)
		}
		if os.Getenv("SO_MIGRATION_LOCK_KIND") == "database" {
			file, err = os.Open(filepath.Join(dir, "fixture.s4wave"))
			if err == nil {
				err = syscall.Flock(int(file.Fd()), syscall.LOCK_SH|syscall.LOCK_NB)
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := file.Close(); err != nil {
				t.Error(err)
			}
		}()
		if _, err := io.WriteString(os.Stdout, "locked\n"); err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			t.Fatal(err)
		}
		return
	}

	// Exercise each ownership fence using pipe events, without polling or timers.
	for _, kind := range []string{"runtime", "database"} {
		t.Run(kind, func(t *testing.T) {
			// Create the existing lease and database lock targets in an isolated directory.
			dir := t.TempDir()
			for _, name := range []string{"runtime-lease.s4wave-lock-coord", "fixture.s4wave"} {
				if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestLiveLocks$")
			cmd.Env = append(os.Environ(), "SO_MIGRATION_LOCK_FIXTURE="+dir, "SO_MIGRATION_LOCK_KIND="+kind)
			input, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			output, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				// Closing the input pipe signals the helper to release its kernel lock.
				if err := input.Close(); err != nil {
					t.Error(err)
				}
				if err := cmd.Wait(); err != nil {
					t.Error(err)
				}
			})

			// Wait for the helper's lock-acquired event before attempting maintenance.
			ready, err := bufio.NewReader(output).ReadString('\n')
			if err != nil || ready != "locked\n" {
				t.Fatalf("lock helper did not report ownership: %v", err)
			}
			if kind == "runtime" {
				if lease, err := acquireLease(dir); err == nil {
					if err := lease.Close(); err != nil {
						t.Fatal(err)
					}
					t.Fatal("live runtime lease was ignored")
				}
			}
			if kind == "database" {
				if db, err := openDatabase(filepath.Join(dir, "fixture.s4wave"), true); err == nil {
					if err := db.Close(); err != nil {
						t.Fatal(err)
					}
					t.Fatal("live database holder was ignored")
				}
			}
		})
	}
}
