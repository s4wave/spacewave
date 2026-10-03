package execution_tx

import (
	"errors"
	"testing"
	"time"

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
	// Create the World testbed and register execution controller factories.
	t.Helper()
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	tb.StaticResolver.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(forge_lib_kvtx.NewFactory(tb.Bus))

	// Resolve the target and create the fixture execution.
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
		nil,
		timestamp.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Access the execution object for subsequent claim operations.
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
	// Read the current execution record through the fixture World.
	t.Helper()
	execution, objectState, err := forge_execution.LookupExecution(t.Context(), f.tb.WorldState, f.objKey)
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatal(err)
	}
	return execution
}

func TestSecondClaimantObservesLiveClaim(t *testing.T) {
	// Start the first execution claim in a fresh fixture.
	f := newClaimFixture(t)
	if err := f.apply(t, NewTxStart(f.peerID, time.Now().Add(time.Hour), "owner-1")); err != nil {
		t.Fatal(err)
	}

	// Require the first execution claim to begin at epoch one.
	execution := f.execution(t)
	if got := execution.GetClaim(); got.GetClaimId() != "owner-1" || got.GetEpoch() != 1 {
		t.Fatalf("claim = %q/%d, want owner-1/1", got.GetClaimId(), got.GetEpoch())
	}

	// Require a second claimant to observe the live claim.
	err := f.apply(t, NewTxStart(f.peerID, time.Now().Add(time.Hour), "owner-2"))
	var heldErr *ClaimHeldError
	if !errors.As(err, &heldErr) {
		t.Fatalf("second claim error = %v, want ClaimHeldError", err)
	}

	// Require a non-owner completion to preserve the running execution.
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
	// Replace the first execution claim, whose lease has expired, after
	// setting a waiting plugin.
	f := newClaimFixture(t)
	if err := f.apply(t, NewTxStart(f.peerID, time.Now().Add(-time.Minute), "owner-1")); err != nil {
		t.Fatal(err)
	}
	owner1 := &forge_execution.Claim{ClaimId: "owner-1", Epoch: 1}
	if err := f.apply(t, NewTxSetWaitingPlugin("plugin-a", owner1)); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := f.apply(t, NewTxReclaim(f.peerID, "owner-2", 1, now, now.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}

	// Require reclaim to advance the claim epoch and clear the waiting plugin.
	execution := f.execution(t)
	if got := execution.GetClaim(); got.GetClaimId() != "owner-2" || got.GetEpoch() != 2 {
		t.Fatalf("claim = %q/%d, want owner-2/2", got.GetClaimId(), got.GetEpoch())
	}
	if got := execution.GetWaitingPluginId(); got != "" {
		t.Fatalf("waiting plugin after reclaim = %q, want empty", got)
	}

	// Require stale plugin and completion writes to fail after reclaim.
	var staleErr *StaleClaimEpochError
	err := f.apply(t, NewTxSetWaitingPlugin("plugin-a", owner1))
	if !errors.As(err, &staleErr) {
		t.Fatalf("stale waiting plugin error = %v, want StaleClaimEpochError", err)
	}
	err = f.apply(t, NewTxComplete(
		forge_value.NewResultWithSuccess(),
		&forge_execution.Claim{ClaimId: "owner-1", Epoch: 1},
	))
	if !errors.As(err, &staleErr) {
		t.Fatalf("stale completion error = %v, want StaleClaimEpochError", err)
	}

	// Require stale output writes to fail after reclaim.
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

	// Require stale log writes to fail after reclaim.
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

	// Require stale writes to preserve the running execution contents.
	execution = f.execution(t)
	if state := execution.GetExecutionState(); state != forge_execution.State_ExecutionState_RUNNING {
		t.Fatalf("state = %s, want RUNNING", state)
	}
	if len(execution.GetValueSet().GetOutputs()) != 0 || len(execution.GetLogEntries()) != 0 {
		t.Fatal("stale owner changed execution output or log state")
	}

	// Complete the execution with its current claim.
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
	// Retry the execution start with the same claim identity.
	f := newClaimFixture(t)
	if err := f.apply(t, NewTxStart(f.peerID, time.Now().Add(time.Hour), "owner-1")); err != nil {
		t.Fatal(err)
	}
	if err := f.apply(t, NewTxStart(f.peerID, time.Now().Add(time.Hour), "owner-1")); err != nil {
		t.Fatalf("same owner retry: %v", err)
	}

	// Require the retried start to preserve the original claim epoch.
	execution := f.execution(t)
	if got := execution.GetClaim(); got.GetClaimId() != "owner-1" || got.GetEpoch() != 1 {
		t.Fatalf("claim after retry = %q/%d, want owner-1/1", got.GetClaimId(), got.GetEpoch())
	}
}

func TestReclaimRequiresExpiredLease(t *testing.T) {
	// Start a claim whose lease is live.
	f := newClaimFixture(t)
	now := time.Now()
	if err := f.apply(t, NewTxStart(f.peerID, now.Add(time.Hour), "owner-1")); err != nil {
		t.Fatal(err)
	}

	// Require a reclaim observed before the lease expires to leave the claim in place.
	err := f.apply(t, NewTxReclaim(f.peerID, "owner-2", 1, now, now.Add(time.Hour)))
	if _, ok := errors.AsType[*ClaimLiveError](err); !ok {
		t.Fatalf("early reclaim error = %v, want ClaimLiveError", err)
	}
	if got := f.execution(t).GetClaim(); got.GetClaimId() != "owner-1" || got.GetEpoch() != 1 {
		t.Fatalf("claim = %q/%d, want owner-1/1", got.GetClaimId(), got.GetEpoch())
	}

	// Require a reclaim observed at the lease expiry to take the claim and its new lease.
	expiry := now.Add(time.Hour)
	newLease := expiry.Add(time.Minute)
	if err := f.apply(t, NewTxReclaim(f.peerID, "owner-2", 1, expiry, newLease)); err != nil {
		t.Fatal(err)
	}
	got := f.execution(t).GetClaim()
	if got.GetClaimId() != "owner-2" || got.GetEpoch() != 2 || !got.GetLeaseExpiresAt().AsTime().Equal(newLease) {
		t.Fatalf("claim = %q/%d lease %v, want owner-2/2 lease %v", got.GetClaimId(), got.GetEpoch(), got.GetLeaseExpiresAt().AsTime(), newLease)
	}

	// Require a second reclaim of the replaced claim to lose the epoch fence.
	later := newLease.Add(time.Minute)
	err = f.apply(t, NewTxReclaim(f.peerID, "owner-3", 1, later, later.Add(time.Hour)))
	if _, ok := errors.AsType[*StaleClaimEpochError](err); !ok {
		t.Fatalf("second reclaim error = %v, want StaleClaimEpochError", err)
	}
}

func TestRenewClaimExtendsLease(t *testing.T) {
	// Start a claim and renew its lease.
	f := newClaimFixture(t)
	now := time.Now()
	if err := f.apply(t, NewTxStart(f.peerID, now.Add(time.Minute), "owner-1")); err != nil {
		t.Fatal(err)
	}
	owner1 := &forge_execution.Claim{ClaimId: "owner-1", Epoch: 1}
	renewed := now.Add(time.Hour)
	if err := f.apply(t, NewTxRenewClaim(owner1, renewed)); err != nil {
		t.Fatal(err)
	}
	if got := f.execution(t).GetClaim().GetLeaseExpiresAt().AsTime(); !got.Equal(renewed) {
		t.Fatalf("lease = %v, want %v", got, renewed)
	}

	// Require a renewal that does not extend the lease to change nothing.
	if err := f.apply(t, NewTxRenewClaim(owner1, now.Add(time.Minute))); err != nil {
		t.Fatal(err)
	}
	if got := f.execution(t).GetClaim().GetLeaseExpiresAt().AsTime(); !got.Equal(renewed) {
		t.Fatalf("lease after shorter renewal = %v, want %v", got, renewed)
	}

	// Require a reclaim observed before the renewed lease expires to fail.
	err := f.apply(t, NewTxReclaim(f.peerID, "owner-2", 1, now.Add(2*time.Minute), now.Add(time.Hour)))
	if _, ok := errors.AsType[*ClaimLiveError](err); !ok {
		t.Fatalf("reclaim of renewed claim error = %v, want ClaimLiveError", err)
	}

	// Require the previous owner's renewal to fail the epoch fence after reclaim.
	expiry := renewed.Add(time.Minute)
	if err := f.apply(t, NewTxReclaim(f.peerID, "owner-2", 1, expiry, expiry.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	err = f.apply(t, NewTxRenewClaim(owner1, expiry.Add(time.Hour)))
	if _, ok := errors.AsType[*StaleClaimEpochError](err); !ok {
		t.Fatalf("stale renewal error = %v, want StaleClaimEpochError", err)
	}
}

func TestReclaimCancelingExecution(t *testing.T) {
	// Cancel an execution whose claimant then missed its lease.
	f := newClaimFixture(t)
	now := time.Now()
	if err := f.apply(t, NewTxStart(f.peerID, now.Add(-time.Minute), "owner-1")); err != nil {
		t.Fatal(err)
	}
	if err := f.apply(t, NewTxCancel()); err != nil {
		t.Fatal(err)
	}

	// Require another claimant to take the canceling execution.
	if err := f.apply(t, NewTxReclaim(f.peerID, "owner-2", 1, now, now.Add(time.Hour))); err != nil {
		t.Fatal(err)
	}
	execution := f.execution(t)
	if state := execution.GetExecutionState(); state != forge_execution.State_ExecutionState_CANCELING {
		t.Fatalf("state = %s, want CANCELING", state)
	}
	if got := execution.GetClaim(); got.GetClaimId() != "owner-2" || got.GetEpoch() != 2 {
		t.Fatalf("claim = %q/%d, want owner-2/2", got.GetClaimId(), got.GetEpoch())
	}
}
