package artifact

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPublishLastCompleteWinsAndIgnoresPartialGeneration(t *testing.T) {
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	storeDir := filepath.Join(t.TempDir(), "store")

	releaseA, prerenderA := newArtifactFixture(t, "first")
	publishedRelease, publishedPrerender, err := Publish(storeDir, releaseA, prerenderA, identity)
	if err != nil {
		t.Fatal(err)
	}
	assertArtifactMarker(t, publishedRelease, publishedPrerender, "first")

	partialDir := filepath.Join(storeDir, ".publish-killed")
	writeTestFile(t, filepath.Join(partialDir, "release", "browser-release.json"), "{}")
	writeTestFile(t, filepath.Join(partialDir, "prerender", "index.html"), "partial")
	currentRelease, currentPrerender, err := Current(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	assertArtifactMarker(t, currentRelease, currentPrerender, "first")

	releaseB, prerenderB := newArtifactFixture(t, "second")
	if _, _, err := Publish(storeDir, releaseB, prerenderB, identity); err != nil {
		t.Fatal(err)
	}
	currentRelease, currentPrerender, err = Current(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	assertArtifactMarker(t, currentRelease, currentPrerender, "second")
}

func TestConcurrentPublishNeverInterleavesOutputs(t *testing.T) {
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	storeDir := filepath.Join(t.TempDir(), "store")

	var wg sync.WaitGroup
	for _, marker := range []string{"one", "two", "three", "four"} {
		releaseDir, prerenderDir := newArtifactFixture(t, marker)
		wg.Go(func() {
			if _, _, err := Publish(storeDir, releaseDir, prerenderDir, identity); err != nil {
				t.Errorf("publish %s: %v", marker, err)
			}
		})
	}
	wg.Wait()

	releaseDir, prerenderDir, err := Current(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	releaseMarker, err := os.ReadFile(filepath.Join(releaseDir, "marker.txt"))
	if err != nil {
		t.Fatal(err)
	}
	prerenderMarker, err := os.ReadFile(filepath.Join(prerenderDir, "marker.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(releaseMarker) != string(prerenderMarker) {
		t.Fatalf("interleaved artifact outputs: release=%q prerender=%q", releaseMarker, prerenderMarker)
	}
}

func TestValidateRejectsPartialAndModifiedArtifacts(t *testing.T) {
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())

	partialRelease := filepath.Join(t.TempDir(), "release")
	partialPrerender := filepath.Join(t.TempDir(), "prerender")
	writeTestFile(t, filepath.Join(partialRelease, "browser-release.json"), "{}")
	writeTestFile(t, filepath.Join(partialPrerender, "index.html"), "<!doctype html>")
	if err := Validate(partialRelease, partialPrerender, identity); err == nil {
		t.Fatal("partial artifact validated")
	}

	releaseDir, prerenderDir := newArtifactFixture(t, "complete")
	publishedRelease, publishedPrerender, err := Publish(filepath.Join(t.TempDir(), "store"), releaseDir, prerenderDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(publishedRelease, publishedPrerender, identity); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(publishedRelease, "marker.txt"), "truncated")
	if err := Validate(publishedRelease, publishedPrerender, identity); err == nil {
		t.Fatal("modified artifact validated")
	}
}

func TestValidateRejectsStaleIdentity(t *testing.T) {
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	releaseDir, prerenderDir := newArtifactFixture(t, "complete")
	publishedRelease, publishedPrerender, err := Publish(filepath.Join(t.TempDir(), "store"), releaseDir, prerenderDir, identity)
	if err != nil {
		t.Fatal(err)
	}

	changedInputs := testBuildInputs()
	changedInputs.Environment["BLDR_GO_WASM_OPTIMIZE"] = "false"
	changedIdentity := computeTestIdentity(t, repoRoot, changedInputs)
	if err := Validate(publishedRelease, publishedPrerender, changedIdentity); err == nil {
		t.Fatal("stale artifact identity validated")
	}
}

func TestForeignIdentityMissesSilentlyAndStaysReportable(t *testing.T) {
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	storeDir := filepath.Join(t.TempDir(), "store")

	releaseDir, prerenderDir := newArtifactFixture(t, "published")
	if _, _, err := Publish(storeDir, releaseDir, prerenderDir, identity); err != nil {
		t.Fatal(err)
	}

	changedInputs := testBuildInputs()
	changedInputs.Environment["BLDR_GO_WASM_OPTIMIZE"] = "false"
	changedIdentity := computeTestIdentity(t, repoRoot, changedInputs)

	// A store holding only another identity's work is a miss, not a failure:
	// the caller rebuilds. The caller can only say why it rebuilt if the names
	// it did not match stay readable.
	generations, err := ValidGenerations(storeDir, changedIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if len(generations) != 0 {
		t.Fatalf("foreign identity matched %d generation(s)", len(generations))
	}
	names, err := GenerationIDs(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || !strings.HasPrefix(names[0], identity.Digest) {
		t.Fatalf("store reported generations %v, want one named for %s", names, identity.Digest)
	}
}

func newArtifactFixture(t *testing.T, marker string) (string, string) {
	t.Helper()
	root := t.TempDir()
	releaseDir := filepath.Join(root, "release")
	prerenderDir := filepath.Join(root, "prerender")
	writeTestFile(t, filepath.Join(releaseDir, "browser-release.json"), `{
  "schemaVersion": 1,
  "generationId": "fixture-generation",
  "shellAssets": {
    "entrypoint": "/entrypoint.mjs",
    "serviceWorker": "/service-worker.mjs",
    "sharedWorker": "/shared-worker.mjs"
  }
}`)
	writeTestFile(t, filepath.Join(releaseDir, "marker.txt"), marker)
	writeTestFile(t, filepath.Join(prerenderDir, "index.html"), "<!doctype html>")
	writeTestFile(t, filepath.Join(prerenderDir, "marker.txt"), marker)
	return releaseDir, prerenderDir
}

func assertArtifactMarker(t *testing.T, releaseDir, prerenderDir, want string) {
	t.Helper()
	for _, path := range []string{filepath.Join(releaseDir, "marker.txt"), filepath.Join(prerenderDir, "marker.txt")} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != want {
			t.Fatalf("artifact marker = %q, want %q", data, want)
		}
	}
}

const (
	publishCrashRoleEnv       = "SPACEWAVE_ARTIFACT_PUBLISH_CRASH_ROLE"
	publishCrashStoreEnv      = "SPACEWAVE_ARTIFACT_PUBLISH_CRASH_STORE"
	publishCrashReleaseEnv    = "SPACEWAVE_ARTIFACT_PUBLISH_CRASH_RELEASE"
	publishCrashPrerenderEnv  = "SPACEWAVE_ARTIFACT_PUBLISH_CRASH_PRERENDER"
	publishCrashRepositoryEnv = "SPACEWAVE_ARTIFACT_PUBLISH_CRASH_REPOSITORY"
)

func TestPublishGenerationAndValidGenerationsTokenParity(t *testing.T) {
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	storeDir := filepath.Join(t.TempDir(), "store")
	releaseDir, prerenderDir := newArtifactFixture(t, "generation")

	generation, err := PublishGeneration(storeDir, releaseDir, prerenderDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if generation.ID == "" {
		t.Fatal("published generation has an empty ID")
	}
	generations, err := ValidGenerations(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(generations) != 1 || generations[0] != generation {
		t.Fatalf("listed generations = %#v, want %#v", generations, generation)
	}
	currentRelease, currentPrerender, err := Current(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if currentRelease != generation.ReleaseDir || currentPrerender != generation.PrerenderDir {
		t.Fatalf("current = (%q, %q), want (%q, %q)", currentRelease, currentPrerender, generation.ReleaseDir, generation.PrerenderDir)
	}
}

func TestValidGenerationsOrdersNonLexicalCurrentFirst(t *testing.T) {
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	storeDir := filepath.Join(t.TempDir(), "store")

	releaseA, prerenderA := newArtifactFixture(t, "first")
	first, err := PublishGeneration(storeDir, releaseA, prerenderA, identity)
	if err != nil {
		t.Fatal(err)
	}
	releaseB, prerenderB := newArtifactFixture(t, "second")
	second, err := PublishGeneration(storeDir, releaseB, prerenderB, identity)
	if err != nil {
		t.Fatal(err)
	}

	firstID := identity.Digest + "-a"
	secondID := identity.Digest + "-z"
	root := filepath.Join(storeDir, generationsDir)
	if err := os.Rename(filepath.Dir(first.ReleaseDir), filepath.Join(root, firstID)); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Dir(second.ReleaseDir), filepath.Join(root, secondID)); err != nil {
		t.Fatal(err)
	}
	if err := writeCurrent(storeDir, secondID); err != nil {
		t.Fatal(err)
	}

	generations, err := ValidGenerations(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(generations) != 2 || generations[0].ID != secondID || generations[1].ID != firstID {
		t.Fatalf("generations = %#v, want current %q before lexical-first %q", generations, secondID, firstID)
	}
}

func TestValidGenerationsCandidatePredicate(t *testing.T) {
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())

	t.Run("absent store is empty", func(t *testing.T) {
		generations, err := ValidGenerations(filepath.Join(t.TempDir(), "absent"), identity)
		if err != nil || len(generations) != 0 {
			t.Fatalf("generations = %#v, err = %v", generations, err)
		}
	})
	t.Run("other digest entry is ignored", func(t *testing.T) {
		storeDir := t.TempDir()
		writeTestFile(t, filepath.Join(storeDir, generationsDir, "other-entry"), "not a directory")
		generations, err := ValidGenerations(storeDir, identity)
		if err != nil || len(generations) != 0 {
			t.Fatalf("generations = %#v, err = %v", generations, err)
		}
	})
	for _, name := range []string{identity.Digest, identity.Digest + "-"} {
		t.Run("malformed matching "+filepath.Base(name), func(t *testing.T) {
			storeDir := t.TempDir()
			writeTestFile(t, filepath.Join(storeDir, generationsDir, name), "malformed")
			if generations, err := ValidGenerations(storeDir, identity); err == nil || len(generations) != 0 {
				t.Fatalf("generations = %#v, err = %v", generations, err)
			}
		})
	}
	t.Run("matching validation error propagates", func(t *testing.T) {
		storeDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(storeDir, generationsDir, identity.Digest+"-invalid"), 0o755); err != nil {
			t.Fatal(err)
		}
		if generations, err := ValidGenerations(storeDir, identity); err == nil || len(generations) != 0 {
			t.Fatalf("generations = %#v, err = %v", generations, err)
		}
	})
}

func TestValidGenerationsTreatsOutputDigestMismatchAsCacheMiss(t *testing.T) {
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	storeDir := filepath.Join(t.TempDir(), "store")
	releaseDir, prerenderDir := newArtifactFixture(t, "downloaded")
	generation, err := PublishGeneration(storeDir, releaseDir, prerenderDir, identity)
	if err != nil {
		t.Fatal(err)
	}

	writeTestFile(t, filepath.Join(generation.ReleaseDir, "marker.txt"), "changed after download")
	generations, err := ValidGenerations(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(generations) != 0 {
		t.Fatalf("generations = %#v, want stale output to be treated as a miss", generations)
	}

	rebuiltRelease, rebuiltPrerender := newArtifactFixture(t, "rebuilt")
	rebuilt, err := PublishGeneration(storeDir, rebuiltRelease, rebuiltPrerender, identity)
	if err != nil {
		t.Fatal(err)
	}
	generations, err = ValidGenerations(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(generations) != 1 || generations[0] != rebuilt {
		t.Fatalf("generations = %#v, want rebuilt generation %#v", generations, rebuilt)
	}
}

func TestValidGenerationsIgnoresTransportModeChanges(t *testing.T) {
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	storeDir := filepath.Join(t.TempDir(), "store")
	releaseDir, prerenderDir := newArtifactFixture(t, "transported")
	recordPath := filepath.Join(releaseDir, ".bundle-cache", "renderer.json")
	writeTestFile(t, recordPath, "{}")
	if err := os.Chmod(recordPath, 0o600); err != nil {
		t.Fatal(err)
	}

	generation, err := PublishGeneration(storeDir, releaseDir, prerenderDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(generation.ReleaseDir, ".bundle-cache", "renderer.json"), 0o644); err != nil {
		t.Fatal(err)
	}

	generations, err := ValidGenerations(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(generations) != 1 || generations[0] != generation {
		t.Fatalf("generations = %#v, want transported generation %#v", generations, generation)
	}
}

func TestValidGenerationsRejectsExternalValidFixtureSymlink(t *testing.T) {
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	externalStore := filepath.Join(t.TempDir(), "external-store")
	releaseDir, prerenderDir := newArtifactFixture(t, "external")
	external, err := PublishGeneration(externalStore, releaseDir, prerenderDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	externalDir := filepath.Dir(external.ReleaseDir)

	storeDir := t.TempDir()
	root := filepath.Join(storeDir, generationsDir)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalDir, filepath.Join(root, identity.Digest+"-external")); err != nil {
		t.Fatal(err)
	}
	if generations, err := ValidGenerations(storeDir, identity); err == nil || len(generations) != 0 {
		t.Fatalf("generations = %#v, err = %v", generations, err)
	}
}

func TestValidGenerationsRejectsMatchingNonDirectory(t *testing.T) {
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	storeDir := t.TempDir()
	writeTestFile(t, filepath.Join(storeDir, generationsDir, identity.Digest+"-file"), "not a directory")
	if generations, err := ValidGenerations(storeDir, identity); err == nil || len(generations) != 0 {
		t.Fatalf("generations = %#v, err = %v", generations, err)
	}
}

func TestPublishCrashRecovery(t *testing.T) {
	if role := os.Getenv(publishCrashRoleEnv); role != "" {
		runPublishCrashRole(t, role)
		return
	}
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	releaseDir, prerenderDir := newArtifactFixture(t, "crash")

	t.Run("before rename leaves no generation", func(t *testing.T) {
		storeDir := filepath.Join(t.TempDir(), "store")
		runPublishCrashChild(t, "before-rename", storeDir, releaseDir, prerenderDir, repoRoot)
		generations, err := ValidGenerations(storeDir, identity)
		if err != nil {
			t.Fatal(err)
		}
		if len(generations) != 0 {
			t.Fatalf("generations = %#v", generations)
		}
	})
	t.Run("after rename recovers unpointed generation", func(t *testing.T) {
		storeDir := filepath.Join(t.TempDir(), "store")
		runPublishCrashChild(t, "after-rename", storeDir, releaseDir, prerenderDir, repoRoot)
		generations, err := ValidGenerations(storeDir, identity)
		if err != nil {
			t.Fatal(err)
		}
		if len(generations) != 1 || generations[0].ID == "" {
			t.Fatalf("generations = %#v", generations)
		}
		if _, _, err := Current(storeDir, identity); err == nil {
			t.Fatal("crashed publication unexpectedly wrote current pointer")
		}
	})
}

func runPublishCrashChild(t *testing.T, role, storeDir, releaseDir, prerenderDir, repoRoot string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPublishCrashRecovery$") //nolint:gosec
	cmd.Env = append(os.Environ(),
		publishCrashRoleEnv+"="+role,
		publishCrashStoreEnv+"="+storeDir,
		publishCrashReleaseEnv+"="+releaseDir,
		publishCrashPrerenderEnv+"="+prerenderDir,
		publishCrashRepositoryEnv+"="+repoRoot,
	)
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("crash child error = %v", err)
	}
	wantCode := 73
	if role == "after-rename" {
		wantCode = 74
	}
	if exitErr.ExitCode() != wantCode {
		t.Fatalf("crash child exit code = %d, want %d", exitErr.ExitCode(), wantCode)
	}
}

func runPublishCrashRole(t *testing.T, role string) {
	t.Helper()
	identity := computeTestIdentity(t, os.Getenv(publishCrashRepositoryEnv), testBuildInputs())
	hooks := publishHooks{}
	switch role {
	case "before-rename":
		hooks.beforeRename = func() { os.Exit(73) }
	case "after-rename":
		hooks.afterRenameBeforeCurrent = func() { os.Exit(74) }
	default:
		t.Fatalf("unknown crash role %q", role)
	}
	if _, err := publish(
		os.Getenv(publishCrashStoreEnv),
		os.Getenv(publishCrashReleaseEnv),
		os.Getenv(publishCrashPrerenderEnv),
		identity,
		hooks,
	); err != nil {
		t.Fatal(err)
	}
	t.Fatal("publish crash hook did not exit")
}
