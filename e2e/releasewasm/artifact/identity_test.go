package artifact

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestComputeIdentityTracksSourceEnvironmentModeAndLockfiles(t *testing.T) {
	repoRoot := newIdentityTestRepo(t)
	inputs := testBuildInputs()
	base := computeTestIdentity(t, repoRoot, inputs)

	writeTestFile(t, filepath.Join(repoRoot, "source", "main.go"), "package source\n\nconst Value = 2\n")
	sourceChanged := computeTestIdentity(t, repoRoot, inputs)
	if sourceChanged.SourceDigest == base.SourceDigest || sourceChanged.Digest == base.Digest {
		t.Fatal("source edit did not change release artifact identity")
	}

	writeTestFile(t, filepath.Join(repoRoot, "source", "main.go"), "package source\n\nconst Value = 1\n")
	envInputs := testBuildInputs()
	envInputs.Environment["BLDR_GO_WASM_OPTIMIZE"] = "false"
	envChanged := computeTestIdentity(t, repoRoot, envInputs)
	if envChanged.BuildDigest == base.BuildDigest || envChanged.Digest == base.Digest {
		t.Fatal("environment edit did not change release artifact identity")
	}

	modeInputs := testBuildInputs()
	modeInputs.Mode = "release/web/e2e/debug"
	modeChanged := computeTestIdentity(t, repoRoot, modeInputs)
	if modeChanged.Mode == base.Mode || modeChanged.Digest == base.Digest {
		t.Fatal("mode edit did not change release artifact identity")
	}

	writeTestFile(t, filepath.Join(repoRoot, "bun.lock"), "lockfile-v2\n")
	lockChanged := computeTestIdentity(t, repoRoot, inputs)
	if lockChanged.LockfileDigest == base.LockfileDigest || lockChanged.Digest == base.Digest {
		t.Fatal("lockfile edit did not change release artifact identity")
	}
}

func TestComputeIdentityTracksBldrAndPrerenderInputs(t *testing.T) {
	repoRoot := newIdentityTestRepo(t)
	inputs := testBuildInputs()
	base := computeTestIdentity(t, repoRoot, inputs)

	writeTestFile(t, filepath.Join(repoRoot, "bldr", "cache-format.go"), "package bldr\n\nconst CacheFormat = 2\n")
	bldrChanged := computeTestIdentity(t, repoRoot, inputs)
	if bldrChanged.BldrDigest == base.BldrDigest || bldrChanged.Digest == base.Digest {
		t.Fatal("Bldr input edit did not change release artifact identity")
	}

	writeTestFile(t, filepath.Join(repoRoot, "bldr", "cache-format.go"), "package bldr\n\nconst CacheFormat = 1\n")
	writeTestFile(t, filepath.Join(repoRoot, "app", "prerender", "entry.ts"), "export const route = '/changed'\n")
	prerenderChanged := computeTestIdentity(t, repoRoot, inputs)
	if prerenderChanged.PrerenderInputsDigest == base.PrerenderInputsDigest || prerenderChanged.Digest == base.Digest {
		t.Fatal("prerender input edit did not change release artifact identity")
	}
}

func newIdentityTestRepo(t *testing.T) string {
	t.Helper()
	repoRoot := t.TempDir()
	writeTestFile(t, filepath.Join(repoRoot, "source", "main.go"), "package source\n\nconst Value = 1\n")
	writeTestFile(t, filepath.Join(repoRoot, "bldr", "cache-format.go"), "package bldr\n\nconst CacheFormat = 1\n")
	writeTestFile(t, filepath.Join(repoRoot, "app", "prerender", "entry.ts"), "export const route = '/'\n")
	writeTestFile(t, filepath.Join(repoRoot, "go.mod"), "module example.com/release-identity\n\ngo 1.25\n")
	writeTestFile(t, filepath.Join(repoRoot, "bun.lock"), "lockfile-v1\n")
	runTestGit(t, repoRoot, "init")
	runTestGit(t, repoRoot, "add", ".")
	return repoRoot
}

func testBuildInputs() *BuildInputs {
	return &BuildInputs{
		Compiler: "tinygo",
		Mode:     "release/web/e2e",
		Environment: map[string]string{
			"BLDR_GO_WASM_OPTIMIZE": "true",
		},
		Tools: map[string]string{
			"bun":    "1.2.0",
			"tinygo": "0.41.1",
		},
	}
}

func computeTestIdentity(t *testing.T, repoRoot string, inputs *BuildInputs) *Identity {
	t.Helper()
	identity, err := ComputeIdentity(repoRoot, inputs)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func writeTestFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runTestGit(t *testing.T, repoRoot string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
