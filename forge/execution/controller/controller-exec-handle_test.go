package execution_controller

import (
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	forge_target "github.com/s4wave/spacewave/forge/target"
)

// TestExecutionHandleRetainsGrantedEpoch keeps a stale caller from adopting a
// newer owner's epoch and verifies that its Execution writes remain fenced.
func TestExecutionHandleRetainsGrantedEpoch(t *testing.T) {
	// Create a disabled Execution in an isolated in-memory World.
	ctx := t.Context()
	tb := world_testbed.MustDefault(t, ctx)
	peerID := tb.Volume.GetPeerID()
	const execKey = "exec/handle-granted-epoch"
	ts := timestamp.Now()
	_, err := forge_execution.CreateExecutionWithTarget(ctx, tb.WorldState, peerID, execKey, peerID, nil, &forge_target.Target{Exec: &forge_target.Exec{Disable: true}}, nil, ts)
	if err != nil {
		t.Fatal(err)
	}
	conf := NewConfig(tb.EngineID, execKey, peerID, &forge_target.InputWorld{EngineId: tb.EngineID})

	// Construct the native handle from the first granted claim.
	obj, err := world.MustGetObject(ctx, tb.WorldState, execKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := obj.ApplyObjectOp(ctx, execution_tx.NewTxStart(peerID, conf.GetClaimId()), peerID); err != nil {
		t.Fatal(err)
	}
	handle := newExecControllerHandle(ctx, NewController(tb.Logger, tb.Bus, conf), tb.WorldState, tb.WorldState, ts, 1)

	// Replace the authoritative claim without replacing the caller's handle.
	if _, _, err := obj.ApplyObjectOp(ctx, execution_tx.NewTxReclaim(peerID, "next-owner", 1), peerID); err != nil {
		t.Fatal(err)
	}
	if handle.GetExecutionObjectKey() != execKey || handle.GetExecutionClaimEpoch() != 1 {
		t.Fatalf("handle changed its granted context: %q epoch %d", handle.GetExecutionObjectKey(), handle.GetExecutionClaimEpoch())
	}

	// Reject the stale handle's write using the epoch it still reports to plugins.
	var stale *execution_tx.StaleClaimEpochError
	if err := handle.WriteLog(ctx, "info", "stale caller"); !errors.As(err, &stale) {
		t.Fatalf("stale log write returned %v, want StaleClaimEpochError", err)
	}
}
