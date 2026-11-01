package s4db

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// admissionEnv selects the process that pauses after announcing foreground intent.
const admissionEnv = "S4DB_TEST_ADMISSION"

// runAdmission pauses between beginWait and take, the production foreground
// protocol's two steps, so the parent can exercise a remote background contender.
func runAdmission(t *testing.T, path string) {
	// Hold the file without automatic maintenance changing the handoff.
	db := openTest(t, path, Options{})
	defer db.Close()
	db.sched.stop()
	ctx := t.Context()

	// Announce a foreground request, then let the parent choose take or exit.
	if err := db.w.beginWait(); err != nil {
		t.Fatal(err)
	}
	fmt.Println("announced")
	command, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if command == "exit\n" {
		return
	}

	// Finish the foreground protocol and publish a value before any background
	// callback proceeds. Exiting above leaves the OS to drop the intent instead.
	if err := db.w.take(ctx); err != nil {
		t.Fatal(err)
	}
	db.w.endWait()
	err = db.w.commit(ctx, db.cur.Load(), []tentry{{key: []byte("admitted"), data: []byte("child")}}, false)
	db.w.unlock()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.flush.syncThrough(ctx, db.cur.Load().seq); err != nil {
		t.Fatal(err)
	}
}

// startAdmission starts a real process with an announced foreground request.
func startAdmission(t *testing.T, ctx context.Context, path string) (*exec.Cmd, io.WriteCloser) {
	// Start the same test binary and retain its input to select take or exit.
	t.Helper()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWriterAdmission$", "-test.timeout=20s")
	cmd.Env = append(os.Environ(), admissionEnv+"="+path)

	// Wire both protocol pipes before starting the child.
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	// Start the child after its protocol endpoints are ready.
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = in.Close(); _ = cmd.Process.Kill() })

	// The ready line establishes that intent is held before the parent acts.
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || line != "announced\n" {
		t.Fatalf("child announcement %q: %v", line, err)
	}
	return cmd, in
}

// TestWriterAdmission checks remote intent, successful handoff, process death
// and cancellation of a local queued transaction against the real writer locks.
func TestWriterAdmission(t *testing.T) {
	// A child executes the controlled foreground half of the same protocol.
	if path := os.Getenv(admissionEnv); path != "" {
		runAdmission(t, path)
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	// Foreground intent excludes a closing mark and runs before background work.
	for _, command := range []string{"write", "exit"} {
		t.Run(command, func(t *testing.T) {
			// Wait for a child to announce its request on its own slot byte.
			path := filepath.Join(t.TempDir(), "admission.s4wave")
			db := openTest(t, path, Options{})
			defer db.Close()
			db.sched.stop()
			cmd, in := startAdmission(t, ctx, path)

			// Optional background work must not pass that announced intent.
			ok, err := db.w.tryLock(ctx)
			if ok {
				db.w.unlock()
			}
			if err != nil || ok {
				t.Fatalf("closing mark passed foreground intent: %t %v", ok, err)
			}

			// The background callback can proceed only after take or process exit.
			done := make(chan error, 1)
			go func() {
				done <- db.w.locked(ctx, func() error {
					if command == "exit" {
						return nil
					}
					v, found, err := db.lookup(db.cur.Load(), []byte("admitted"))
					if err != nil || !found || !bytes.Equal(v.inline, []byte("child")) {
						return fmt.Errorf("background passed foreground commit: %t %v", found, err)
					}
					return nil
				})
			}()
			if _, err := io.WriteString(in, command+"\n"); err != nil {
				t.Fatal(err)
			}
			in.Close()
			if err := cmd.Wait(); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}

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
