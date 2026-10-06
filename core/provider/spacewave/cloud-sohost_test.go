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
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
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
	// Load rejoin fixtures for one owner.
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

	// The inline response advances a real trusted checkpoint rather than replacing a fork.
	previousConfig := state.GetConfig().CloneVT()
	change, err := sobject.BuildSOConfigChange(soID, previousConfig, previousConfig, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_INVITE, ownerPriv, nil)
	if err != nil {
		t.Fatal(err)
	}
	state.Config, err = sobject.VerifyConfigChange(soID, previousConfig, change)
	if err != nil {
		t.Fatal(err)
	}
	chainResp.ConfigChanges = append(chainResp.ConfigChanges, change)
	stateData := mustMarshalVT(t, &api.SOStateMessage{
		Seqno:       1,
		ConfigChain: chainResp,
		Content: &api.SOStateMessage_Snapshot{
			Snapshot: state,
		},
	})

	// Serve the state and fail any separate chain request.
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

	// Pulling uses the inline chain and accepts the state.
	host := &cloudSOHost{
		le:                  logrus.New().WithField("test", t.Name()),
		client:              NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, ownerPriv, ownerPID.String()),
		soID:                soID,
		soHost:              sobject.NewSOHost(nil, nil, nil, soID),
		privKey:             ownerPriv,
		peerID:              ownerPID,
		stateCtr:            ccontainer.NewCContainer[*sobject.SOState](nil),
		lastConfigChainHash: previousConfig.GetConfigChainHash(),
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

	// An operation written under the previous config resolves it from the
	// verified history.
	snap := host.newSnapshot(host.stateCtr.GetValue())
	cfg, err := snap.GetConfigByHash(context.Background(), previousConfig.GetConfigChainHash())
	if err != nil {
		t.Fatalf("previous config by hash: %v", err)
	}
	if !bytes.Equal(cfg.GetConfigChainHash(), previousConfig.GetConfigChainHash()) {
		t.Fatalf("previous config hash = %x, want %x", cfg.GetConfigChainHash(), previousConfig.GetConfigChainHash())
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

// TestApplyChangeLogEntryCheckpointCoversOps checks that change log entries
// add operations and that a checkpoint entry drops the operations it covers,
// once even when replayed.
func TestApplyChangeLogEntryCheckpointCoversOps(t *testing.T) {
	// The owner writes two operations to its copy of genesis.
	state, priv := newTestGenesisState(t)
	replica := state.CloneVT()
	ops := []*sobject.SOOperation{writeTestOperation(t, state, priv), writeTestOperation(t, state, priv)}
	opsData, err := (&api.PostOpsRequest{Operations: ops}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	// The replica applies the operations from the change log.
	if err := applyChangeLogEntry(testSharedObjectID, replica, &api.SOStateDeltaEntry{ChangeType: "ops", ChangeData: opsData}); err != nil {
		t.Fatal(err)
	}
	if len(replica.GetOps()) != 2 {
		t.Fatalf("replica holds %d operations; want 2", len(replica.GetOps()))
	}

	// The checkpoint covering both drops them, and replaying it or a delayed
	// echo of the operations changes nothing.
	checkpoint, err := state.BuildNextCheckpoint(testSharedObjectID, priv, []byte("state"))
	if err != nil {
		t.Fatal(err)
	}
	checkpointData, err := (&api.PostCheckpointRequest{Checkpoint: checkpoint}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []*api.SOStateDeltaEntry{
		{ChangeType: "checkpoint", ChangeData: checkpointData},
		{ChangeType: "checkpoint", ChangeData: checkpointData},
		{ChangeType: "ops", ChangeData: opsData},
	} {
		if err := applyChangeLogEntry(testSharedObjectID, replica, entry); err != nil {
			t.Fatal(err)
		}
		if len(replica.GetOps()) != 0 || !replica.GetCheckpoint().EqualVT(checkpoint) {
			t.Fatalf("%s entry restored covered operations", entry.GetChangeType())
		}
	}
}

// TestApplyChangeLogEntrySequence checks that a sequence entry adds the
// sequencer's positions once, so the replica places the operation as the
// sequencer did.
func TestApplyChangeLogEntrySequence(t *testing.T) {
	// The owner appoints sequencer S.
	state, priv := newTestGenesisState(t)
	sequencer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	state.Config.Sequencer = &sobject.SOSequencer{PeerId: sequencer.GetPeerID().String()}
	replica := state.CloneVT()

	// The owner writes an operation, and S places it.
	op := writeTestOperation(t, state, priv)
	sequencerKey, err := sequencer.GetPrivKey(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	records, err := state.SequenceOperations(testSharedObjectID, sequencerKey)
	if err != nil || len(records) != 1 {
		t.Fatalf("sequencer placed %d operations, err %v", len(records), err)
	}
	data, err := (&api.SOSequenceBatch{Sequence: records}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	// The replica holds the operation and the position, once when replayed.
	if _, err := replica.AddOperation(testSharedObjectID, op); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := applyChangeLogEntry(testSharedObjectID, replica, &api.SOStateDeltaEntry{ChangeType: "sequence", ChangeData: data}); err != nil {
			t.Fatal(err)
		}
	}
	if len(replica.GetSequence()) != 1 || !replica.GetSequence()[0].EqualVT(records[0]) {
		t.Fatalf("replica holds %d positions; want the sequencer's one", len(replica.GetSequence()))
	}
	set, err := replica.OperationSet(testSharedObjectID)
	if err != nil {
		t.Fatal(err)
	}
	if got := set.StablePoint(nil); len(got) != 1 || !bytes.Equal(got[0], op.Hash()) {
		t.Fatal("replica did not place the sequenced operation")
	}
}

// TestVerifyPulledStateIgnoresChangeLogSeqno checks that the change log
// sequence number does not take part in rollback checks of pulled state.
func TestVerifyPulledStateIgnoresChangeLogSeqno(t *testing.T) {
	state, _ := newTestGenesisState(t)
	h := &cloudSOHost{
		soID:     testSharedObjectID,
		stateCtr: ccontainer.NewCContainer(state),
		le:       logrus.New().WithField("test", t.Name()),
	}
	h.lastSeqno = 10
	if err := h.verifyPulledState(state.CloneVT()); err != nil {
		t.Fatalf("verifyPulledState should ignore changelog seqno for rollback checks: %v", err)
	}
}

// TestVerifyChangeLogSeqnoUsesSnapshotCounter rejects a change log rollback.
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

// testStepFactorySet returns the block transforms of shared object keys.
func testStepFactorySet() *block_transform.StepFactorySet {
	sfs := block_transform.NewStepFactorySet()
	sfs.AddStepFactory(transform_blockenc.NewStepFactory())
	return sfs
}

// newTestGenesisState returns the genesis state of a new owner and its key.
func newTestGenesisState(t *testing.T) (*sobject.SOState, crypto.PrivKey) {
	// Build a genesis for a new owner.
	t.Helper()
	owner, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := owner.GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	state, _, err := sobject.BuildGenesisSOState(logrus.NewEntry(logrus.New()), testStepFactorySet(), testSharedObjectID, priv, nil)
	if err != nil {
		t.Fatal(err)
	}
	return state, priv
}

// writeTestOperation signs an operation as priv at the head of its chain in
// state, adds it and returns it.
func writeTestOperation(t *testing.T, state *sobject.SOState, priv crypto.PrivKey) *sobject.SOOperation {
	// Link the operation to the author's chain.
	t.Helper()
	peerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	set, err := state.OperationSet(testSharedObjectID)
	if err != nil {
		t.Fatal(err)
	}
	link := state.NextOperationLink(set, peerID.String())

	// Build and add the operation.
	op, err := sobject.BuildSOOperation(testSharedObjectID, priv, []byte("op"), link, sobject.NewSOOperationLocalID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddOperation(testSharedObjectID, op); err != nil {
		t.Fatal(err)
	}
	return op
}
