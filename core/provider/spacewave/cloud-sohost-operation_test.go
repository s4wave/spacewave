package provider_spacewave

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// TestCloudPublicationSurvivesRestart exercises signed local acceptance, failed
// persistence, cloud loss, coalescing, and blocks-before-root acknowledgment.
func TestCloudPublicationSurvivesRestart(t *testing.T) {
	var requests, roots, uploads atomic.Int32
	var unavailable atomic.Bool
	unavailable.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if unavailable.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/sync/push") {
			uploads.Add(1)
		} else if strings.HasSuffix(r.URL.Path, "/root") {
			if uploads.Load() == 0 {
				t.Error("root reached the cloud before its blocks")
			}
			roots.Add(1)
		} else {
			t.Errorf("unexpected publication route: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	key, peerID := generateTestKeypair(t)
	client := NewSessionClient(server.Client(), server.URL, DefaultSigningEnvPrefix, key, peerID.String())
	client.executeWriteTicketAudience = func(ctx context.Context, resourceID string, audience writeTicketAudience, submit func(string) error) error {
		return submit("test-ticket")
	}
	syncer := newDirtySyncExecuteTestController(t, client, nil)
	account := &ProviderAccount{objStore: newSyncTestKvStore()}
	config := &sobject.SharedObjectConfig{
		ConfigChainHash: []byte("verified history"),
		ConsensusMode:   sobject.SOConsensusMode_SO_CONSENSUS_MODE_SINGLE_VALIDATOR,
		Participants:    []*sobject.SOParticipantConfig{{PeerId: peerID.String(), Role: sobject.SOParticipantRole_SOParticipantRole_OWNER}},
	}
	initial := &sobject.SOState{Config: config, Root: buildTestSORoot(t, key, 1, nil)}
	persistErr := errors.New("metadata unavailable")
	failPersist := true
	persist := func(ctx context.Context, cache *api.VerifiedSOStateCache) error {
		if failPersist {
			return persistErr
		}
		return account.writeVerifiedSOStateCache(ctx, testSharedObjectID, cache)
	}
	host := &cloudSOHost{
		le: logrus.New().WithField("test", t.Name()), client: client,
		soID: testSharedObjectID, peerID: peerID, stateCtr: ccontainer.NewCContainer(initial),
		verifiedConfig: config, lastConfigChainHash: config.ConfigChainHash,
		cloudState: initial.CloneVT(), persistVerifiedStateCache: persist, syncer: syncer,
	}
	queue := func() error {
		return host.QueueOperation(t.Context(), peerID, func(nonce uint64) (*sobject.SOOperation, error) {
			return sobject.BuildSOOperation(testSharedObjectID, key, []byte("operation"), nonce, sobject.NewSOOperationLocalID())
		})
	}
	if err := queue(); !errors.Is(err, persistErr) || len(host.stateCtr.GetValue().GetOps()) != 0 {
		t.Fatalf("failed persistence acknowledged local work: %v", err)
	}
	failPersist = false
	for range 2 {
		if err := queue(); err != nil {
			t.Fatal(err)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("ordinary local acceptance contacted the cloud")
	}
	first := host.pending.GetFirstPendingUnixMilli()

	// Validator acceptance coalesces both operations into one signed checkpoint.
	state := host.stateCtr.GetValue().CloneVT()
	root := buildTestSORoot(t, key, 2, []*sobject.SOAccountNonce{{PeerId: peerID.String(), Nonce: 2}})
	if err := state.UpdateRootState(testSharedObjectID, root, peerID.String(), nil, state.Ops); err != nil {
		t.Fatal(err)
	}
	if err := host.acceptLocalState(t.Context(), state); err != nil {
		t.Fatal(err)
	}
	if len(host.pending.Operations) != 0 || host.pending.GetFirstPendingUnixMilli() != first {
		t.Fatal("coalescing retained resolved operations or extended the deadline")
	}

	// Restart from serialized storage, retaining the cloud delta base and timer.
	cache, err := account.loadVerifiedSOStateCache(t.Context(), testSharedObjectID)
	if err != nil {
		t.Fatal(err)
	}
	reopened := &cloudSOHost{
		le: host.le, client: client, soID: testSharedObjectID, peerID: peerID,
		stateCtr: ccontainer.NewCContainer[*sobject.SOState](nil), persistVerifiedStateCache: persist, syncer: syncer,
	}
	reopened.hydrateVerifiedStateCache(cache)
	syncer.setPublication(host, nil)
	syncer.setPublication(reopened, reopened.pending)
	if reopened.stateCtr.GetValue() == nil || reopened.pending.GetFirstPendingUnixMilli() != first {
		t.Fatal("restart lost accepted state or first-pending deadline")
	}
	if err := reopened.verifyPulledState(initial); err != nil {
		t.Fatal(err)
	}
	if err := reopened.acceptCloudSnapshot(t.Context(), initial, 1); err != nil {
		t.Fatal(err)
	}
	if reopened.stateCtr.GetValue().GetRoot().GetInnerSeqno() != 2 {
		t.Fatal("cloud lag rolled back accepted local state")
	}

	if err := syncer.FlushNowUnordered(t.Context()); err == nil {
		t.Fatal("cloud outage was acknowledged")
	}
	if reopened.pending == nil || roots.Load() != 0 {
		t.Fatal("failed upload lost publication or exposed its root")
	}
	unavailable.Store(false)
	if err := syncer.FlushNowUnordered(t.Context()); err != nil {
		t.Fatal(err)
	}
	if roots.Load() != 1 || reopened.pending != nil {
		t.Fatal("checkpoint did not acknowledge exactly one coalesced root")
	}
	cache, err = account.loadVerifiedSOStateCache(t.Context(), testSharedObjectID)
	if err != nil || cache.GetPendingPublication() != nil {
		t.Fatalf("acknowledgment was not durable: %v", err)
	}
}

// TestCloudPublicationMissingBlockCannotPublish keeps root visibility behind the
// complete dirty-block fence even when all state metadata is already durable.
func TestCloudPublicationMissingBlockCannotPublish(t *testing.T) {
	syncer := newDirtySyncExecuteTestController(t, nil, nil)
	ref, err := block.BuildBlockRef([]byte("unavailable dependency"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncer.MarkDirty(t.Context(), ref.GetHash(), 22); err != nil {
		t.Fatal(err)
	}
	host := &cloudSOHost{pending: &api.PendingSOPublication{FirstPendingUnixMilli: 1}}
	syncer.setPublication(host, host.pending)
	if err := syncer.FlushNowUnordered(t.Context()); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("missing dependency fence: %v", err)
	}
}

// TestCloudPublicationConcurrentAcceptance preserves work accepted during a
// checkpoint and fences retained work when the writer loses publication rights.
func TestCloudPublicationConcurrentAcceptance(t *testing.T) {
	started := make(chan struct{})
	resume := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/ops") {
			t.Errorf("unexpected route: %s", r.URL.Path)
		}
		if calls.Add(1) == 1 {
			close(started)
			select {
			case <-resume:
			case <-r.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/protobuf")
	}))
	t.Cleanup(server.Close)
	key, peerID := generateTestKeypair(t)
	client := NewSessionClient(server.Client(), server.URL, DefaultSigningEnvPrefix, key, peerID.String())
	client.executeWriteTicketAudience = func(ctx context.Context, resourceID string, audience writeTicketAudience, submit func(string) error) error {
		return submit("test-ticket")
	}
	account := &ProviderAccount{objStore: newSyncTestKvStore()}
	config := &sobject.SharedObjectConfig{
		ConfigChainHash: []byte("verified history"),
		ConsensusMode:   sobject.SOConsensusMode_SO_CONSENSUS_MODE_SINGLE_VALIDATOR,
		Participants:    []*sobject.SOParticipantConfig{{PeerId: peerID.String(), Role: sobject.SOParticipantRole_SOParticipantRole_OWNER}},
	}
	host := &cloudSOHost{
		le: logrus.New().WithField("test", t.Name()), client: client,
		soID: testSharedObjectID, peerID: peerID,
		stateCtr:       ccontainer.NewCContainer(&sobject.SOState{Config: config}),
		verifiedConfig: config, lastConfigChainHash: config.ConfigChainHash,
		persistVerifiedStateCache: func(ctx context.Context, cache *api.VerifiedSOStateCache) error {
			return account.writeVerifiedSOStateCache(ctx, testSharedObjectID, cache)
		},
	}
	queue := func() {
		t.Helper()
		if err := host.QueueOperation(t.Context(), peerID, func(nonce uint64) (*sobject.SOOperation, error) {
			return sobject.BuildSOOperation(testSharedObjectID, key, []byte("operation"), nonce, sobject.NewSOOperationLocalID())
		}); err != nil {
			t.Fatal(err)
		}
	}
	queue()
	sent := host.pendingPublication()
	done := make(chan error, 1)
	go func() { done <- host.publishCheckpoint(t.Context(), sent) }()
	select {
	case <-started:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	queue()
	close(resume)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	cache, err := account.loadVerifiedSOStateCache(t.Context(), testSharedObjectID)
	if err != nil {
		t.Fatal(err)
	}
	pending := cache.GetPendingPublication()
	if len(pending.GetOperations()) != 1 || pending.GetFirstPendingUnixMilli() != sent.GetFirstPendingUnixMilli() {
		t.Fatal("acknowledgment lost concurrent work or extended its deadline")
	}
	inner, err := pending.Operations[0].UnmarshalInner()
	if err != nil || inner.GetNonce() != 2 {
		t.Fatalf("wrong pending operation: %v", err)
	}
	revoked := config.CloneVT()
	revoked.Participants[0].Role = sobject.SOParticipantRole_SOParticipantRole_READER
	host.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { host.verifiedConfig = revoked })
	if err := host.publishCheckpoint(t.Context(), host.pendingPublication()); err == nil {
		t.Fatal("revoked writer published retained work")
	}
	if calls.Load() != 1 || len(host.pendingPublication().GetOperations()) != 1 {
		t.Fatal("revocation contacted the cloud or discarded retained work")
	}
}

// TestCloudPublicationRecoversRejectedNonce preserves unsubmitted local work
// when a restored cache reused a nonce consumed by a different rejected write.
func TestCloudPublicationRecoversRejectedNonce(t *testing.T) {
	key, peerID := generateTestKeypair(t)
	config := &sobject.SharedObjectConfig{
		ConfigChainHash: []byte("verified history"),
		ConsensusMode:   sobject.SOConsensusMode_SO_CONSENSUS_MODE_SINGLE_VALIDATOR,
		Participants:    []*sobject.SOParticipantConfig{{PeerId: peerID.String(), Role: sobject.SOParticipantRole_SOParticipantRole_OWNER}},
	}
	rejectedOp := buildTestSOOperation(t, key, 2)
	pendingOp := buildTestSOOperation(t, key, 2)
	laterOp := buildTestSOOperation(t, key, 3)
	original, err := pendingOp.UnmarshalInner()
	if err != nil {
		t.Fatal(err)
	}
	cloud := &sobject.SOState{
		Config:              config,
		Root:                buildTestSORoot(t, key, 3, []*sobject.SOAccountNonce{{PeerId: peerID.String(), Nonce: 1}}),
		QueuedAccountNonces: []*sobject.SOAccountNonce{{PeerId: peerID.String(), Nonce: 2}},
		OpRejections:        []*sobject.SOPeerOpRejections{{PeerId: peerID.String(), Rejections: []*sobject.SOOperationRejection{buildTestSOOperationRejection(t, key, peerID, 2, rejectedOp)}}},
	}
	if got := cloud.GetNextAccountNonce(peerID.String()); got != 3 {
		t.Fatalf("next nonce = %d", got)
	}
	previous := cloud.CloneVT()
	previous.OpRejections = nil
	previous.Ops = []*sobject.SOOperation{pendingOp, laterOp}
	var persisted *api.VerifiedSOStateCache
	host := &cloudSOHost{
		le: logrus.New().WithField("test", t.Name()), soID: testSharedObjectID,
		privKey: key, peerID: peerID, stateCtr: ccontainer.NewCContainer(previous),
		verifiedConfig: config, lastConfigChainHash: config.ConfigChainHash,
		cloudState: cloud.CloneVT(), peerState: previous,
		pending: &api.PendingSOPublication{FirstPendingUnixMilli: 17, Operations: []*sobject.SOOperation{pendingOp, laterOp}},
		persistVerifiedStateCache: func(_ context.Context, cache *api.VerifiedSOStateCache) error {
			persisted = cache.CloneVT()
			return nil
		},
	}
	var submissions atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/ops") {
			if submissions.Add(1) > 1 {
				data, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				var batch api.PostOpsRequest
				if err := batch.UnmarshalVT(data); err != nil {
					t.Error(err)
					return
				}
				if len(batch.Operations) != 2 {
					t.Errorf("batch operation count = %d", len(batch.Operations))
					return
				}
				for i, operation := range batch.Operations {
					inner, err := operation.UnmarshalInner()
					if err != nil {
						t.Error(err)
						return
					}
					if inner.Nonce != uint64(i+3) {
						t.Errorf("batch operation %d nonce = %d", i, inner.Nonce)
					}
				}
				w.Header().Set("Content-Type", "application/protobuf")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":"nonce_too_low","message":"Operation nonce conflicts with accepted work"}`))
			return
		}
		data, err := (&api.SOStateMessage{Content: &api.SOStateMessage_Snapshot{Snapshot: cloud}, Seqno: 4}).MarshalVT()
		if err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/protobuf")
		_, _ = w.Write(data)
	}))
	t.Cleanup(server.Close)
	host.client = NewSessionClient(server.Client(), server.URL, DefaultSigningEnvPrefix, key, peerID.String())
	host.client.executeWriteTicketAudience = func(_ context.Context, _ string, _ writeTicketAudience, submit func(string) error) error {
		return submit("test-ticket")
	}
	if err := host.publishCheckpoint(t.Context(), host.pendingPublication()); err == nil {
		t.Fatal("conflicting request was acknowledged")
	}
	recovered := host.pending.GetOperations()[0]
	inner, err := recovered.UnmarshalInner()
	if err != nil {
		t.Fatal(err)
	}
	if inner.GetNonce() != 4 || inner.GetLocalId() != original.GetLocalId() || string(inner.GetOpData()) != string(original.GetOpData()) {
		t.Fatalf("recovery changed operation identity or contents: %v", inner)
	}
	if err := recovered.ValidateSignature(testSharedObjectID, config.Participants); err != nil {
		t.Fatal(err)
	}
	if persisted == nil || !persisted.GetPendingPublication().EqualVT(host.pending) || host.pending.GetFirstPendingUnixMilli() != 17 {
		t.Fatal("recovery was not durable or extended the publication deadline")
	}
	if got := host.stateCtr.GetValue().GetNextAccountNonce(peerID.String()); got != 5 {
		t.Fatalf("next nonce after recovery = %d", got)
	}
	host.hydrateVerifiedStateCache(persisted)
	if err := host.acceptCloudSnapshot(t.Context(), cloud, 4); err != nil {
		t.Fatal(err)
	}
	if !host.pending.GetOperations()[0].EqualVT(recovered) {
		t.Fatal("restart rewrote the recovered operation again")
	}
	if err := host.publishCheckpoint(t.Context(), host.pendingPublication()); err != nil {
		t.Fatal(err)
	}
	if host.pending != nil {
		t.Fatal("accepted recovery batch remained pending")
	}
}

// TestWaitOperationPublishesWithoutBatchDelay exercises the confirmation wait
// against the real publication scheduler with its background routine stopped.
func TestWaitOperationPublishesWithoutBatchDelay(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	key, peerID := generateTestKeypair(t)
	config := &sobject.SharedObjectConfig{
		ConfigChainHash: []byte("verified history"),
		ConsensusMode:   sobject.SOConsensusMode_SO_CONSENSUS_MODE_SINGLE_VALIDATOR,
		Participants:    []*sobject.SOParticipantConfig{{PeerId: peerID.String(), Role: sobject.SOParticipantRole_SOParticipantRole_OWNER}},
	}
	operation := buildTestSOOperation(t, key, 1)
	inner, err := operation.UnmarshalInner()
	if err != nil {
		t.Fatal(err)
	}
	initial := &sobject.SOState{Config: config, Root: buildTestSORoot(t, key, 1, nil), Ops: []*sobject.SOOperation{operation}, QueuedAccountNonces: []*sobject.SOAccountNonce{{PeerId: peerID.String(), Nonce: 1}}}
	accepted := &sobject.SOState{Config: config, Root: buildTestSORoot(t, key, 2, []*sobject.SOAccountNonce{{PeerId: peerID.String(), Nonce: 1}})}
	var host *cloudSOHost
	var posts, uploads atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sync/push") {
			uploads.Add(1)
			w.Header().Set("Content-Type", "application/protobuf")
			return
		}
		if !strings.HasSuffix(r.URL.Path, "/ops") {
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if uploads.Load() == 0 {
			t.Error("operation reached cloud before its blocks")
		}
		posts.Add(1)
		if err := host.handleStateDelta(ctx, &api.SOStateMessage{Content: &api.SOStateMessage_Snapshot{Snapshot: accepted}, Seqno: 2}); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/protobuf")
	}))
	t.Cleanup(server.Close)
	client := NewSessionClient(server.Client(), server.URL, DefaultSigningEnvPrefix, key, peerID.String())
	client.executeWriteTicketAudience = func(_ context.Context, _ string, _ writeTicketAudience, submit func(string) error) error {
		return submit("test-ticket")
	}
	syncer := newDirtySyncExecuteTestController(t, client, nil)
	host = &cloudSOHost{
		le: logrus.New().WithField("test", t.Name()), client: client, soID: testSharedObjectID,
		privKey: key, peerID: peerID, stateCtr: ccontainer.NewCContainer(initial),
		verifiedConfig: config, lastConfigChainHash: config.ConfigChainHash,
		cloudState: initial.CloneVT(), peerState: initial.CloneVT(), syncer: syncer,
		pending:                   &api.PendingSOPublication{FirstPendingUnixMilli: time.Now().UnixMilli(), Operations: []*sobject.SOOperation{operation}},
		persistVerifiedStateCache: func(context.Context, *api.VerifiedSOStateCache) error { return nil },
	}
	host.soHost = sobject.NewSOHost(ctx, func(context.Context, string, func()) (ccontainer.Watchable[*sobject.SOState], func(), error) {
		return host.stateCtr, func() {}, nil
	}, nil, testSharedObjectID)
	syncer.setPublication(host, host.pending)
	shared := &SharedObject{host: host, localPid: peerID}
	seq, rejected, err := shared.WaitOperation(ctx, inner.GetLocalId())
	if err != nil || rejected || seq != 2 || posts.Load() != 1 {
		t.Fatalf("confirmation did not publish immediately: seq=%d rejected=%t posts=%d error=%v", seq, rejected, posts.Load(), err)
	}
}
