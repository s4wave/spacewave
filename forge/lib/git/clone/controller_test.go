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
	// Attribute repository fixture failures to the calling test.
	t.Helper()

	// Initialize a source repository and open its worktree.
	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err.Error())
	}

	// Stage the source repository README for the initial snapshot.
	if err := os.WriteFile(dir+"/README.md", []byte("test repo\n"), 0o644); err != nil {
		t.Fatal(err.Error())
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatal(err.Error())
	}

	// Commit the initial snapshot that the controller will clone.
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
	// Start the Forge testbed with the clone factory and source repository.
	tb, err := testbed.Default(context.Background())
	if err != nil {
		t.Fatal(err.Error())
	}
	ctx := tb.Context
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))
	repoRoot := createSourceRepo(t)

	// Resolve the clone execution target from its repository URL.
	tgt, err := target_json.ResolveYAML(ctx, tb.Bus, []byte(buildTestYAML(repoRoot)))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Supply the value set that the Task controller normally resolves.
	valueSet := &forge_target.ValueSet{}

	// Execute the clone target with the supplied value set.
	// handle := forge_target.ExecControllerHandleWithAccess(ws.AccessWorldState)
	ts := timestamp.Now()
	finalState, err := tb.RunExecutionWithTarget(tgt, valueSet, ts)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Decode execution outputs for the repository presence check.
	outputs := forge_value.ValueSlice(finalState.GetValueSet().GetOutputs())
	valMap, err := outputs.BuildValueMap(true, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify that the clone output is present.
	stv := valMap["repo"]
	if stv.IsEmpty() {
		t.Fatal("expected repo output to be set but was empty")
	}
}
