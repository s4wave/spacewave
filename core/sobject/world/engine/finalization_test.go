package sobject_world_engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
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

func TestStaleFinalizationDecisionIsRetryableGeneration(t *testing.T) {
	err := finalizationDecisionError(&SpaceWorldFinalizationDecision{
		Status: SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_STALE_BASE,
		Error:  "base World root is stale",
	})
	if !errors.Is(err, coord.ErrStaleGeneration) {
		t.Fatalf("stale decision error = %v, want ErrStaleGeneration", err)
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

func TestFinalizeSpaceWorldCandidateMissingBlockRestoresAndResubmits(t *testing.T) {
	// Build a two-block candidate World.
	ctx := context.Background()
	leafData := []byte("leaf")
	leaf, err := block.BuildBlockRef(leafData, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	rootData := []byte("root")
	rootRef, err := block.BuildBlockRef(rootData, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Store it in a store that counts writes.
	puts := &testPutCountStore{StoreOps: block_mock.NewMockStore(0)}
	err = puts.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: leaf, Data: leafData},
		{Ref: rootRef, Data: rootData, Refs: []*block.BlockRef{leaf}},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	puts.n = 0
	store := &testUploadBlockStore{testBlockStore: newTestBlockStore("world", puts)}

	// Reject the first submission as missing a block and accept the second.
	baseRoot := &sobject.SORoot{InnerSeqno: 1}
	baseWorldRoot := testFinalizationObjectRef(t, "base-world")
	candidateWorldRoot := &bucket.ObjectRef{BucketId: "world", RootRef: rootRef}
	snap := newTestFinalizationSnapshot(t, baseRoot, baseWorldRoot)
	so := &testFinalizationSharedObject{
		testSharedObject: testSharedObject{blockStore: store},
		snapshot:         snap,
		localStore:       newTestRejectedCandidateStore(),
		waitRejected:     true,
		waitErr:          fmt.Errorf("%w: %w", sobject.ErrRejectedOp, block.ErrNotFound),
	}
	so.afterWait = func() {
		if len(so.queuedOps) == 2 {
			so.waitRejected, so.waitErr = false, nil
			snap.setRoot(t, &sobject.SORoot{InnerSeqno: 2}, candidateWorldRoot)
		}
	}
	eng := &soEngine{so: so}

	// Finalize: the candidate's blocks are written again and uploaded.
	decision, err := eng.finalizeSpaceWorldCandidate(
		ctx,
		newTestFinalizationPacket(t, baseRoot, baseWorldRoot, candidateWorldRoot, "candidate-op"),
		[]byte("serialized-world-op"),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if decision.GetStatus() != SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_ACCEPTED {
		t.Fatalf("expected accepted decision, got %s: %s", decision.GetStatus().String(), decision.GetError())
	}
	if len(so.queuedOps) != 2 || !slices.Equal(so.clearedIDs, []string{"authority-op"}) {
		t.Fatalf("expected one cleared rejection and a resubmit, got %d ops and cleared %v", len(so.queuedOps), so.clearedIDs)
	}
	if puts.n != 2 || store.waited != 1 {
		t.Fatalf("expected both blocks re-put and one upload wait, got %d puts and %d waits", puts.n, store.waited)
	}
}

func TestFinalizeSpaceWorldCandidateUnrestorableMissingBlock(t *testing.T) {
	// Reject every submission as missing a block the follower cannot restore.
	ctx := context.Background()
	baseRoot := &sobject.SORoot{InnerSeqno: 1}
	baseWorldRoot := testFinalizationObjectRef(t, "base-world")
	so := &testFinalizationSharedObject{
		testSharedObject: testSharedObject{blockStore: newTestBlockStore("world", block_mock.NewMockStore(0))},
		snapshot:         newTestFinalizationSnapshot(t, baseRoot, baseWorldRoot),
		localStore:       newTestRejectedCandidateStore(),
		waitRejected:     true,
		waitErr:          fmt.Errorf("%w: %w", sobject.ErrRejectedOp, block.ErrNotFound),
	}
	eng := &soEngine{so: so}

	// Finalize: the restore fails and the decision is a retryable miss.
	decision, err := eng.finalizeSpaceWorldCandidate(
		ctx,
		newTestFinalizationPacket(t, baseRoot, baseWorldRoot, testFinalizationObjectRef(t, "candidate-world"), "candidate-op"),
		[]byte("serialized-world-op"),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if decision.GetStatus() != SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_MISSING_BLOCK || !decision.GetRetryable() {
		t.Fatalf("expected retryable missing-block decision, got %s", decision.GetStatus().String())
	}
	if len(so.queuedOps) != 1 {
		t.Fatalf("expected no resubmit after a failed restore, got %d ops", len(so.queuedOps))
	}
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

// testPutCountStore counts the blocks written through it and keeps their refs,
// as a volume does.
type testPutCountStore struct {
	block.StoreOps
	n    int
	refs map[string][]*block.BlockRef
}

func (s *testPutCountStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	// Record each entry's refs before writing it.
	if s.refs == nil {
		s.refs = make(map[string][]*block.BlockRef)
	}
	for _, entry := range entries {
		s.refs[entry.Ref.MarshalString()] = entry.Refs
	}
	s.n += len(entries)
	return s.StoreOps.PutBlockBatch(ctx, entries)
}

func (s *testPutCountStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Attach the recorded refs to the stored bytes.
	data, found, err := s.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return &block.StoredBlock{Data: data, Refs: s.refs[ref.MarshalString()], RefsKnown: true}, nil
}

// testUploadBlockStore counts upload waits.
type testUploadBlockStore struct {
	*testBlockStore
	waited int
}

func (s *testUploadBlockStore) WaitUploaded(context.Context) error {
	s.waited++
	return nil
}
