//go:build darwin || linux || windows

package s4db

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// ownerEnv selects a child that holds writer ownership until killed or released.
const ownerEnv = "S4DB_TEST_OWNER"

// TestWriterOwner checks exclusion before mutation, live read-only opens, orderly
// handoff, cancellation, alias exclusion and ownership recovery after process death.
func TestWriterOwner(t *testing.T) {
	// A child announces only after recovery and writer ownership have completed.
	if path := os.Getenv(ownerEnv); path != "" {
		db := openTest(t, path, Options{})
		fmt.Println("owned")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	// Test both OS death cleanup and an ordinary writer close.
	for _, death := range []bool{false, true} {
		t.Run(fmt.Sprint(death), func(t *testing.T) {
			// Start an independent writable process on a fresh file.
			path := filepath.Join(t.TempDir(), "owner.s4wave")
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWriterOwner$", "-test.timeout=15s")
			cmd.Env = append(os.Environ(), ownerEnv+"="+path)
			in, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			out, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			cmd.Stderr = os.Stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = in.Close(); _ = cmd.Process.Kill() })
			if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "owned\n" {
				t.Fatalf("owner startup %q: %v", line, err)
			}

			// Rejection happens before reader-slot or recovery writes, including aliases.
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			alias := path + ".alias"
			if err := os.Link(path, alias); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{path, alias} {
				if db, err := Open(name, Options{}); !errors.Is(err, ErrWriterOwned) {
					if db != nil {
						_ = db.Close()
					}
					t.Fatalf("second writer: %v", err)
				}
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatal("rejected writer mutated file")
			}

			// A reader remains usable alongside the writer and rejects writes itself.
			reader := openTest(t, path, Options{ReadOnly: true})
			if _, err := reader.NewTransaction(ctx, true); !errors.Is(err, ErrReadOnly) {
				t.Fatalf("read-only writer: %v", err)
			}
			if err := reader.Close(); err != nil {
				t.Fatal(err)
			}

			// Cancellation returns while the OS still holds the old owner's lock.
			cancelled, stop := context.WithTimeout(ctx, 20*time.Millisecond)
			defer stop()
			if db, err := OpenWait(cancelled, path, Options{}); !errors.Is(err, context.DeadlineExceeded) {
				if db != nil {
					_ = db.Close()
				}
				t.Fatalf("cancelled owner wait: %v", err)
			}

			// Death releases the lifetime lock without a persisted flag or repair.
			if death {
				_ = cmd.Process.Kill()
			} else {
				_ = in.Close()
			}
			err = cmd.Wait()
			if err != nil && !death {
				t.Fatal(err)
			}
			if err := openFiles.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
				return len(openFiles.ids) == 0, nil
			}); err != nil {
				t.Fatal(err)
			}
			db, err := OpenWait(ctx, path, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
