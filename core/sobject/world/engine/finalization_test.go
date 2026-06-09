package sobject_world_engine

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_hashmap "github.com/s4wave/spacewave/db/kvtx/hashmap"
	bifhash "github.com/s4wave/spacewave/net/hash"
)

func TestSpaceWorldFinalizationPacketValidate(t *testing.T) {
	valid := &SpaceWorldFinalizationPacket{
		BaseSharedObjectRoot: &sobject.SORoot{InnerSeqno: 7},
		BaseWorldRoot:        testFinalizationObjectRef(t, "base"),
		CandidateWorldRoot:   testFinalizationObjectRef(t, "candidate"),
		CandidateContentId:   []byte("candidate-content"),
		StorageGeneration:    11,
		AuthorityEpoch:       13,
		BlocksAvailable:      true,
		Op: &SOWorldOp{
			Body: &SOWorldOp_ApplyTxOp{ApplyTxOp: &ApplyTxOp{}},
		},
		FollowerParticipantId: "follower",
		LocalOperationId:      "local-op",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid packet rejected: %v", err)
	}

	missingCandidate := valid.CloneVT()
	missingCandidate.CandidateContentId = nil
	if err := missingCandidate.Validate(); err == nil {
		t.Fatal("expected missing candidate content id to reject")
	}
}

func TestSpaceWorldFinalizationDecisionValidate(t *testing.T) {
	accepted := &SpaceWorldFinalizationDecision{
		Status:                   SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_ACCEPTED,
		AcceptedSharedObjectRoot: &sobject.SORoot{InnerSeqno: 8},
		AcceptedWorldRoot:        testFinalizationObjectRef(t, "accepted"),
		LocalOperationId:         "local-op",
	}
	if err := accepted.Validate(); err != nil {
		t.Fatalf("valid accepted decision rejected: %v", err)
	}

	missingRoot := accepted.CloneVT()
	missingRoot.AcceptedWorldRoot = nil
	if err := missingRoot.Validate(); err == nil {
		t.Fatal("expected accepted decision without World root to reject")
	}

	rejected := &SpaceWorldFinalizationDecision{
		Status:           SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_STALE_BASE,
		Error:            "stale base",
		Retryable:        true,
		LocalOperationId: "local-op",
	}
	if err := rejected.Validate(); err != nil {
		t.Fatalf("valid stale-base decision rejected: %v", err)
	}
}

func TestFinalizeSpaceWorldCandidateAcceptedUsesAuthorityState(t *testing.T) {
	ctx := context.Background()
	baseRoot := &sobject.SORoot{InnerSeqno: 1}
	baseWorldRoot := testFinalizationObjectRef(t, "base-world")
	acceptedRoot := &sobject.SORoot{InnerSeqno: 2}
	acceptedWorldRoot := testFinalizationObjectRef(t, "accepted-world")

	snap := newTestFinalizationSnapshot(t, baseRoot, baseWorldRoot)
	so := &testFinalizationSharedObject{snapshot: snap}
	so.afterWait = func() {
		snap.setRoot(t, acceptedRoot, acceptedWorldRoot)
	}
	eng := &soEngine{so: so}
	packet := newTestFinalizationPacket(t, baseRoot, baseWorldRoot, acceptedWorldRoot, "candidate-op")
	opData := []byte("serialized-world-op")

	decision, err := eng.finalizeSpaceWorldCandidate(ctx, packet, opData)
	if err != nil {
		t.Fatal(err.Error())
	}
	if decision.GetStatus() != SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_ACCEPTED {
		t.Fatalf("expected accepted decision, got %s", decision.GetStatus().String())
	}
	if !decision.GetAcceptedSharedObjectRoot().EqualVT(acceptedRoot) {
		t.Fatalf("expected accepted SharedObject root %v, got %v", acceptedRoot, decision.GetAcceptedSharedObjectRoot())
	}
	if !decision.GetAcceptedWorldRoot().EqualsRef(acceptedWorldRoot) {
		t.Fatalf("expected accepted World root %s, got %s", acceptedWorldRoot.MarshalString(), decision.GetAcceptedWorldRoot().MarshalString())
	}
	if decision.GetLocalOperationId() != "candidate-op" {
		t.Fatalf("expected candidate correlation id to round trip, got %q", decision.GetLocalOperationId())
	}
	if len(so.queuedOps) != 1 || !slices.Equal(so.queuedOps[0], opData) {
		t.Fatalf("expected one queued authority op matching candidate data, got %d", len(so.queuedOps))
	}
}

func TestFinalizeSpaceWorldCandidateStaleBaseDoesNotQueue(t *testing.T) {
	ctx := context.Background()
	staleRoot := &sobject.SORoot{InnerSeqno: 1}
	currentRoot := &sobject.SORoot{InnerSeqno: 2}
	baseWorldRoot := testFinalizationObjectRef(t, "base-world")
	candidateWorldRoot := testFinalizationObjectRef(t, "candidate-world")
	so := &testFinalizationSharedObject{
		snapshot:   newTestFinalizationSnapshot(t, currentRoot, baseWorldRoot),
		localStore: newTestRejectedCandidateStore(),
	}
	eng := &soEngine{so: so}

	decision, err := eng.finalizeSpaceWorldCandidate(
		ctx,
		newTestFinalizationPacket(t, staleRoot, baseWorldRoot, candidateWorldRoot, "candidate-op"),
		[]byte("serialized-world-op"),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if decision.GetStatus() != SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_STALE_BASE {
		t.Fatalf("expected stale-base decision, got %s", decision.GetStatus().String())
	}
	if !decision.GetRetryable() {
		t.Fatal("expected stale-base decision to be retryable")
	}
	if len(so.queuedOps) != 0 {
		t.Fatalf("expected stale candidate not to queue authority op, queued %d", len(so.queuedOps))
	}
	record := readTestRejectedCandidateRecord(t, ctx, so.localStore, "candidate-op")
	if record.GetDecision().GetStatus() != SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_STALE_BASE {
		t.Fatalf("expected stale-base retention record, got %s", record.GetDecision().GetStatus().String())
	}
}

func TestFinalizeSpaceWorldCandidateMissingBlocksDoesNotQueue(t *testing.T) {
	ctx := context.Background()
	baseRoot := &sobject.SORoot{InnerSeqno: 1}
	baseWorldRoot := testFinalizationObjectRef(t, "base-world")
	candidateWorldRoot := testFinalizationObjectRef(t, "candidate-world")
	so := &testFinalizationSharedObject{
		snapshot:   newTestFinalizationSnapshot(t, baseRoot, baseWorldRoot),
		localStore: newTestRejectedCandidateStore(),
	}
	eng := &soEngine{so: so}
	packet := newTestFinalizationPacket(t, baseRoot, baseWorldRoot, candidateWorldRoot, "candidate-op")
	packet.BlocksAvailable = false

	decision, err := eng.finalizeSpaceWorldCandidate(ctx, packet, []byte("serialized-world-op"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if decision.GetStatus() != SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_MISSING_BLOCK {
		t.Fatalf("expected missing-block decision, got %s", decision.GetStatus().String())
	}
	if !decision.GetRetryable() {
		t.Fatal("expected missing-block decision to be retryable")
	}
	if len(so.queuedOps) != 0 {
		t.Fatalf("expected missing-block candidate not to queue authority op, queued %d", len(so.queuedOps))
	}
	record := readTestRejectedCandidateRecord(t, ctx, so.localStore, "candidate-op")
	if record.GetDecision().GetStatus() != SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_MISSING_BLOCK {
		t.Fatalf("expected missing-block retention record, got %s", record.GetDecision().GetStatus().String())
	}
}

func TestFinalizeSpaceWorldCandidateUnavailableRetentionStoreDoesNotBlockDecision(t *testing.T) {
	ctx := context.Background()
	baseRoot := &sobject.SORoot{InnerSeqno: 1}
	baseWorldRoot := testFinalizationObjectRef(t, "base-world")
	candidateWorldRoot := testFinalizationObjectRef(t, "candidate-world")
	so := &testFinalizationSharedObject{
		snapshot: newTestFinalizationSnapshot(t, baseRoot, baseWorldRoot),
	}
	eng := &soEngine{so: so}
	packet := newTestFinalizationPacket(t, baseRoot, baseWorldRoot, candidateWorldRoot, "candidate-op")
	packet.BlocksAvailable = false

	decision, err := eng.finalizeSpaceWorldCandidate(ctx, packet, []byte("serialized-world-op"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if decision.GetStatus() != SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_MISSING_BLOCK {
		t.Fatalf("expected missing-block decision, got %s", decision.GetStatus().String())
	}
	if len(so.queuedOps) != 0 {
		t.Fatalf("expected unavailable retention store not to queue authority op, queued %d", len(so.queuedOps))
	}
}

func TestFinalizeSpaceWorldCandidateRejectedClearsAuthorityResult(t *testing.T) {
	ctx := context.Background()
	baseRoot := &sobject.SORoot{InnerSeqno: 1}
	baseWorldRoot := testFinalizationObjectRef(t, "base-world")
	candidateWorldRoot := testFinalizationObjectRef(t, "candidate-world")
	so := &testFinalizationSharedObject{
		snapshot:     newTestFinalizationSnapshot(t, baseRoot, baseWorldRoot),
		localStore:   newTestRejectedCandidateStore(),
		waitRejected: true,
		waitErr:      errors.New("operation rejected"),
	}
	eng := &soEngine{so: so}

	decision, err := eng.finalizeSpaceWorldCandidate(
		ctx,
		newTestFinalizationPacket(t, baseRoot, baseWorldRoot, candidateWorldRoot, "candidate-op"),
		[]byte("serialized-world-op"),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if decision.GetStatus() != SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_REJECTED {
		t.Fatalf("expected rejected decision, got %s", decision.GetStatus().String())
	}
	if !strings.Contains(decision.GetError(), "operation rejected") {
		t.Fatalf("expected rejection reason, got %q", decision.GetError())
	}
	if !slices.Equal(so.clearedIDs, []string{"authority-op"}) {
		t.Fatalf("expected rejected authority result to be cleared, got %v", so.clearedIDs)
	}
	record := readTestRejectedCandidateRecord(t, ctx, so.localStore, "candidate-op")
	if record.GetDecision().GetStatus() != SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_REJECTED {
		t.Fatalf("expected rejected retention record, got %s", record.GetDecision().GetStatus().String())
	}
	if err := eng.clearRejectedSpaceWorldCandidate(ctx, "candidate-op"); err != nil {
		t.Fatal(err.Error())
	}
	assertTestRejectedCandidateMissing(t, ctx, so.localStore, "candidate-op")
}

func testFinalizationObjectRef(t *testing.T, seed string) *bucket.ObjectRef {
	t.Helper()
	h, err := bifhash.Sum(bifhash.HashType_HashType_SHA256, []byte(seed))
	if err != nil {
		t.Fatal(err.Error())
	}
	return &bucket.ObjectRef{
		BucketId: "world",
		RootRef:  &block.BlockRef{Hash: h},
	}
}

func newTestFinalizationPacket(
	t *testing.T,
	baseRoot *sobject.SORoot,
	baseWorldRoot *bucket.ObjectRef,
	candidateWorldRoot *bucket.ObjectRef,
	localOperationID string,
) *SpaceWorldFinalizationPacket {
	t.Helper()
	return &SpaceWorldFinalizationPacket{
		BaseSharedObjectRoot: baseRoot.CloneVT(),
		BaseWorldRoot:        baseWorldRoot.CloneVT(),
		CandidateWorldRoot:   candidateWorldRoot.CloneVT(),
		CandidateContentId:   []byte("candidate-content"),
		BlocksAvailable:      true,
		Op: &SOWorldOp{
			Body: &SOWorldOp_ApplyTxOp{ApplyTxOp: &ApplyTxOp{}},
		},
		FollowerParticipantId: "follower",
		LocalOperationId:      localOperationID,
	}
}

func newTestFinalizationSnapshot(t *testing.T, root *sobject.SORoot, worldRoot *bucket.ObjectRef) *testFinalizationSnapshot {
	t.Helper()
	snap := &testFinalizationSnapshot{}
	snap.setRoot(t, root, worldRoot)
	return snap
}

type testFinalizationSnapshot struct {
	testSharedObjectSnapshot
	root *sobject.SORoot
}

func (s *testFinalizationSnapshot) setRoot(t *testing.T, root *sobject.SORoot, worldRoot *bucket.ObjectRef) {
	t.Helper()
	stateData, err := (&InnerState{HeadRef: worldRoot.CloneVT()}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	s.root = root.CloneVT()
	s.rootInner = &sobject.SORootInner{
		Seqno:     root.GetInnerSeqno(),
		StateData: stateData,
	}
}

func (s *testFinalizationSnapshot) GetRootState(ctx context.Context) (*sobject.SORoot, error) {
	return s.root.CloneVT(), nil
}

type testFinalizationSharedObject struct {
	testSharedObject
	snapshot     *testFinalizationSnapshot
	localStore   kvtx.Store
	queuedOps    [][]byte
	clearedIDs   []string
	waitRejected bool
	waitErr      error
	afterWait    func()
}

func (s *testFinalizationSharedObject) GetSharedObjectState(ctx context.Context) (sobject.SharedObjectStateSnapshot, error) {
	return s.snapshot, nil
}

func (s *testFinalizationSharedObject) AccessLocalStateStore(ctx context.Context, storeID string, released func()) (kvtx.Store, func(), error) {
	if s.localStore == nil {
		return nil, nil, errors.New("local state store unavailable")
	}
	return s.localStore, func() {}, nil
}

func (s *testFinalizationSharedObject) QueueOperation(ctx context.Context, op []byte) (string, error) {
	s.queuedOps = append(s.queuedOps, append([]byte(nil), op...))
	return "authority-op", nil
}

func (s *testFinalizationSharedObject) WaitOperation(ctx context.Context, localID string) (uint64, bool, error) {
	if s.afterWait != nil {
		s.afterWait()
	}
	return 0, s.waitRejected, s.waitErr
}

func (s *testFinalizationSharedObject) ClearOperationResult(ctx context.Context, localID string) error {
	s.clearedIDs = append(s.clearedIDs, localID)
	return nil
}

func newTestRejectedCandidateStore() kvtx.Store {
	return store_kvtx_hashmap.NewHashmapKvtx(store_kvtx_hashmap.NewHashmap[[]byte]())
}

func readTestRejectedCandidateRecord(
	t *testing.T,
	ctx context.Context,
	store kvtx.Store,
	localOperationID string,
) *SpaceWorldRejectedCandidate {
	t.Helper()
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()
	data, found, err := tx.Get(ctx, spaceWorldRejectedCandidateKeyForID(localOperationID))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.Fatalf("expected rejected candidate record for %q", localOperationID)
	}
	record := &SpaceWorldRejectedCandidate{}
	if err := record.UnmarshalVT(data); err != nil {
		t.Fatal(err.Error())
	}
	if record.GetRetainedUnixNano() == 0 {
		t.Fatal("expected retained timestamp")
	}
	return record
}

func assertTestRejectedCandidateMissing(
	t *testing.T,
	ctx context.Context,
	store kvtx.Store,
	localOperationID string,
) {
	t.Helper()
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tx.Discard()
	found, err := tx.Exists(ctx, spaceWorldRejectedCandidateKeyForID(localOperationID))
	if err != nil {
		t.Fatal(err.Error())
	}
	if found {
		t.Fatalf("expected rejected candidate record %q to be cleared", localOperationID)
	}
}
