//go:build !js

package checkoutroot

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/sirupsen/logrus"
)

// TestUploadStageSweep checks that cleanup retains active and unowned files.
func TestUploadStageSweep(t *testing.T) {
	// Place a named payload and its locked owner marker in a nested directory.
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	if err := root.Mkdir("nested", 0o700); err != nil {
		t.Fatal(err)
	}
	dir, err := root.OpenRoot("nested")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = dir.Close() })
	journal := newUploadJournal(hashmap.NewHashmapKvtx(hashmap.NewBTreeMap[[]byte]()), "test")
	stage, err := openNamedStage(t.Context(), journal, dir, "nested")
	if err != nil {
		t.Fatal(err)
	}
	defer stage.discard(t.Context())
	if _, err := stage.file.WriteString("partial"); err != nil {
		t.Fatal(err)
	}

	// Keep an ordinary file with no lease beside the hidden active payload.
	unowned := uploadStagePrefix + strings.Repeat("A", 26)
	if err := dir.WriteFile(unowned, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !hiddenUploadName(stage.name) || !hiddenUploadName(stage.name+uploadLeaseSuffix) {
		t.Fatal("staging entries must be hidden")
	}
	le := logrus.New().WithField("test", t.Name())
	journal.sweepUploads(t.Context(), le, root)
	if _, err := dir.Stat(stage.name); err != nil {
		t.Fatalf("active upload was removed: %v", err)
	}

	// Drop the open handles as a killed publisher would and reclaim its stage.
	if err := errors.Join(stage.file.Close(), stage.lease.Close()); err != nil {
		t.Fatal(err)
	}
	journal.sweepUploads(t.Context(), le, root)
	for _, name := range []string{stage.name, stage.name + uploadLeaseSuffix} {
		if _, err := dir.Stat(name); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("abandoned stage %s was retained: %v", name, err)
		}
	}
	if data, err := dir.ReadFile(unowned); err != nil || string(data) != "keep" {
		t.Fatalf("unowned file = %q, error %v", data, err)
	}
}

// TestUploadJournalRecoversInterruptedStage checks recovery after the kernel releases a killed publisher's lease.
func TestUploadJournalRecoversInterruptedStage(t *testing.T) {
	// The child uses the same journal database but gets its own OS lease.
	const fixtureEnv = "SPACEWAVE_UPLOAD_JOURNAL_FIXTURE"
	if fixture := os.Getenv(fixtureEnv); fixture != "" {
		runInterruptedUpload(t, fixture)
		return
	}
	fixture := t.TempDir()
	folder := filepath.Join(fixture, "checkout")
	for _, path := range []string{filepath.Join(folder, "nested"), filepath.Join(folder, "other")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	// Kill the writer only once it has persisted the journal and written partial bytes.
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestUploadJournalRecoversInterruptedStage$")
	cmd.Env = append(os.Environ(), fixtureEnv+"="+fixture)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	var name string
	if _, err := fmt.Fscanln(stdout, &name); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("the interrupted publisher exited successfully")
	}

	// Reopen the on-disk journal as the next daemon start would.
	db, err := s4db.Open(filepath.Join(fixture, "state.s4wave"), s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	journal := newUploadJournal(db, "test")
	root, err := os.OpenRoot(folder)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()

	// A journal hint cannot make an unmarked host file ours.
	lookalike := uploadStagePrefix + strings.Repeat("A", 26)
	if err := root.WriteFile("nested/"+lookalike, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := journal.record(t.Context(), lookalike, "nested"); err != nil {
		t.Fatal(err)
	}

	// With no location in this journal, even a valid stale lease is out of scope.
	unindexed := uploadStagePrefix + strings.Repeat("B", 26)
	if err := root.WriteFile("other/"+unindexed, []byte("untouched"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := root.WriteFile("other/"+unindexed+uploadLeaseSuffix, []byte(uploadLeaseMarker), 0o600); err != nil {
		t.Fatal(err)
	}

	// Startup reclaims just the interrupted stage and retires its journal row.
	journal.sweepUploads(t.Context(), logrus.New().WithField("test", t.Name()), root)
	for _, path := range []string{"nested/" + name, "nested/" + name + uploadLeaseSuffix} {
		if _, err := root.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("interrupted stage %s was retained: %v", path, err)
		}
	}
	for path, want := range map[string]string{"nested/" + lookalike: "keep", "other/" + unindexed: "untouched"} {
		if data, err := root.ReadFile(path); err != nil || string(data) != want {
			t.Errorf("read %s: got %q, error %v, want %q", path, data, err, want)
		}
	}
	key, _, err := journal.next(t.Context(), "")
	if err != nil || key != "" {
		t.Fatalf("journal after recovery: key %q, error %v", key, err)
	}
}

// runInterruptedUpload leaves a partially written stage open until the parent kills the process.
func runInterruptedUpload(t *testing.T, fixture string) {
	// Use a durable state volume outside the linked folder.
	db, err := s4db.Open(filepath.Join(fixture, "state.s4wave"), s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	journal := newUploadJournal(db, "test")
	dir, err := os.OpenRoot(filepath.Join(fixture, "checkout", "nested"))
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()

	// Signal over the pipe only when both the on-disk location and the bytes exist.
	stage, err := openNamedStage(t.Context(), journal, dir, "nested")
	if err != nil {
		t.Fatal(err)
	}
	defer stage.discard(t.Context())
	if _, err := stage.file.WriteString("partial bytes"); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(os.Stdout, stage.name); err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
}
