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
	// Create the repository identity and artifact store.
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	storeDir := filepath.Join(t.TempDir(), "store")

	// Publish the first complete artifact pair.
	releaseA, prerenderA := newArtifactFixture(t, "first")
	publishedRelease, publishedPrerender, err := Publish(storeDir, releaseA, prerenderA, identity)
	if err != nil {
		t.Fatal(err)
	}

	// Verify both published outputs carry the first marker.
	assertArtifactMarker(t, publishedRelease, publishedPrerender, "first")

	// Leave an incomplete staging tree beside the published generation.
	partialDir := filepath.Join(storeDir, ".publish-killed")
	writeTestFile(t, filepath.Join(partialDir, "release", "browser-release.json"), "{}")
	writeTestFile(t, filepath.Join(partialDir, "prerender", "index.html"), "partial")

	// Resolve the current generation after interrupted staging.
	currentRelease, currentPrerender, err := Current(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}

	// Verify incomplete staging preserves the first output pair.
	assertArtifactMarker(t, currentRelease, currentPrerender, "first")

	// Publish a second complete artifact pair.
	releaseB, prerenderB := newArtifactFixture(t, "second")
	if _, _, err := Publish(storeDir, releaseB, prerenderB, identity); err != nil {
		t.Fatal(err)
	}

	// Resolve the current generation after the second publication.
	currentRelease, currentPrerender, err = Current(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the second output pair becomes current.
	assertArtifactMarker(t, currentRelease, currentPrerender, "second")
}

func TestConcurrentPublishNeverInterleavesOutputs(t *testing.T) {
	// Create the repository identity and shared artifact store.
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	storeDir := filepath.Join(t.TempDir(), "store")

	// Publish competing artifact pairs and wait for every publication.
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

	// Read the current generation and both output markers.
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

	// Verify competing publishers preserve matching output markers.
	if string(releaseMarker) != string(prerenderMarker) {
		t.Fatalf("interleaved artifact outputs: release=%q prerender=%q", releaseMarker, prerenderMarker)
	}
}

func TestValidateRejectsPartialAndModifiedArtifacts(t *testing.T) {
	// Create the expected artifact identity.
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())

	// Verify incomplete release descriptors fail artifact validation.
	partialRelease := filepath.Join(t.TempDir(), "release")
	partialPrerender := filepath.Join(t.TempDir(), "prerender")
	writeTestFile(t, filepath.Join(partialRelease, "browser-release.json"), "{}")
	writeTestFile(t, filepath.Join(partialPrerender, "index.html"), "<!doctype html>")
	if err := Validate(partialRelease, partialPrerender, identity); err == nil {
		t.Fatal("partial artifact validated")
	}

	// Publish and validate a complete artifact pair.
	releaseDir, prerenderDir := newArtifactFixture(t, "complete")
	publishedRelease, publishedPrerender, err := Publish(filepath.Join(t.TempDir(), "store"), releaseDir, prerenderDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(publishedRelease, publishedPrerender, identity); err != nil {
		t.Fatal(err)
	}

	// Verify modified output bytes fail artifact validation.
	writeTestFile(t, filepath.Join(publishedRelease, "marker.txt"), "truncated")
	if err := Validate(publishedRelease, publishedPrerender, identity); err == nil {
		t.Fatal("modified artifact validated")
	}
}

func TestValidateRejectsStaleIdentity(t *testing.T) {
	// Create and publish an artifact with the baseline identity.
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	releaseDir, prerenderDir := newArtifactFixture(t, "complete")
	publishedRelease, publishedPrerender, err := Publish(filepath.Join(t.TempDir(), "store"), releaseDir, prerenderDir, identity)
	if err != nil {
		t.Fatal(err)
	}

	// Verify a changed build environment rejects the saved artifact.
	changedInputs := testBuildInputs()
	changedInputs.Environment["BLDR_GO_WASM_OPTIMIZE"] = "false"
	changedIdentity := computeTestIdentity(t, repoRoot, changedInputs)
	if err := Validate(publishedRelease, publishedPrerender, changedIdentity); err == nil {
		t.Fatal("stale artifact identity validated")
	}
}

func TestPublishCopiesNestedFilesAndSymlinks(t *testing.T) {
	// Create the expected identity and nested artifact fixture.
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	releaseDir, prerenderDir := newArtifactFixture(t, "nested")

	// Add a nested file and symbolic link to the release output.
	nestedPath := filepath.Join(releaseDir, "assets", "nested.txt")
	writeTestFile(t, nestedPath, "nested artifact")
	if err := os.Symlink("nested.txt", filepath.Join(releaseDir, "assets", "nested.link")); err != nil {
		t.Fatal(err)
	}

	// Publish the nested artifact fixture.
	generation, err := PublishGeneration(filepath.Join(t.TempDir(), "store"), releaseDir, prerenderDir, identity)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the published nested file preserves its bytes.
	data, err := os.ReadFile(filepath.Join(generation.ReleaseDir, "assets", "nested.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "nested artifact" {
		t.Fatalf("nested artifact = %q, want %q", data, "nested artifact")
	}

	// Verify the published symbolic link preserves its target.
	link, err := os.Readlink(filepath.Join(generation.ReleaseDir, "assets", "nested.link"))
	if err != nil {
		t.Fatal(err)
	}
	if link != "nested.txt" {
		t.Fatalf("nested symlink = %q, want %q", link, "nested.txt")
	}
}

func TestForeignIdentityMissesSilentlyAndStaysReportable(t *testing.T) {
	// Create the expected identity and artifact store.
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	storeDir := filepath.Join(t.TempDir(), "store")

	// Publish an artifact with the baseline identity.
	releaseDir, prerenderDir := newArtifactFixture(t, "published")
	if _, _, err := Publish(storeDir, releaseDir, prerenderDir, identity); err != nil {
		t.Fatal(err)
	}

	// Compute an identity for a different build environment.
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

	// Verify foreign generation names remain available for diagnostics.
	names, err := GenerationIDs(storeDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || !strings.HasPrefix(names[0], identity.Digest) {
		t.Fatalf("store reported generations %v, want one named for %s", names, identity.Digest)
	}
}

func newArtifactFixture(t *testing.T, marker string) (string, string) {
	// Create release and prerender directories for the artifact fixture.
	t.Helper()
	root := t.TempDir()
	releaseDir := filepath.Join(root, "release")
	prerenderDir := filepath.Join(root, "prerender")

	// Write complete release and prerender outputs with matching markers.
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
	// Create the identity, store, and generation fixture.
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	storeDir := filepath.Join(t.TempDir(), "store")
	releaseDir, prerenderDir := newArtifactFixture(t, "generation")

	// Publish a generation and require its identifier.
	generation, err := PublishGeneration(storeDir, releaseDir, prerenderDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if generation.ID == "" {
		t.Fatal("published generation has an empty ID")
	}

	// Verify generation listing returns the published generation.
	generations, err := ValidGenerations(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(generations) != 1 || generations[0] != generation {
		t.Fatalf("listed generations = %#v, want %#v", generations, generation)
	}

	// Verify the current pointer resolves the published output paths.
	currentRelease, currentPrerender, err := Current(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if currentRelease != generation.ReleaseDir || currentPrerender != generation.PrerenderDir {
		t.Fatalf("current = (%q, %q), want (%q, %q)", currentRelease, currentPrerender, generation.ReleaseDir, generation.PrerenderDir)
	}
}

func TestValidGenerationsOrdersNonLexicalCurrentFirst(t *testing.T) {
	// Create the identity and artifact store.
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	storeDir := filepath.Join(t.TempDir(), "store")

	// Publish two complete artifact generations.
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

	// Give the generations ordered names and select the lexical-last generation.
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

	// Verify the current generation precedes the lexical-first generation.
	generations, err := ValidGenerations(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(generations) != 2 || generations[0].ID != secondID || generations[1].ID != firstID {
		t.Fatalf("generations = %#v, want current %q before lexical-first %q", generations, secondID, firstID)
	}
}

func TestValidGenerationsCandidatePredicate(t *testing.T) {
	// Create the expected artifact identity.
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())

	// Exercise absent, foreign, malformed, and incomplete generation candidates.
	t.Run("absent store is empty", func(t *testing.T) {
		generations, err := ValidGenerations(filepath.Join(t.TempDir(), "absent"), identity)
		if err != nil || len(generations) != 0 {
			t.Fatalf("generations = %#v, err = %v", generations, err)
		}
	})
	t.Run("other digest entry is ignored", func(t *testing.T) {
		// Create a foreign generation entry in the artifact store.
		storeDir := t.TempDir()
		writeTestFile(t, filepath.Join(storeDir, generationsDir, "other-entry"), "not a directory")

		// Verify the foreign entry does not match the expected identity.
		generations, err := ValidGenerations(storeDir, identity)
		if err != nil || len(generations) != 0 {
			t.Fatalf("generations = %#v, err = %v", generations, err)
		}
	})
	for _, name := range []string{identity.Digest, identity.Digest + "-"} {
		t.Run("malformed matching "+filepath.Base(name), func(t *testing.T) {
			// Create a malformed entry with the expected identity digest.
			storeDir := t.TempDir()
			writeTestFile(t, filepath.Join(storeDir, generationsDir, name), "malformed")

			// Verify matching entries require a complete generation name.

			// Verify incomplete matching generations propagate validation errors.
			if generations, err := ValidGenerations(storeDir, identity); err == nil || len(generations) != 0 {
				t.Fatalf("generations = %#v, err = %v", generations, err)
			}
		})
	}
	t.Run("matching validation error propagates", func(t *testing.T) {
		// Create an incomplete generation with the expected identity digest.
		storeDir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(storeDir, generationsDir, identity.Digest+"-invalid"), 0o755); err != nil {
			t.Fatal(err)
		}

		// Verify incomplete matching generations propagate validation errors.
		if generations, err := ValidGenerations(storeDir, identity); err == nil || len(generations) != 0 {
			t.Fatalf("generations = %#v, err = %v", generations, err)
		}
	})
}

func TestValidGenerationsTreatsOutputDigestMismatchAsCacheMiss(t *testing.T) {
	// Publish an artifact generation for the expected identity.
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	storeDir := filepath.Join(t.TempDir(), "store")
	releaseDir, prerenderDir := newArtifactFixture(t, "downloaded")
	generation, err := PublishGeneration(storeDir, releaseDir, prerenderDir, identity)
	if err != nil {
		t.Fatal(err)
	}

	// Verify modified output bytes make the stored generation a cache miss.
	writeTestFile(t, filepath.Join(generation.ReleaseDir, "marker.txt"), "changed after download")
	generations, err := ValidGenerations(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(generations) != 0 {
		t.Fatalf("generations = %#v, want stale output to be treated as a miss", generations)
	}

	// Publish a rebuilt artifact generation.
	rebuiltRelease, rebuiltPrerender := newArtifactFixture(t, "rebuilt")
	rebuilt, err := PublishGeneration(storeDir, rebuiltRelease, rebuiltPrerender, identity)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the rebuilt generation is the sole usable artifact.
	generations, err = ValidGenerations(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(generations) != 1 || generations[0] != rebuilt {
		t.Fatalf("generations = %#v, want rebuilt generation %#v", generations, rebuilt)
	}
}

func TestValidGenerationsIgnoresTransportModeChanges(t *testing.T) {
	// Create an artifact fixture with a restricted cache record.
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	storeDir := filepath.Join(t.TempDir(), "store")
	releaseDir, prerenderDir := newArtifactFixture(t, "transported")
	recordPath := filepath.Join(releaseDir, ".bundle-cache", "renderer.json")
	writeTestFile(t, recordPath, "{}")
	if err := os.Chmod(recordPath, 0o600); err != nil {
		t.Fatal(err)
	}

	// Publish the artifact and change the transported cache record permissions.
	generation, err := PublishGeneration(storeDir, releaseDir, prerenderDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(generation.ReleaseDir, ".bundle-cache", "renderer.json"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Verify transport permission changes preserve the usable generation.
	generations, err := ValidGenerations(storeDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	if len(generations) != 1 || generations[0] != generation {
		t.Fatalf("generations = %#v, want transported generation %#v", generations, generation)
	}
}

func TestValidGenerationsRejectsExternalValidFixtureSymlink(t *testing.T) {
	// Publish a complete artifact in an external store.
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	externalStore := filepath.Join(t.TempDir(), "external-store")
	releaseDir, prerenderDir := newArtifactFixture(t, "external")
	external, err := PublishGeneration(externalStore, releaseDir, prerenderDir, identity)
	if err != nil {
		t.Fatal(err)
	}
	externalDir := filepath.Dir(external.ReleaseDir)

	// Verify a generation symlink cannot import the external artifact.
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
	// Verify a matching generation file fails directory validation.
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	storeDir := t.TempDir()
	writeTestFile(t, filepath.Join(storeDir, generationsDir, identity.Digest+"-file"), "not a directory")
	if generations, err := ValidGenerations(storeDir, identity); err == nil || len(generations) != 0 {
		t.Fatalf("generations = %#v, err = %v", generations, err)
	}
}

func TestPublishCrashRecovery(t *testing.T) {
	// Dispatch the crash child process when its role is selected.
	if role := os.Getenv(publishCrashRoleEnv); role != "" {
		runPublishCrashRole(t, role)
		return
	}

	// Create the identity and artifact pair for crash recovery.
	repoRoot := newIdentityTestRepo(t)
	identity := computeTestIdentity(t, repoRoot, testBuildInputs())
	releaseDir, prerenderDir := newArtifactFixture(t, "crash")

	// Exercise interruption before and after generation commit.
	t.Run("before rename leaves no generation", func(t *testing.T) {
		// Interrupt publication before committing a generation.
		storeDir := filepath.Join(t.TempDir(), "store")
		runPublishCrashChild(t, "before-rename", storeDir, releaseDir, prerenderDir, repoRoot)

		// Verify interruption before commit leaves no usable generation.
		generations, err := ValidGenerations(storeDir, identity)
		if err != nil {
			t.Fatal(err)
		}
		if len(generations) != 0 {
			t.Fatalf("generations = %#v", generations)
		}
	})
	t.Run("after rename recovers unpointed generation", func(t *testing.T) {
		// Interrupt publication after committing a generation.
		storeDir := filepath.Join(t.TempDir(), "store")
		runPublishCrashChild(t, "after-rename", storeDir, releaseDir, prerenderDir, repoRoot)

		// Verify the committed generation survives without a current pointer.
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
	// Create the bounded child process context for crash testing.
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	// Configure the child process with its artifact paths and crash role.
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPublishCrashRecovery$") //nolint:gosec
	cmd.Env = append(os.Environ(),
		publishCrashRoleEnv+"="+role,
		publishCrashStoreEnv+"="+storeDir,
		publishCrashReleaseEnv+"="+releaseDir,
		publishCrashPrerenderEnv+"="+prerenderDir,
		publishCrashRepositoryEnv+"="+repoRoot,
	)

	// Run the child and require an intentional process exit.
	err := cmd.Run()
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("crash child error = %v", err)
	}

	// Verify the child exited at the selected publication hook.
	wantCode := 73
	if role == "after-rename" {
		wantCode = 74
	}
	if exitErr.ExitCode() != wantCode {
		t.Fatalf("crash child exit code = %d, want %d", exitErr.ExitCode(), wantCode)
	}
}

func runPublishCrashRole(t *testing.T, role string) {
	// Create the artifact identity and hook for the selected crash role.
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

	// Run publication and require the crash hook to exit the process.
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
