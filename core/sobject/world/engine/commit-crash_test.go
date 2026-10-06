//go:build !js

package sobject_world_engine_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/world"
)

// TestLocalWorldCommitCrash cuts the process at each point of a local World
// commit and checks the World a restarted engine reads: before the physical
// commit, after it with the replay save it defers, and after the next commit
// carries that save. A restarted engine replays from its saved cursor, reads
// every committed object and commits again.
func TestLocalWorldCommitCrash(t *testing.T) {
	const roleEnv = "SPACEWAVE_WORLD_COMMIT_CRASH_ROLE"
	if role := os.Getenv(roleEnv); role != "" {
		worldCommitCrashWorker(t, role, os.Getenv("SPACEWAVE_WORLD_COMMIT_CRASH_DIR"))
		return
	}
	for _, cut := range []string{"before", "after", "derived"} {
		t.Run(cut, func(t *testing.T) {
			dir := t.TempDir()
			for _, step := range []struct {
				role string
				code int
			}{{"seed", 0}, {cut, 74}, {"verify-" + cut, 0}} {
				ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
				cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLocalWorldCommitCrash$", "-test.timeout=50s")
				cmd.Env = append(os.Environ(), roleEnv+"="+step.role, "SPACEWAVE_WORLD_COMMIT_CRASH_DIR="+dir)
				out, err := cmd.CombinedOutput()
				cancel()
				if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != step.code {
					t.Fatalf("%s: %v\n%s", step.role, err, out)
				}
			}
		})
	}
}

// worldCommitCrashWorker runs one step of the crash test on the s4db storage
// under dir.
func worldCommitCrashWorker(t *testing.T, role, dir string) {
	// Start the World, creating it in the seed step.
	refPath := filepath.Join(dir, "so-ref")
	var soRef *sobject.SharedObjectRef
	if role != "seed" {
		data, err := os.ReadFile(refPath)
		if err != nil {
			t.Fatal(err)
		}
		soRef = &sobject.SharedObjectRef{}
		if err := soRef.UnmarshalVT(data); err != nil {
			t.Fatal(err)
		}
	}
	storage := filepath.Join(dir, "storage")
	if err := os.MkdirAll(storage, 0o700); err != nil {
		t.Fatal(err)
	}
	w := startLocalWorld(t, storage, soRef)

	// Seed two committed objects and save the SharedObject.
	switch role {
	case "seed":
		w.create(t, "crash/0")
		w.create(t, "crash/1")
		data, err := w.soRef.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(refPath, data, 0o600); err != nil {
			t.Fatal(err)
		}
		return

	// Cut before the physical commit of crash/2.
	case "before":
		_ = w.newWrite(t, "crash/2")
		os.Exit(74)

	// Cut after it, with its replay save deferred.
	case "after":
		w.create(t, "crash/2")
		os.Exit(74)

	// Cut after crash/3 published the replay save of crash/2.
	case "derived":
		w.create(t, "crash/2")
		w.create(t, "crash/3")
		os.Exit(74)
	}

	// Read the objects the cut committed, then commit again.
	want := map[string][]string{
		"verify-before":  {"crash/0", "crash/1"},
		"verify-after":   {"crash/0", "crash/1", "crash/2"},
		"verify-derived": {"crash/0", "crash/1", "crash/2", "crash/3"},
	}[role]
	if want == nil {
		t.Fatalf("unknown role %q", role)
	}
	tx, err := w.eng.NewTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"crash/0", "crash/1", "crash/2", "crash/3"} {
		obj, found, err := tx.GetObject(t.Context(), key)
		if err != nil {
			t.Fatal(err)
		}
		if found {
			_, _, err := obj.GetRootRef(t.Context())
			world.ReleaseObjectState(obj)
			if err != nil {
				t.Fatalf("read %s: %v", key, err)
			}
		}
		if committed := slices.Contains(want, key); found != committed {
			t.Fatalf("%s: found = %v, want %v", key, found, committed)
		}
	}
	tx.Discard()
	w.create(t, "crash/next")
}
