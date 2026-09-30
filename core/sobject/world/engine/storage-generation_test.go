package sobject_world_engine

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/coord"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/sirupsen/logrus"
)

// TestProcessOpRejectsStaleStorageGeneration checks a transaction built before
// a storage generation advance is rejected.
func TestProcessOpRejectsStaleStorageGeneration(t *testing.T) {
	// Advance the World past generation 0.
	ctx := context.Background()
	c, so, headState := newProcessTestWorld(t, ctx)
	headState.StorageGeneration = 1

	// Process a transaction built on generation 0.
	objectTx, err := world_block_tx.NewTxCreateObject("stale-object", headState.GetHeadRef().CloneVT())
	if err != nil {
		t.Fatal(err.Error())
	}
	nextState, res, err := c.processOp(
		ctx,
		logrus.NewEntry(logrus.New()),
		so,
		marshalApplyTxOpForProcessTest(t, objectTx),
		"test-op",
		newProcessTestPeerID(t),
		1,
		0,
		headState,
	)
	if err != nil {
		t.Fatalf("process op returned error: %v", err)
	}

	// Check it is rejected.
	if nextState != nil {
		t.Fatal("stale transaction must not produce a next world state")
	}
	if res.GetSuccess() || res.GetErrorDetails().GetErrorMsg() != "storage generation is stale" {
		t.Fatalf("expected stale storage generation rejection, got %#v", res)
	}
}

// TestProcessAdvanceStorageGeneration checks only the validator advances the
// storage generation, and only from the current generation.
func TestProcessAdvanceStorageGeneration(t *testing.T) {
	// Build a World on generation 2 with a validator.
	ctx := context.Background()
	c, so, headState := newProcessTestWorld(t, ctx)
	so.peerID = newProcessTestPeerID(t)
	headState.StorageGeneration = 2

	// advance processes an advance from generation from, submitted by the
	// validator or another peer.
	advance := func(from uint64, submitter string) (*InnerState, *sobject.SOOperationResult) {
		// Encode the advance.
		t.Helper()
		opData, err := (&SOWorldOp{
			Body: &SOWorldOp_AdvanceStorageGeneration{
				AdvanceStorageGeneration: &AdvanceStorageGenerationOp{StorageGeneration: from},
			},
		}).MarshalVT()
		if err != nil {
			t.Fatal(err.Error())
		}

		// Process it from the submitter.
		pid := so.peerID
		if submitter != "validator" {
			pid = newProcessTestPeerID(t)
		}
		nextState, res, err := c.processOp(ctx, c.le, so, opData, "advance-op", pid, 1, 0, headState)
		if err != nil {
			t.Fatalf("process op returned error: %v", err)
		}
		return nextState, res
	}

	// The validator advances the current generation and keeps the head.
	nextState, res := advance(2, "validator")
	if !res.GetSuccess() {
		t.Fatalf("expected validator advance to succeed, got %#v", res)
	}
	if nextState.GetStorageGeneration() != 3 {
		t.Fatalf("expected storage generation 3, got %d", nextState.GetStorageGeneration())
	}
	if !nextState.GetHeadRef().EqualVT(headState.GetHeadRef()) {
		t.Fatal("advance must keep the World head")
	}

	// Another peer and a stale generation are rejected.
	for _, tc := range []struct {
		name      string
		from      uint64
		submitter string
		wantMsg   string
	}{
		{"other peer", 2, "other", "storage generation advance must come from the validator"},
		{"stale generation", 1, "validator", "storage generation is stale"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			nextState, res := advance(tc.from, tc.submitter)
			if nextState != nil || res.GetSuccess() {
				t.Fatal("expected advance to be rejected")
			}
			if msg := res.GetErrorDetails().GetErrorMsg(); msg != tc.wantMsg {
				t.Fatalf("expected %q, got %q", tc.wantMsg, msg)
			}
		})
	}
}

// TestFinalizeSpaceWorldCandidateRejectedAfterAdvanceIsStale checks a follower
// retries a candidate the storage generation advance rejected.
func TestFinalizeSpaceWorldCandidateRejectedAfterAdvanceIsStale(t *testing.T) {
	// Reject the candidate and publish the advance before the rejection
	// returns.
	ctx := context.Background()
	baseRoot := &sobject.SORoot{InnerSeqno: 1}
	baseWorldRoot := testFinalizationObjectRef(t, "base-world")
	candidateWorldRoot := testFinalizationObjectRef(t, "candidate-world")
	so := &testFinalizationSharedObject{
		snapshot:     newTestFinalizationSnapshot(t, baseRoot, baseWorldRoot),
		localStore:   newTestRejectedCandidateStore(),
		waitRejected: true,
		waitErr:      errors.New("storage generation is stale"),
	}
	so.afterWait = func() {
		// Publish generation 1 on the same World.
		stateData, err := (&InnerState{
			HeadRef:           baseWorldRoot.CloneVT(),
			StorageGeneration: 1,
		}).MarshalVT()
		if err != nil {
			t.Fatal(err.Error())
		}
		so.snapshot.rootInner = &sobject.SORootInner{Seqno: 2, StateData: stateData}
	}
	eng := &soEngine{so: so}

	// Finalize the candidate.
	decision, err := eng.finalizeSpaceWorldCandidate(
		ctx,
		newTestFinalizationPacket(t, baseRoot, baseWorldRoot, candidateWorldRoot, "candidate-op"),
		[]byte("serialized-world-op"),
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check the follower reruns the transaction.
	if decision.GetStatus() != SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_STALE_BASE {
		t.Fatalf("expected stale-base decision, got %s", decision.GetStatus().String())
	}
	if !decision.GetRetryable() {
		t.Fatal("expected stale-base decision to be retryable")
	}
	if !errors.Is(finalizationDecisionError(decision), coord.ErrStaleGeneration) {
		t.Fatal("expected stale-base decision to rerun the transaction")
	}
}

// TestAdvanceStorageGenerationWaitsForLocalWorld checks the reclaim fence
// queues nothing while the accepted World is not completely local.
func TestAdvanceStorageGenerationWaitsForLocalWorld(t *testing.T) {
	// Fence a World whose store cannot prove it complete.
	c := &Controller{le: logrus.NewEntry(logrus.New()), conf: &Config{}}
	so := &testMaintenanceSharedObject{
		snapshot: &testMaintenanceSnapshot{role: sobject.SOParticipantRole_SOParticipantRole_OWNER},
	}
	err := c.advanceStorageGeneration(t.Context(), so)

	// Check the fence refused without queueing.
	if !errors.Is(err, errStorageReclaimNotReady) {
		t.Fatalf("advance = %v; want not ready", err)
	}
	if len(so.queueOps) != 0 {
		t.Fatalf("queued %d operations", len(so.queueOps))
	}
}
