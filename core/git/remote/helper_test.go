package git_remote

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

const testRepoKey = "repo"

func TestHelperPushListFetch(t *testing.T) {
	ctx := t.Context()
	engine := newTestWorld(t, ctx)
	src := newLocalRepo(t)
	head := commit(t, src, "one")

	push := NewHelper(engine, testRepoKey, filepath.Join(src, ".git"))
	if got := run(t, ctx, push, "capabilities\n"); got != "fetch\npush\n\n" {
		t.Fatalf("capabilities = %q", got)
	}
	if got := run(t, ctx, push, "list for-push\n"); got != "\n" {
		t.Fatalf("list before the first push = %q", got)
	}
	if got := run(t, ctx, push, "push refs/heads/master:refs/heads/master\n\n"); got != "ok refs/heads/master\n\n" {
		t.Fatalf("push = %q", got)
	}
	if got := run(t, ctx, push, "list for-push\n"); !strings.Contains(got, head+" refs/heads/master\n") {
		t.Fatalf("list = %q, want %s", got, head)
	}

	// A fresh clone receives the objects without the helper's transfer refs.
	dst := newLocalRepo(t)
	fetch := NewHelper(engine, testRepoKey, filepath.Join(dst, ".git"))
	if got := run(t, ctx, fetch, "fetch "+head+" refs/heads/master\n\n"); got != "\n" {
		t.Fatalf("fetch = %q", got)
	}
	gitCmd(t, dst, "cat-file", "-e", head+"^{commit}")
	if refs := gitCmd(t, dst, "for-each-ref", fetchRefPrefix); refs != "" {
		t.Fatalf("transfer refs remain: %q", refs)
	}
}

func TestHelperRejectsNonFastForward(t *testing.T) {
	ctx := t.Context()
	engine := newTestWorld(t, ctx)
	src := newLocalRepo(t)
	commit(t, src, "one")
	h := NewHelper(engine, testRepoKey, filepath.Join(src, ".git"))
	run(t, ctx, h, "push refs/heads/master:refs/heads/master\n\n")

	// Replace the branch with an unrelated history.
	gitCmd(t, src, "checkout", "-q", "--orphan", "other")
	gitCmd(t, src, "branch", "-q", "-D", "master")
	gitCmd(t, src, "checkout", "-q", "-b", "master")
	rewritten := commit(t, src, "two")

	if got := run(t, ctx, h, "push refs/heads/master:refs/heads/master\n\n"); got != "error refs/heads/master non-fast-forward\n\n" {
		t.Fatalf("push = %q", got)
	}
	if got := run(t, ctx, h, "push +refs/heads/master:refs/heads/master\n\n"); got != "ok refs/heads/master\n\n" {
		t.Fatalf("forced push = %q", got)
	}
	if got := run(t, ctx, h, "list\n"); !strings.Contains(got, rewritten+" refs/heads/master\n") {
		t.Fatalf("list = %q, want %s", got, rewritten)
	}
}

func TestHelperDeletesRef(t *testing.T) {
	ctx := t.Context()
	engine := newTestWorld(t, ctx)
	src := newLocalRepo(t)
	commit(t, src, "one")
	h := NewHelper(engine, testRepoKey, filepath.Join(src, ".git"))
	run(t, ctx, h, "push refs/heads/master:refs/heads/master\npush refs/heads/master:refs/heads/topic\n\n")

	if got := run(t, ctx, h, "push :refs/heads/topic\n\n"); got != "ok refs/heads/topic\n\n" {
		t.Fatalf("delete = %q", got)
	}
	if got := run(t, ctx, h, "list\n"); strings.Contains(got, "refs/heads/topic") || !strings.Contains(got, "refs/heads/master") {
		t.Fatalf("list = %q", got)
	}
}

// newTestWorld returns an empty World; the first push creates the Repo object.
func newTestWorld(t *testing.T, ctx context.Context) world.Engine {
	t.Helper()
	return world_testbed.MustDefault(t, ctx).Engine
}

// newLocalRepo initializes a local repository on master.
func newLocalRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	gitCmd(t, dir, "init", "-q", "-b", "master")
	return dir
}

// commit records an empty commit and returns its hash.
func commit(t *testing.T, dir, msg string) string {
	t.Helper()
	gitCmd(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "--allow-empty", "-m", msg)
	return gitCmd(t, dir, "rev-parse", "HEAD")
}

// gitCmd runs git in dir and returns its trimmed output.
func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// run sends input to the helper and returns its output.
func run(t *testing.T, ctx context.Context, h *Helper, input string) string {
	t.Helper()
	var out bytes.Buffer
	if err := h.Run(ctx, strings.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	return out.String()
}
