package provider_spacewave

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestCloudPublicationSurvivesRestart exercises local acceptance, failed
// persistence, cloud loss, checkpoint coalescing, and blocks-before-checkpoint
// acknowledgment.
func TestCloudPublicationSurvivesRestart(t *testing.T) {
	// Serve publication routes that fail until released, checking blocks precede checkpoints.
	var requests, checkpoints, uploads atomic.Int32
	var unavailable atomic.Bool
	unavailable.Store(true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fail while unavailable, then count each publication route.
		requests.Add(1)
		if unavailable.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/sync/push") {
			uploads.Add(1)
		} else if strings.HasSuffix(r.URL.Path, "/checkpoint") {
			if uploads.Load() == 0 {
				t.Error("checkpoint reached the cloud before its blocks")
			}
			checkpoints.Add(1)
		} else {
			t.Errorf("unexpected publication route: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	// Open a session client and syncer for the owner of a genesis state.
	initial, key := newTestGenesisState(t)
	peerID := mustPeerID(t, key)
	client := newTestPublicationClient(server, key, peerID)
	syncer := newDirtySyncExecuteTestController(t, client, nil)
	account := &ProviderAccount{objStore: newSyncTestKvStore()}

	// Hold a cache store that fails at first.
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
		verifiedConfig: initial.GetConfig(), lastConfigChainHash: initial.GetConfig().GetConfigChainHash(),
		cloudState: initial.CloneVT(), persistVerifiedStateCache: persist, syncer: syncer,
	}

	// Failed persistence acknowledges nothing; later operations stay local.
	if err := writeLocalOperation(t, host, key); !errors.Is(err, persistErr) || len(host.stateCtr.GetValue().GetOps()) != 0 {
		t.Fatalf("failed persistence acknowledged local work: %v", err)
	}
	failPersist = false
	for range 2 {
		if err := writeLocalOperation(t, host, key); err != nil {
			t.Fatal(err)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("ordinary local acceptance contacted the cloud")
	}
	first := host.pending.GetFirstPendingUnixMilli()

	// The owner's checkpoint coalesces both operations.
	base := host.stateCtr.GetValue()
	written := base.CloneVT()
	checkpoint, err := written.BuildNextCheckpoint(testSharedObjectID, key, []byte("state"))
	if err != nil {
		t.Fatal(err)
	}
	if err := written.AdoptCheckpoint(testSharedObjectID, checkpoint); err != nil {
		t.Fatal(err)
	}
	if err := host.acceptLocalWrite(t.Context(), base, written); err != nil {
		t.Fatal(err)
	}
	if len(host.pending.Operations) != 0 || host.pending.GetFirstPendingUnixMilli() != first {
		t.Fatal("coalescing retained covered operations or extended the deadline")
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

	// The reopened host keeps accepted state over a lagging cloud.
	if reopened.stateCtr.GetValue() == nil || reopened.pending.GetFirstPendingUnixMilli() != first {
		t.Fatal("restart lost accepted state or first-pending deadline")
	}
	if err := reopened.verifyPulledState(initial); err != nil {
		t.Fatal(err)
	}
	if err := reopened.acceptCloudSnapshot(t.Context(), initial, 1); err != nil {
		t.Fatal(err)
	}
	if !reopened.stateCtr.GetValue().GetCheckpoint().EqualVT(checkpoint) {
		t.Fatal("cloud lag rolled back accepted local state")
	}

	// A cloud outage keeps the publication pending.
	if err := syncer.FlushNowUnordered(t.Context()); err == nil {
		t.Fatal("cloud outage was acknowledged")
	}
	if reopened.pending == nil || checkpoints.Load() != 0 {
		t.Fatal("failed upload lost publication or exposed its checkpoint")
	}

	// Once the cloud returns, one checkpoint acknowledges the publication durably.
	unavailable.Store(false)
	if err := syncer.FlushNowUnordered(t.Context()); err != nil {
		t.Fatal(err)
	}
	if checkpoints.Load() != 1 || reopened.pending != nil {
		t.Fatal("flush did not acknowledge exactly one coalesced checkpoint")
	}
	cache, err = account.loadVerifiedSOStateCache(t.Context(), testSharedObjectID)
	if err != nil || cache.GetPendingPublication() != nil {
		t.Fatalf("acknowledgment was not durable: %v", err)
	}
}

// TestCloudPublicationMissingBlockCannotPublish keeps checkpoint visibility behind the
// complete dirty-block fence even when all state metadata is already durable.
func TestCloudPublicationMissingBlockCannotPublish(t *testing.T) {
	syncer := newDirtySyncExecuteTestController(t, nil, nil)
	ref, err := block.BuildBlockRef([]byte("unavailable dependency"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncer.MarkDirty(t.Context(), []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: 22}}); err != nil {
		t.Fatal(err)
	}
	host := &cloudSOHost{pending: &api.PendingSOPublication{FirstPendingUnixMilli: 1}}
	syncer.setPublication(host, host.pending)
	if err := syncer.FlushNowUnordered(t.Context()); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("missing dependency fence: %v", err)
	}
}

// TestCloudPublicationConcurrentAcceptance preserves work accepted during a
// publication.
func TestCloudPublicationConcurrentAcceptance(t *testing.T) {
	// Hold the first operation upload until released.
	started := make(chan struct{})
	resume := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/ops") {
			t.Errorf("unexpected route: %s", r.URL.Path)
		}
		select {
		case <-started:
		default:
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

	// Open a host as the owner of a genesis state.
	initial, key := newTestGenesisState(t)
	peerID := mustPeerID(t, key)
	account := &ProviderAccount{objStore: newSyncTestKvStore()}
	host := &cloudSOHost{
		le: logrus.New().WithField("test", t.Name()), client: newTestPublicationClient(server, key, peerID),
		soID: testSharedObjectID, peerID: peerID, stateCtr: ccontainer.NewCContainer(initial),
		verifiedConfig: initial.GetConfig(), lastConfigChainHash: initial.GetConfig().GetConfigChainHash(),
		persistVerifiedStateCache: func(ctx context.Context, cache *api.VerifiedSOStateCache) error {
			return account.writeVerifiedSOStateCache(ctx, testSharedObjectID, cache)
		},
	}

	// Write a second operation while the first publication is in flight.
	if err := writeLocalOperation(t, host, key); err != nil {
		t.Fatal(err)
	}
	sent := host.pendingPublication()
	done := make(chan error, 1)
	go func() { done <- host.publishCheckpoint(t.Context(), sent) }()
	select {
	case <-started:
	case <-t.Context().Done():
		t.Fatal(t.Context().Err())
	}
	if err := writeLocalOperation(t, host, key); err != nil {
		t.Fatal(err)
	}
	close(resume)
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	// The acknowledgment keeps the concurrent operation and its deadline.
	cache, err := account.loadVerifiedSOStateCache(t.Context(), testSharedObjectID)
	if err != nil {
		t.Fatal(err)
	}
	pending := cache.GetPendingPublication()
	if len(pending.GetOperations()) != 1 || pending.GetFirstPendingUnixMilli() != sent.GetFirstPendingUnixMilli() {
		t.Fatal("acknowledgment lost concurrent work or extended its deadline")
	}
	kept := pending.Operations[0]
	if kept.EqualVT(sent.GetOperations()[0]) || !slices.ContainsFunc(host.stateCtr.GetValue().GetOps(), kept.EqualVT) {
		t.Fatal("acknowledgment kept the wrong operation")
	}
}

// newTestPublicationClient returns a session client for server.
func newTestPublicationClient(server *httptest.Server, key crypto.PrivKey, peerID peer.ID) *SessionClient {
	return NewSessionClient(server.Client(), server.URL, DefaultSigningEnvPrefix, key, peerID.String())
}

// writeLocalOperation writes one operation signed with key to the accepted
// state of host, as the local Shared Object host does.
func writeLocalOperation(t *testing.T, host *cloudSOHost, key crypto.PrivKey) error {
	// Write an operation onto a copy of the host state and accept it.
	t.Helper()
	base := host.stateCtr.GetValue()
	written := base.CloneVT()
	writeTestOperation(t, written, key)
	return host.acceptLocalWrite(t.Context(), base, written)
}

// mustPeerID returns the peer ID of key.
func mustPeerID(t *testing.T, key crypto.PrivKey) peer.ID {
	t.Helper()
	peerID, err := peer.IDFromPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return peerID
}
