package artifact

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
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
