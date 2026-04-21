package forge_lib_git_clone

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	forge_target "github.com/s4wave/spacewave/forge/target"
	target_json "github.com/s4wave/spacewave/forge/target/json"
	"github.com/s4wave/spacewave/forge/testbed"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// buildTestYAML returns the test YAML with an absolute clone URL.
// go-billy/v6 enforces chroot boundaries, so relative paths like
// "../../../" are rejected.
func buildTestYAML(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err.Error())
	}
	repoRoot, err := filepath.Abs(filepath.Join(wd, "..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err.Error())
	}
	return strings.ReplaceAll(`
# note: this test is just for Execution controller.
# the inputs / outputs listed in the Target are not used.
exec:
  controller:
    # rev: 0 -> defaults to 1
    config:
      objectKey: "my-repo"
      cloneOpts:
        url: "REPO_ROOT"
      worktreeOpts:
        objectKey: "my-worktree"
        workdirRef:
          objectKey: "my-workdir"
        createWorkdir: true
    id: forge/lib/git/clone
`, "REPO_ROOT", repoRoot)
}

// TestGitClone tests the git clone controller.
func TestGitClone(t *testing.T) {
	tb, err := testbed.Default(context.Background())
	if err != nil {
		t.Fatal(err.Error())
	}
	ctx := tb.Context
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	tgt, err := target_json.ResolveYAML(ctx, tb.Bus, []byte(buildTestYAML(t)))
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
