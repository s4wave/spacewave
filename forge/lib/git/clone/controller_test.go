package forge_lib_git_clone

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	forge_target "github.com/s4wave/spacewave/forge/target"
	target_json "github.com/s4wave/spacewave/forge/target/json"
	"github.com/s4wave/spacewave/forge/testbed"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// buildTestYAML returns the test YAML with an absolute file:// clone URL.
// go-billy/v6 enforces chroot boundaries, so relative paths like
// "../../../" are rejected, and go-git clone expects a transport URL here.
func buildTestYAML(repoRoot string) string {
	repoURL := (&url.URL{
		Scheme: "file",
		Path:   repoRoot,
	}).String()
	return strings.ReplaceAll(`
# note: this test is just for Execution controller.
# the inputs / outputs listed in the Target are not used.
exec:
  controller:
    # rev: 0 -> defaults to 1
    config:
      objectKey: "my-repo"
      cloneOpts:
        url: "REPO_URL"
      worktreeOpts:
        objectKey: "my-worktree"
        workdirRef:
          objectKey: "my-workdir"
        createWorkdir: true
    id: forge/lib/git/clone
`, "REPO_URL", repoURL)
}

func createSourceRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := os.WriteFile(dir+"/README.md", []byte("test repo\n"), 0o644); err != nil {
		t.Fatal(err.Error())
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatal(err.Error())
	}
	sig := &object.Signature{
		Name:  "Test",
		Email: "test@example.com",
		When:  time.Now(),
	}
	if _, err := wt.Commit("initial", &git.CommitOptions{
		Author:    sig,
		Committer: sig,
	}); err != nil {
		t.Fatal(err.Error())
	}
	return dir
}

// TestGitClone tests the git clone controller.
func TestGitClone(t *testing.T) {
	tb, err := testbed.Default(context.Background())
	if err != nil {
		t.Fatal(err.Error())
	}
	ctx := tb.Context
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))
	repoRoot := createSourceRepo(t)

	tgt, err := target_json.ResolveYAML(ctx, tb.Bus, []byte(buildTestYAML(repoRoot)))
	if err != nil {
		t.Fatal(err.Error())
	}

	// ordinarily resolved by Task controller, set it manually
	valueSet := &forge_target.ValueSet{}
	// handle := forge_target.ExecControllerHandleWithAccess(ws.AccessWorldState)
	ts := timestamp.Now()
	finalState, err := tb.RunExecutionWithTarget(tgt, valueSet, ts)
	if err != nil {
		t.Fatal(err.Error())
	}

	outputs := forge_value.ValueSlice(finalState.GetValueSet().GetOutputs())
	valMap, err := outputs.BuildValueMap(true, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// check repo output
	stv := valMap["repo"]
	if stv.IsEmpty() {
		t.Fatal("expected repo output to be set but was empty")
	}
}
