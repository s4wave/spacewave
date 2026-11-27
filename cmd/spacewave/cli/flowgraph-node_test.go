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

// TestFlowgraphNodeObjectBinding binds a node to a Space object from its flag
// and describes each binding state and declared port for the show output.
func TestFlowgraphNodeObjectBinding(t *testing.T) {
	// Bind a node by flag, and leave a node without the flag unbound.
	args := &flowgraphNodeSetArgs{typeID: "object", object: "chat/room"}
	node, err := args.buildNode()
	if err != nil {
		t.Fatal(err)
	}
	if node.GetObjectKey() != "chat/room" {
		t.Fatalf("node object key = %q", node.GetObjectKey())
	}
	if node, err = (&flowgraphNodeSetArgs{typeID: "object"}).buildNode(); err != nil || node.GetObjectKey() != "" {
		t.Fatalf("node without --object = %v, %v", node, err)
	}

	// Describe each state and the declared ports.
	bound := &flowgraph.FlowgraphBinding{
		ObjectKey: "chat/room",
		State:     flowgraph.FlowgraphBindingState_FLOWGRAPH_BINDING_STATE_BOUND,
		Ports: []*flowgraph.FlowgraphPort{
			{Name: "in", Direction: flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT, TypeId: "stream"},
			{Name: "out", Direction: flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT, TypeId: "event"},
		},
	}
	if got := describeBindingState(bound); got != "BOUND" {
		t.Fatalf("bound state = %q", got)
	}
	if got := describePorts(bound.GetPorts()); got != "in in:stream, out out:event" {
		t.Fatalf("declared ports = %q", got)
	}
	unavailable := &flowgraph.FlowgraphBinding{ObjectKey: "chat/room", State: flowgraph.FlowgraphBindingState_FLOWGRAPH_BINDING_STATE_UNAVAILABLE}
	if got := describeBindingState(unavailable); got != "UNAVAILABLE" {
		t.Fatalf("unavailable state = %q", got)
	}
	if got := describeBindingState(&flowgraph.FlowgraphBinding{State: flowgraph.FlowgraphBindingState_FLOWGRAPH_BINDING_STATE_UNREGISTERED, ObjectKey: "x"}); got != "UNREGISTERED" {
		t.Fatalf("unregistered state = %q", got)
	}
	if got := describeBindingState(&flowgraph.FlowgraphBinding{}); got != "UNBOUND" {
		t.Fatalf("unbound state = %q", got)
	}
}

// TestFlowgraphNodeKeyed sets a Step keyed from its flag and rejects the flag
// on a node that is not a Step.
func TestFlowgraphNodeKeyed(t *testing.T) {
	// A Step is unkeyed unless the flag is set.
	node, err := (&flowgraphNodeSetArgs{typeID: flowgraph.StepNodeTypeID}).buildNode()
	if err != nil || node.GetStep().GetKeyed() {
		t.Fatalf("Step without --keyed = %v, %v", node, err)
	}
	node, err = (&flowgraphNodeSetArgs{typeID: flowgraph.StepNodeTypeID, keyed: true}).buildNode()
	if err != nil || !node.GetStep().GetKeyed() {
		t.Fatalf("Step with --keyed = %v, %v", node, err)
	}

	// A Device node has no Step to key.
	if _, err := (&flowgraphNodeSetArgs{typeID: flowgraph.TCPPortNodeTypeID, keyed: true}).buildNode(); err == nil {
		t.Fatal("Device node accepted --keyed")
	}
}

// TestFlowgraphNodeTemplate selects a template placement without replacing the Step.
func TestFlowgraphNodeTemplate(t *testing.T) {
	// Resolve the template and describe it in the same format as graph
	// readback.
	args := &flowgraphNodeSetArgs{template: "template/fixer"}
	placement, err := args.buildPlacement()
	if err != nil {
		t.Fatal(err)
	}
	if placement.GetKind() != flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR_TEMPLATE || describePlacement(placement) != "template template/fixer" {
		t.Fatalf("template placement = %v", placement)
	}

	// A template cannot share its node's placement with another actor or Device.
	args.actor = "actor/existing"
	if _, err := args.buildPlacement(); err == nil {
		t.Fatal("template and actor accepted together")
	}
	args.actor, args.device = "", "device/local"
	if _, err := args.buildPlacement(); err == nil {
		t.Fatal("template and Device accepted together")
	}
}
