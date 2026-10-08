//go:build !js

package spacewave_cli

import (
	"os"
	"path/filepath"
	"testing"

	forge_target "github.com/s4wave/spacewave/forge/target"
	flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// TestFlowgraphNodeTargetAndBound preserves the typed execution definition and
// both bounds when authoring a code Step from CLI flags.
func TestFlowgraphNodeTargetAndBound(t *testing.T) {
	// Write the Target using its production protobuf JSON codec.
	target := &forge_target.Target{Outputs: []*forge_target.Output{
		{Name: "chosen", OutputType: forge_target.OutputType_OutputType_EXEC, ExecOutput: "chosen"},
	}}
	data, err := target.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "target.json")
	if err := os.WriteFile(file, data, 0o600); err != nil {
		t.Fatal(err)
	}

	// Decode the same execution definition while retaining the bounds.
	args := &flowgraphNodeSetArgs{typeID: flowgraph.StepNodeTypeID, targetFile: file, maxVisits: 3, maxSpend: 9}
	node, err := args.buildNode()
	if err != nil {
		t.Fatal(err)
	}
	if !node.GetStep().GetTarget().EqualVT(target) {
		t.Fatal("Step Target changed during CLI decoding")
	}
	if !node.GetStep().GetBound().EqualVT(&flowgraph.FlowgraphBound{MaxVisits: 3, MaxSpend: 9}) {
		t.Fatal("Step bounds changed during CLI decoding")
	}

	// Reject a Target on a standing Device node.
	args.typeID = flowgraph.TCPPortNodeTypeID
	if _, err := args.buildNode(); err == nil {
		t.Fatal("Device node accepted a Step Target")
	}
}
