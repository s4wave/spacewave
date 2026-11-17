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

// TestWriterOwnerBuildCleanup keeps partial-file cleanup inside ownership and
// refuses a waiting descriptor after its builder removed the failed file.
func TestWriterOwnerBuildCleanup(t *testing.T) {
	// Hold a real builder in another process until the parent has opened its wait.
	const buildEnv = "S4DB_TEST_BUILD_OWNER"
	if path := os.Getenv(buildEnv); path != "" {
		failed := errors.New("source failed")
		err := Build(t.Context(), path, Options{}, func(func([]byte, []byte) error) error {
			fmt.Println("building")
			_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
			return failed
		})
		if !errors.Is(err, failed) {
			t.Fatal(err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	path := filepath.Join(t.TempDir(), "failed-build.s4wave")
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWriterOwnerBuildCleanup$", "-test.timeout=15s")
	cmd.Env = append(os.Environ(), buildEnv+"="+path)
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
	t.Cleanup(func() { _ = in.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() })
	if line, err := bufio.NewReader(out).ReadString('\n'); err != nil || line != "building\n" {
		t.Fatalf("builder startup %q: %v", line, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	// Observe real descriptor registration, not merely the wait goroutine's start.
	done := make(chan struct{})
	var waited *DB
	var waitErr error
	go func() {
		waited, waitErr = OpenWait(ctx, path, Options{})
		close(done)
	}()
	t.Cleanup(func() {
		// Release the remote owner before joining the cancelled native open.
		cancel()
		_ = in.Close()
		_ = cmd.Process.Kill()
		<-done
		if waited != nil {
			_ = waited.Close()
		}
	})
	for {
		registered := false
		openFiles.bcast.HoldLock(func(func(), func() <-chan struct{}) {
			for _, held := range openFiles.ids {
				registered = registered || os.SameFile(info, held)
			}
		})
		if registered {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}

	// Failure removes the partial path before handing the native byte to a waiter.
	_ = in.Close()
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if waited != nil || waitErr == nil {
		t.Fatalf("failed-build waiter returned writable handle %v, error %v", waited != nil, waitErr)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial build remains: %v", err)
	}
	db := openTest(t, path, Options{})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestWriterAdmission retains local overlapping demand and cancellation coverage.
func TestWriterAdmission(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	// A background step yields once, then runs behind a stream that always has
	// its next request announced, as a tight commit loop on cheap syncs does.
	t.Run("steady", func(t *testing.T) {
		// Two overlapping requests keep this handle's intent held throughout.
		db := openTest(t, filepath.Join(t.TempDir(), "steady.s4wave"), Options{})
		defer db.Close()
		db.sched.stop()
		for range 2 {
			if err := db.w.beginWait(); err != nil {
				t.Fatal(err)
			}
		}
		defer db.w.endWait()

		// The step sees the first request and yields to it.
		ran := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- db.w.locked(ctx, func() error {
				close(ran)
				return nil
			})
		}()
		select {
		case <-ran:
			t.Fatal("background step passed an announced request")
		case <-time.After(50 * time.Millisecond):
		}

		// Serving one request lets the step run though the other is announced.
		db.w.endWait()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	})

	// Cancelling before the local mutex grants a transaction drops its intent.
	t.Run("cancel", func(t *testing.T) {
		// Hold the local writer while a real transaction announces itself.
		db := openTest(t, filepath.Join(t.TempDir(), "cancel.s4wave"), Options{})
		defer db.Close()
		db.sched.stop()
		release, err := db.w.mtx.Lock(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer release()

		// Queue a cancellable transaction behind the held local writer.
		request, stop := context.WithCancel(ctx)
		defer stop()
		done := make(chan error, 1)
		go func() {
			tx, err := db.NewTransaction(request, true)
			if tx != nil {
				tx.Discard()
			}
			done <- err
		}()

		// Wait on the announcement itself, then cancel and verify its cleanup.
		if err := db.w.requests.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
			return db.w.waiting.Load() != 0, nil
		}); err != nil {
			t.Fatal(err)
		}
		stop()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled transaction: %v", err)
		}
		if n := db.w.waiting.Load(); n != 0 {
			t.Fatalf("cancelled request retains %d intents", n)
		}
	})
}
