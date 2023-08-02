package forge_lib_git_clone

import (
	"context"
	"testing"

	forge_target "github.com/aperturerobotics/forge/target"
	target_json "github.com/aperturerobotics/forge/target/json"
	"github.com/aperturerobotics/forge/testbed"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/timestamp"
)

const testYAML = `
# note: this test is just for Execution controller.
# the inputs / outputs listed in the Target are not used.
exec:
  controller:
    # rev: 0 -> defaults to 1
    config:
      objectKey: "my-repo"
      cloneOpts:
        url: "../../../"
      worktreeOpts:
        objectKey: "my-worktree"
        workdirRef:
          objectKey: "my-workdir"
        createWorkdir: true
    id: forge/lib/git/clone
`

// TestGitClone tests the git clone controller.
func TestGitClone(t *testing.T) {
	tb, err := testbed.Default(context.Background())
	if err != nil {
		t.Fatal(err.Error())
	}
	ctx := tb.Context
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	tgt, err := target_json.ResolveYAML(ctx, tb.Bus, []byte(testYAML))
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
