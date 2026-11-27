package forge_lib_skiffos

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
)

// TestSyncRepo checks that a checkout reaches exactly the source's commits,
// the superproject's and its submodule's, on a first checkout and on a move,
// copying the objects from the source repository, also from a shallow one,
// that a checkout already at the commit stays on its branch, and that a move
// refuses to overwrite a local change.
func TestSyncRepo(t *testing.T) {
	// Build a source with a submodule, at a first and a second commit.
	ctx := context.Background()
	tmp := t.TempDir()
	sub, src, dst := filepath.Join(tmp, "sub"), filepath.Join(tmp, "src"), filepath.Join(tmp, "dst")
	runTestGit(t, tmp, "init", "-q", sub)
	commitFile(t, sub, "a", "1")
	runTestGit(t, tmp, "init", "-q", src)
	commitFile(t, src, "r", "1")
	runTestGit(t, src, "-c", "protocol.file.allow=always", "submodule", "add", "-q", sub, "mod")
	runTestGit(t, src, "commit", "-qm", "add mod")
	first := runTestGit(t, src, "rev-parse", "HEAD")
	commitFile(t, sub, "a", "2")
	runTestGit(t, filepath.Join(src, "mod"), "pull", "-q")
	commitFile(t, src, "mod", "")
	second := runTestGit(t, src, "rev-parse", "HEAD")

	// Check out each commit into an empty repository, then move the
	// checkout.
	repo, err := git.PlainOpen(src)
	if err != nil {
		t.Fatal(err)
	}
	runTestGit(t, tmp, "init", "-q", dst)
	for _, want := range []string{first, second} {
		checkSync(t, repo, src, dst, want)
	}

	// A deinitialized submodule is linked again, not mistaken for the
	// superproject.
	runTestGit(t, dst, "submodule", "deinit", "-q", "--all")
	checkSync(t, repo, src, dst, second)

	// A checkout already at the commit keeps its branch.
	runTestGit(t, dst, "checkout", "-q", "-b", "kept")
	checkSync(t, repo, src, dst, second)
	if got := runTestGit(t, dst, "symbolic-ref", "HEAD"); got != "refs/heads/kept" {
		t.Fatalf("checkout at the commit moved to %s", got)
	}

	// A depth-1 clone of the source, submodule included, also fills an empty
	// repository, which records the copied commits as shallow.
	shallow, fresh := filepath.Join(tmp, "shallow"), filepath.Join(tmp, "fresh")
	runTestGit(t, tmp, "clone", "-q", "--depth", "1", "file://"+src, shallow)
	runTestGit(t, shallow, "-c", "protocol.file.allow=always", "submodule", "update", "-q", "--init", "--depth", "1")
	shallowRepo, err := git.PlainOpen(shallow)
	if err != nil {
		t.Fatal(err)
	}
	runTestGit(t, tmp, "init", "-q", fresh)
	checkSync(t, shallowRepo, src, fresh, second)
	if got := runTestGit(t, fresh, "rev-list", "--count", "HEAD"); got != "1" {
		t.Fatalf("shallow checkout lists %s commits, want 1", got)
	}

	// A local change to a file the move rewrites is refused, not discarded.
	if err := os.WriteFile(filepath.Join(dst, "mod", "a"), []byte("local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syncRepo(ctx, repo, dst, filepath.Join(dst, ".git"), plumbing.NewHash(first)); err == nil {
		t.Fatal("move over a local change succeeded")
	}
	if data, _ := os.ReadFile(filepath.Join(dst, "mod", "a")); string(data) != "local\n" {
		t.Fatalf("local change lost: %q", data)
	}
}

// checkSync syncs dst to the commit want from repo and checks HEAD, the
// submodule's pin as src records it, and a clean status.
func checkSync(t *testing.T, repo *git.Repository, src, dst, want string) {
	t.Helper()
	if err := syncRepo(t.Context(), repo, dst, filepath.Join(dst, ".git"), plumbing.NewHash(want)); err != nil {
		t.Fatal(err)
	}
	if got := runTestGit(t, dst, "rev-parse", "HEAD"); got != want {
		t.Fatalf("checkout at %s, want %s", got, want)
	}
	pinned := strings.Fields(runTestGit(t, src, "ls-tree", want, "mod"))[2]
	if got := runTestGit(t, filepath.Join(dst, "mod"), "rev-parse", "HEAD"); got != pinned {
		t.Fatalf("submodule at %s, want %s", got, pinned)
	}
	if status := runTestGit(t, dst, "status", "--porcelain"); status != "" {
		t.Fatalf("checkout not clean:\n%s", status)
	}
}

// commitFile writes name in the repository at dir, a file holding content or
// the staged submodule when content is empty, and commits it.
func commitFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if content != "" {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runTestGit(t, dir, "add", name)
	runTestGit(t, dir, "commit", "-qm", name)
}

// runTestGit runs git in dir with a fixed identity and returns its trimmed output.
func runTestGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		"GIT_CONFIG_GLOBAL=/dev/null",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
