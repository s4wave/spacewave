package provider_spacewave

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

const testSharedObjectID = "test-shared-object"

func TestCoalescedTriggerRoutineQueuesSinglePendingRun(t *testing.T) {
	// Initialize a coalescing trigger routine.
	started := make(chan struct{}, 3)
	release := make(chan struct{})
	routine := newCoalescedTriggerRoutine(
		logrus.New().WithField("test", t.Name()),
		"test-trigger",
		func(ctx context.Context) {
			started <- struct{}{}
			select {
			case <-ctx.Done():
			case <-release:
			}
		},
	)

	// Start the routine and observe its first run.
	ctx := t.Context()
	routine.SetContext(ctx)
	defer routine.ClearContext()

	routine.Trigger()
	waitCoalescedTriggerRun(t, started)

	// Queue duplicate triggers while the first run is active.
	routine.Trigger()
	routine.Trigger()
	release <- struct{}{}
	waitCoalescedTriggerRun(t, started)

	// Confirm duplicates do not create an extra pending run.
	release <- struct{}{}
	select {
	case <-started:
		t.Fatal("expected duplicate triggers while running to coalesce into one pending run")
	case <-time.After(100 * time.Millisecond):
	}
}

func waitCoalescedTriggerRun(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for coalesced trigger routine")
	}
}

func TestAsyncCallbackJobsStartsOneOwnedJobPerTrigger(t *testing.T) {
	// Initialize callback jobs and their lifecycle context.
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	jobs := newAsyncCallbackJobs(func(ctx context.Context) {
		started <- struct{}{}
		select {
		case <-ctx.Done():
		case <-release:
		}
	})

	jobs.SetContext(t.Context())
	defer jobs.ClearContext()

	// Trigger jobs and observe each admitted run.
	jobs.Trigger()
	waitAsyncCallbackJob(t, started)
	jobs.Trigger()
	waitAsyncCallbackJob(t, started)

	close(release)
	waitAsyncCallbackJobsEmpty(t, jobs)
}

func TestAsyncCallbackJobsClearContextWaitsForOwnedJobs(t *testing.T) {
	// Initialize a job blocked on cancellation.
	started := make(chan struct{})
	canceled := make(chan struct{})
	allowReturn := make(chan struct{})
	clearReturned := make(chan struct{})
	jobs := newAsyncCallbackJobs(func(ctx context.Context) {
		started <- struct{}{}
		<-ctx.Done()
		close(canceled)
		<-allowReturn
	})

	// Cancel the job context and start cleanup.
	ctx, cancel := context.WithCancel(context.Background())
	jobs.SetContext(ctx)
	jobs.Trigger()
	waitAsyncCallbackJob(t, started)

	cancel()
	go func() {
		jobs.ClearContext()
		close(clearReturned)
	}()

	// Verify cleanup waits for the callback to return.
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async callback job cancellation")
	}
	select {
	case <-clearReturned:
		t.Fatal("ClearContext returned before the owned job exited")
	case <-time.After(100 * time.Millisecond):
	}
	close(allowReturn)
	select {
	case <-clearReturned:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ClearContext to return")
	}
}

func TestAsyncCallbackJobsIgnoresTriggerWithoutContext(t *testing.T) {
	started := make(chan struct{}, 1)
	jobs := newAsyncCallbackJobs(func(context.Context) {
		started <- struct{}{}
	})

	jobs.Trigger()
	if got := jobs.Pending(); got != 0 {
		t.Fatalf("trigger without context queued %d jobs", got)
	}
	select {
	case <-started:
		t.Fatal("trigger without context started a job")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestAsyncCallbackJobsDoesNotReplayTriggerAfterCancel(t *testing.T) {
	started := make(chan struct{}, 1)
	jobs := newAsyncCallbackJobs(func(context.Context) {
		started <- struct{}{}
	})

	ctx, cancel := context.WithCancel(context.Background())
	jobs.SetContext(ctx)
	cancel()
	jobs.Trigger()
	jobs.SetContext(t.Context())

	if got := jobs.Pending(); got != 0 {
		t.Fatalf("trigger after canceled context queued %d jobs", got)
	}
	select {
	case <-started:
		t.Fatal("trigger after canceled context replayed on the next context")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestCloudSOHostRefreshesBlockManifestBeforeInlineStateWhenNonceAdvances(t *testing.T) {
	host := &cloudSOHost{
		le:       logrus.New().WithField("test", t.Name()),
		soID:     testSharedObjectID,
		stateCtr: ccontainer.NewCContainer[*sobject.SOState](nil),
	}
	var refreshed bool
	host.refreshBlockManifest = func(context.Context) error {
		if host.stateCtr.GetValue() != nil {
			t.Fatal("inline SO state was published before block manifest refresh")
		}
		refreshed = true
		return nil
	}

	host.handleSONotify(&api.SONotifyEventPayload{
		Seqno:           1,
		ChangeType:      "op",
		BlockStoreNonce: 1,
		StateMessage: &api.SOStateMessage{
			Seqno: 1,
			Content: &api.SOStateMessage_Snapshot{
				Snapshot: &sobject.SOState{},
			},
		},
	})

	if !refreshed {
		t.Fatal("block manifest refresh was not called for an advanced block-store nonce")
	}
	if host.stateCtr.GetValue() == nil {
		t.Fatal("inline SO state was not published after refresh")
	}
}

func TestCloudSOHostSkipsBlockManifestRefreshWithoutAdvancedNonce(t *testing.T) {
	host := &cloudSOHost{
		le:       logrus.New().WithField("test", t.Name()),
		soID:     testSharedObjectID,
		stateCtr: ccontainer.NewCContainer[*sobject.SOState](nil),
	}
	var localManifestSeq uint64
	host.blockManifestSequence = func(context.Context) (uint64, error) {
		return localManifestSeq, nil
	}
	var refreshCalls int
	host.refreshBlockManifest = func(context.Context) error {
		refreshCalls++
		return nil
	}
	notify := func(seqno, blockStoreNonce uint64) {
		t.Helper()
		host.handleSONotify(&api.SONotifyEventPayload{
			Seqno:           seqno,
			ChangeType:      "op",
			BlockStoreNonce: blockStoreNonce,
			StateMessage: &api.SOStateMessage{
				Seqno: seqno,
				Content: &api.SOStateMessage_Snapshot{
					Snapshot: &sobject.SOState{},
				},
			},
		})
	}

	notify(1, 0)
	if refreshCalls != 0 {
		t.Fatalf("refreshBlockManifest called %d time(s) without a block-store nonce", refreshCalls)
	}
	if host.stateCtr.GetValue() == nil {
		t.Fatal("inline SO state without a block-store nonce was not applied")
	}

	notify(2, 7)
	if refreshCalls != 1 {
		t.Fatalf("refreshBlockManifest calls after advanced nonce = %d, want 1", refreshCalls)
	}
	localManifestSeq = 7
	notify(3, 7)
	if refreshCalls != 1 {
		t.Fatalf("refreshBlockManifest calls after unchanged nonce = %d, want 1", refreshCalls)
	}
	notify(4, 8)
	if refreshCalls != 2 {
		t.Fatalf("refreshBlockManifest calls after second advanced nonce = %d, want 2", refreshCalls)
	}
}

func TestCloudSOHostTriggersPullWhenAdvancedBlockManifestRefreshFails(t *testing.T) {
	host := &cloudSOHost{
		le:          logrus.New().WithField("test", t.Name()),
		soID:        testSharedObjectID,
		stateCtr:    ccontainer.NewCContainer[*sobject.SOState](nil),
		pullRoutine: newCoalescedTriggerRoutine(nil, t.Name(), nil),
	}
	refreshErr := errors.New("refresh failed")
	host.refreshBlockManifest = func(context.Context) error {
		return refreshErr
	}

	host.handleSONotify(&api.SONotifyEventPayload{
		Seqno:           1,
		ChangeType:      "op",
		BlockStoreNonce: 1,
		StateMessage: &api.SOStateMessage{
			Seqno: 1,
			Content: &api.SOStateMessage_Snapshot{
				Snapshot: &sobject.SOState{},
			},
		},
	})

	if host.stateCtr.GetValue() != nil {
		t.Fatal("inline SO state was published after block manifest refresh failed")
	}
	if !host.pullRoutine.Pending() {
		t.Fatal("pull recovery was not triggered after block manifest refresh failed")
	}
}

func TestCloudSOHostUsesInlineConfigChainWhenPulledStateHashChanges(t *testing.T) {
	const accountID = "acct-inline-chain"
	soID := testSharedObjectID
	entityPriv, _ := generateTestKeypair(t)
	ownerPriv, ownerPID := generateTestKeypair(t)
	state, chainResp, _, _ := buildRejoinTestFixtures(
		t,
		soID,
		accountID,
		ownerPriv,
		ownerPID,
		entityPriv,
		1,
	)
	stateData := mustMarshalVT(t, &api.SOStateMessage{
		Seqno:       1,
		ConfigChain: chainResp,
		Content: &api.SOStateMessage_Snapshot{
			Snapshot: state,
		},
	})

	var configChainRequests int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/state":
			_, _ = w.Write(stateData)
		case "/api/sobject/" + soID + "/config-chain":
			configChainRequests++
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	host := &cloudSOHost{
		le:                  logrus.New().WithField("test", t.Name()),
		client:              NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, ownerPriv, ownerPID.String()),
		soID:                soID,
		privKey:             ownerPriv,
		peerID:              ownerPID,
		stateCtr:            ccontainer.NewCContainer[*sobject.SOState](nil),
		lastConfigChainHash: []byte("stale-config-hash"),
	}
	if err := host.pullState(context.Background(), SeedReasonColdSeed); err != nil {
		t.Fatalf("pull state: %v", err)
	}
	if configChainRequests != 0 {
		t.Fatalf("pulled state made %d separate /config-chain request(s) despite inline config_chain", configChainRequests)
	}
	if host.stateCtr.GetValue() == nil {
		t.Fatal("pulled inline-config-chain state was not accepted")
	}
	if !bytes.Equal(host.lastConfigChainHash, state.GetConfig().GetConfigChainHash()) {
		t.Fatalf("verified config chain hash = %x, want %x", host.lastConfigChainHash, state.GetConfig().GetConfigChainHash())
	}
}

func waitAsyncCallbackJob(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async callback job")
	}
}

func waitAsyncCallbackJobsEmpty(t *testing.T, jobs *asyncCallbackJobs) {
	t.Helper()
	deadline := time.After(time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if jobs.Pending() == 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for async callback jobs to drain: %d", jobs.Pending())
		case <-tick.C:
		}
	}
}

func TestApplyChangeLogEntryRootPrunesAcceptedAndRejectedOps(t *testing.T) {
	validator, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatalf("validator peer: %v", err)
	}
	writer1, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatalf("writer1 peer: %v", err)
	}
	writer2, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatalf("writer2 peer: %v", err)
	}

	validatorPriv, err := validator.GetPrivKey(context.Background())
	if err != nil {
		t.Fatalf("validator privkey: %v", err)
	}
	writer1Priv, err := writer1.GetPrivKey(context.Background())
	if err != nil {
		t.Fatalf("writer1 privkey: %v", err)
	}
	writer2Priv, err := writer2.GetPrivKey(context.Background())
	if err != nil {
		t.Fatalf("writer2 privkey: %v", err)
	}

	op1 := buildTestSOOperation(t, writer1Priv, 1)
	op2 := buildTestSOOperation(t, writer1Priv, 2)
	op3 := buildTestSOOperation(t, writer2Priv, 1)
	rejection := buildTestSOOperationRejection(t, validatorPriv, writer1.GetPeerID(), 2, op2)
	state := &sobject.SOState{
		Config: buildTestSharedObjectConfig(validator, writer1, writer2),
		Root:   buildTestSORoot(t, validatorPriv, 1, nil),
		Ops:    []*sobject.SOOperation{op1, op2, op3},
		OpRejections: []*sobject.SOPeerOpRejections{{
			PeerId:     writer1.GetPeerID().String(),
			Rejections: []*sobject.SOOperationRejection{rejection},
		}},
	}
	root := buildTestSORoot(t, validatorPriv, 2, []*sobject.SOAccountNonce{{
		PeerId: writer1.GetPeerID().String(),
		Nonce:  1,
	}})
	rootData, err := (&api.PostRootRequest{Root: root}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal post root request: %v", err)
	}

	err = applyChangeLogEntry(testSharedObjectID, state, &api.SOStateDeltaEntry{
		ChangeType: "root",
		ChangeData: rootData,
	})
	if err != nil {
		t.Fatalf("applyChangeLogEntry(root): %v", err)
	}
	if len(state.GetOps()) != 1 {
		t.Fatalf("expected 1 pending op after prune, got %d", len(state.GetOps()))
	}

	inner, err := state.GetOps()[0].UnmarshalInner()
	if err != nil {
		t.Fatalf("unmarshal remaining op: %v", err)
	}
	if inner.GetPeerId() != writer2.GetPeerID().String() || inner.GetNonce() != 1 {
		t.Fatalf("unexpected remaining op: peer=%s nonce=%d", inner.GetPeerId(), inner.GetNonce())
	}
}

func TestApplyChangeLogEntryRootIsIdempotentForMatchingRoot(t *testing.T) {
	validator, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatalf("validator peer: %v", err)
	}
	validatorPriv, err := validator.GetPrivKey(context.Background())
	if err != nil {
		t.Fatalf("validator privkey: %v", err)
	}
	root := buildTestSORoot(t, validatorPriv, 6, nil)
	reqData, err := (&api.PostRootRequest{Root: root}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal post root request: %v", err)
	}

	state := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			ConsensusMode: sobject.SOConsensusMode_SO_CONSENSUS_MODE_SINGLE_VALIDATOR,
			Participants: []*sobject.SOParticipantConfig{{
				PeerId: validator.GetPeerID().String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_VALIDATOR,
			}},
		},
		Root: root.CloneVT(),
	}
	if err := applyChangeLogEntry(testSharedObjectID, state, &api.SOStateDeltaEntry{
		ChangeType: "root",
		ChangeData: reqData,
	}); err != nil {
		t.Fatalf("applyChangeLogEntry(root): %v", err)
	}
	if !state.GetRoot().EqualVT(root) {
		t.Fatal("expected matching root replay to leave cached root unchanged")
	}
}

func TestVerifyPulledStateIgnoresChangeLogSeqno(t *testing.T) {
	validator, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatalf("validator peer: %v", err)
	}
	validatorPriv, err := validator.GetPrivKey(context.Background())
	if err != nil {
		t.Fatalf("validator privkey: %v", err)
	}
	cachedRoot := buildTestSORoot(t, validatorPriv, 6, nil)
	h := &cloudSOHost{
		soID:     testSharedObjectID,
		stateCtr: ccontainer.NewCContainer[*sobject.SOState](&sobject.SOState{Root: cachedRoot}),
		le:       logrus.New().WithField("test", t.Name()),
	}
	h.lastSeqno = 10

	next := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			ConsensusMode: sobject.SOConsensusMode_SO_CONSENSUS_MODE_SINGLE_VALIDATOR,
			Participants: []*sobject.SOParticipantConfig{{
				PeerId: validator.GetPeerID().String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_VALIDATOR,
			}},
		},
		Root: cachedRoot.CloneVT(),
	}
	if err := h.verifyPulledState(next); err != nil {
		t.Fatalf("verifyPulledState should ignore changelog seqno for rollback checks: %v", err)
	}
}

func TestVerifyChangeLogSeqnoUsesSnapshotCounter(t *testing.T) {
	h := &cloudSOHost{
		stateCtr: ccontainer.NewCContainer[*sobject.SOState](nil),
		le:       logrus.New().WithField("test", t.Name()),
	}
	h.lastSeqno = 10

	if err := h.verifyChangeLogSeqno(6); err == nil {
		t.Fatal("expected changelog rollback error")
	}
	if err := h.verifyChangeLogSeqno(11); err != nil {
		t.Fatalf("expected changelog seqno 11 to be accepted: %v", err)
	}
}

func buildTestSharedObjectConfig(
	validator peer.Peer,
	writer1 peer.Peer,
	writer2 peer.Peer,
) *sobject.SharedObjectConfig {
	return &sobject.SharedObjectConfig{
		ConsensusMode: sobject.SOConsensusMode_SO_CONSENSUS_MODE_SINGLE_VALIDATOR,
		Participants: []*sobject.SOParticipantConfig{
			{
				PeerId: validator.GetPeerID().String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_VALIDATOR,
			},
			{
				PeerId: writer1.GetPeerID().String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_WRITER,
			},
			{
				PeerId: writer2.GetPeerID().String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_WRITER,
			},
		},
	}
}

func buildTestSOOperation(
	t *testing.T,
	privKey crypto.PrivKey,
	nonce uint64,
) *sobject.SOOperation {
	t.Helper()

	op, err := sobject.BuildSOOperation(
		testSharedObjectID,
		privKey,
		[]byte("op"),
		nonce,
		sobject.NewSOOperationLocalID(),
	)
	if err != nil {
		t.Fatalf("build op: %v", err)
	}
	return op
}

func buildTestSOOperationRejection(
	t *testing.T,
	validatorPrivKey crypto.PrivKey,
	submitterPeerID peer.ID,
	nonce uint64,
	op *sobject.SOOperation,
) *sobject.SOOperationRejection {
	t.Helper()

	inner, err := op.UnmarshalInner()
	if err != nil {
		t.Fatalf("unmarshal op inner: %v", err)
	}
	rejection, err := sobject.BuildSOOperationRejection(
		validatorPrivKey,
		testSharedObjectID,
		submitterPeerID,
		nonce,
		inner.GetLocalId(),
		nil,
	)
	if err != nil {
		t.Fatalf("build rejection: %v", err)
	}
	return rejection
}

func buildTestSORoot(
	t *testing.T,
	validatorPrivKey crypto.PrivKey,
	seqno uint64,
	accountNonces []*sobject.SOAccountNonce,
) *sobject.SORoot {
	t.Helper()

	innerData, err := (&sobject.SORootInner{
		Seqno:     seqno,
		StateData: []byte("state"),
	}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal root inner: %v", err)
	}
	root := &sobject.SORoot{
		Inner:         innerData,
		InnerSeqno:    seqno,
		AccountNonces: accountNonces,
	}
	if err := root.SignInnerData(
		validatorPrivKey,
		testSharedObjectID,
		seqno,
		hash.RecommendedHashType,
	); err != nil {
		t.Fatalf("sign root: %v", err)
	}
	return root
}
