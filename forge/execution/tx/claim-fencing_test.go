package execution_tx

import (
	"errors"
	"testing"

	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_lib_kvtx "github.com/s4wave/spacewave/forge/lib/kvtx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	target_mock "github.com/s4wave/spacewave/forge/target/mock"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

type claimFixture struct {
	tb     *world_testbed.Testbed
	peerID peer.ID
	objKey string
	obj    world.ObjectState
}

func newClaimFixture(t *testing.T) *claimFixture {
	t.Helper()

	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	tb.StaticResolver.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(forge_lib_kvtx.NewFactory(tb.Bus))

	target, err := target_mock.ResolveMockTarget(ctx, tb.Bus)
	if err != nil {
		t.Fatal(err)
	}
	peerID := tb.Volume.GetPeerID()
	objKey := "test/execution/claim-fencing"
	_, err = forge_execution.CreateExecutionWithTarget(
		ctx,
		tb.WorldState,
		peerID,
		objKey,
		peerID,
		forge_target.NewValueSet(),
		target,
		timestamp.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := world.MustGetObject(ctx, tb.WorldState, objKey)
	if err != nil {
		t.Fatal(err)
	}
	return &claimFixture{tb: tb, peerID: peerID, objKey: objKey, obj: obj}
}

func (f *claimFixture) apply(t *testing.T, tx *Tx) error {
	t.Helper()
	_, _, err := f.obj.ApplyObjectOp(t.Context(), tx, f.peerID)
	return err
}

func (f *claimFixture) execution(t *testing.T) *forge_execution.Execution {
	t.Helper()
	execution, _, err := forge_execution.LookupExecution(t.Context(), f.tb.WorldState, f.objKey)
	if err != nil {
		t.Fatal(err)
	}
	return execution
}

func TestSecondClaimantObservesLiveClaim(t *testing.T) {
	f := newClaimFixture(t)
	if err := f.apply(t, NewTxStart(f.peerID, "owner-1")); err != nil {
		t.Fatal(err)
	}

	execution := f.execution(t)
	if got := execution.GetClaim(); got.GetClaimId() != "owner-1" || got.GetEpoch() != 1 {
		t.Fatalf("claim = %q/%d, want owner-1/1", got.GetClaimId(), got.GetEpoch())
	}

	err := f.apply(t, NewTxStart(f.peerID, "owner-2"))
	var heldErr *ClaimHeldError
	if !errors.As(err, &heldErr) {
		t.Fatalf("second claim error = %v, want ClaimHeldError", err)
	}

	err = f.apply(t, NewTxComplete(
		forge_value.NewResultWithSuccess(),
		&forge_execution.Claim{ClaimId: "owner-2", Epoch: 1},
	))
	if !errors.As(err, &heldErr) {
		t.Fatalf("non-owner completion error = %v, want ClaimHeldError", err)
	}
	if state := f.execution(t).GetExecutionState(); state != forge_execution.State_ExecutionState_RUNNING {
		t.Fatalf("state = %s, want RUNNING", state)
	}
}

func TestStaleWritesRejectedAfterReclaim(t *testing.T) {
	f := newClaimFixture(t)
	if err := f.apply(t, NewTxStart(f.peerID, "owner-1")); err != nil {
		t.Fatal(err)
	}
	if err := f.apply(t, NewTxReclaim(f.peerID, "owner-2", 1)); err != nil {
		t.Fatal(err)
	}

	execution := f.execution(t)
	if got := execution.GetClaim(); got.GetClaimId() != "owner-2" || got.GetEpoch() != 2 {
		t.Fatalf("claim = %q/%d, want owner-2/2", got.GetClaimId(), got.GetEpoch())
	}

	var staleErr *StaleClaimEpochError
	err := f.apply(t, NewTxComplete(
		forge_value.NewResultWithSuccess(),
		&forge_execution.Claim{ClaimId: "owner-1", Epoch: 1},
	))
	if !errors.As(err, &staleErr) {
		t.Fatalf("stale completion error = %v, want StaleClaimEpochError", err)
	}

	setOutputs, err := NewTxSetOutputs(
		nil,
		true,
		&forge_execution.Claim{ClaimId: "owner-1", Epoch: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.apply(t, setOutputs); !errors.As(err, &staleErr) {
		t.Fatalf("stale output error = %v, want StaleClaimEpochError", err)
	}

	appendLog, err := NewTxAppendLog(
		[]*forge_execution.LogEntry{{Timestamp: timestamp.Now(), Message: "late"}},
		&forge_execution.Claim{ClaimId: "owner-1", Epoch: 1},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.apply(t, appendLog); !errors.As(err, &staleErr) {
		t.Fatalf("stale log error = %v, want StaleClaimEpochError", err)
	}

	execution = f.execution(t)
	if state := execution.GetExecutionState(); state != forge_execution.State_ExecutionState_RUNNING {
		t.Fatalf("state = %s, want RUNNING", state)
	}
	if len(execution.GetValueSet().GetOutputs()) != 0 || len(execution.GetLogEntries()) != 0 {
		t.Fatal("stale owner changed execution output or log state")
	}

	err = f.apply(t, NewTxComplete(
		forge_value.NewResultWithSuccess(),
		&forge_execution.Claim{ClaimId: "owner-2", Epoch: 2},
	))
	if err != nil {
		t.Fatalf("current owner completion: %v", err)
	}
	if state := f.execution(t).GetExecutionState(); state != forge_execution.State_ExecutionState_COMPLETE {
		t.Fatalf("state = %s, want COMPLETE after current owner completion", state)
	}
}

func TestClaimOwnerSurvivesControllerRetry(t *testing.T) {
	f := newClaimFixture(t)
	if err := f.apply(t, NewTxStart(f.peerID, "owner-1")); err != nil {
		t.Fatal(err)
	}
	if err := f.apply(t, NewTxStart(f.peerID, "owner-1")); err != nil {
		t.Fatalf("same owner retry: %v", err)
	}

	execution := f.execution(t)
	if got := execution.GetClaim(); got.GetClaimId() != "owner-1" || got.GetEpoch() != 1 {
		t.Fatalf("claim after retry = %q/%d, want owner-1/1", got.GetClaimId(), got.GetEpoch())
	}
}
