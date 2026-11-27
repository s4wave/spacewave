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
	"time"

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
	// Serve publication routes that fail until released, checking blocks precede
	// checkpoints.
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
	persist := func(ctx context.Context, cache *api.VerifiedSOStateCache, changes []*sobject.SOConfigChange) error {
		if failPersist {
			return persistErr
		}
		return account.writeVerifiedSOStateCache(ctx, testSharedObjectID, cache, changes)
	}
	host := &cloudSOHost{
		le: logrus.New().WithField("test", t.Name()), client: client,
		soID: testSharedObjectID, peerID: peerID, stateCtr: ccontainer.NewCContainer(initial),
		verifiedConfig: initial.GetConfig(), lastConfigChainHash: initial.GetConfig().GetConfigChainHash(),
		cloudState: initial.CloneVT(), persistVerifiedStateCache: persist, syncer: syncer,
	}

	// Failed persistence acknowledges nothing. Later operations stay local.
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

// TestCloudPublicationPostsOperationsAtOnce checks that a local operation
// reaches the cloud without waiting for the checkpoint interval, and only
// after the blocks dirty before it, so a member that applies it at once finds
// every block it references.
func TestCloudPublicationPostsOperationsAtOnce(t *testing.T) {
	// Record the order of block pushes and operation posts.
	var pushes atomic.Int32
	posted := make(chan struct{}, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sync/push"):
			pushes.Add(1)
		case strings.HasSuffix(r.URL.Path, "/ops"):
			if pushes.Load() == 0 {
				t.Error("operation reached the cloud before its blocks")
			}
			posted <- struct{}{}
		default:
			t.Errorf("unexpected publication route: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	// Run the scheduler with a dirty block behind an interval the test never
	// reaches.
	initial, key := newTestGenesisState(t)
	peerID := mustPeerID(t, key)
	client := newTestPublicationClient(server, key, peerID)
	syncer := newDirtySyncExecuteTestController(t, client, nil)
	syncer.conf = &SyncConfig{SizeThresholdBytes: 1 << 30, CheckpointIntervalSecs: 3600}
	account := &ProviderAccount{objStore: newSyncTestKvStore()}
	host := &cloudSOHost{
		le: logrus.New().WithField("test", t.Name()), client: client,
		soID: testSharedObjectID, peerID: peerID, stateCtr: ccontainer.NewCContainer(initial),
		verifiedConfig: initial.GetConfig(), lastConfigChainHash: initial.GetConfig().GetConfigChainHash(),
		cloudState: initial.CloneVT(), syncer: syncer,
		persistVerifiedStateCache: func(ctx context.Context, cache *api.VerifiedSOStateCache, changes []*sobject.SOConfigChange) error {
			return account.writeVerifiedSOStateCache(ctx, testSharedObjectID, cache, changes)
		},
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- syncer.Execute(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	// Each operation posts at once and leaves nothing pending.
	for range 2 {
		if err := writeLocalOperation(t, host, key); err != nil {
			t.Fatal(err)
		}
		select {
		case <-posted:
		case <-time.After(syncExecuteRequestTimeout):
			t.Fatal("operation waited for the checkpoint interval")
		}
	}
	timeout := time.After(syncExecuteRequestTimeout)
	for {
		var pending bool
		var changed <-chan struct{}
		host.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			pending, changed = host.pending != nil, getWait()
		})
		if !pending {
			return
		}
		select {
		case <-changed:
		case <-timeout:
			t.Fatal("posted operations stayed pending")
		}
	}
}

// TestCloudPublicationHoldsPastCorruptBlock checks that a flush which holds
// back a corrupt dirty block uploads the intact blocks but publishes nothing,
// keeps the corrupt block's marker, local copy and count, and resumes
// publication once the block is removed and written again while mounted.
// A write alone keeps the stored bytes, so the removal comes first.
func TestCloudPublicationHoldsPastCorruptBlock(t *testing.T) {
	// Serve block pushes and checkpoints, counting each.
	var uploads, checkpoints atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sync/push"):
			uploads.Add(1)
		case strings.HasSuffix(r.URL.Path, "/checkpoint"):
			checkpoints.Add(1)
		default:
			t.Errorf("unexpected publication route: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	// Queue three dirty blocks and read the last as corrupt.
	initial, key := newTestGenesisState(t)
	peerID := mustPeerID(t, key)
	client := newTestPublicationClient(server, key, peerID)
	syncer := newDirtySyncExecuteTestController(t, client, nil)
	inner := syncer.upper
	var marks []block_store_writeback.Mark
	var bad *block.BlockRef
	for _, body := range []string{"intact block", "corrupt block"} {
		ref, _, err := inner.PutBlock(t.Context(), []byte(body), nil)
		if err != nil {
			t.Fatal(err)
		}
		marks = append(marks, block_store_writeback.Mark{Hash: ref.GetHash(), Size: int64(len(body))})
		bad = ref
	}
	if err := syncer.MarkDirty(t.Context(), marks); err != nil {
		t.Fatal(err)
	}
	syncer.upper = &corruptUpperStore{StoreOps: inner, bad: bad.GetHash()}

	// Accept a local checkpoint so a publication is pending.
	account := &ProviderAccount{objStore: newSyncTestKvStore()}
	persist := func(ctx context.Context, cache *api.VerifiedSOStateCache, changes []*sobject.SOConfigChange) error {
		return account.writeVerifiedSOStateCache(ctx, testSharedObjectID, cache, changes)
	}
	host := &cloudSOHost{
		le: logrus.New().WithField("test", t.Name()), client: client,
		soID: testSharedObjectID, peerID: peerID, stateCtr: ccontainer.NewCContainer(initial),
		verifiedConfig: initial.GetConfig(), lastConfigChainHash: initial.GetConfig().GetConfigChainHash(),
		cloudState: initial.CloneVT(), persistVerifiedStateCache: persist, syncer: syncer,
	}
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

	// The flush uploads the intact block, holds the corrupt one and
	// publishes nothing.
	err = syncer.FlushNow(t.Context())
	var held *heldError
	if !errors.As(err, &held) || held.count != 1 || !errors.Is(err, block.ErrBlockCorrupt) {
		t.Fatalf("flush = %v; want held with 1 block", err)
	}
	if uploads.Load() == 0 || checkpoints.Load() != 0 || host.pending == nil {
		t.Fatalf("uploads = %d, checkpoints = %d, pending = %v; want pushed and nothing published",
			uploads.Load(), checkpoints.Load(), host.pending)
	}

	// The corrupt block keeps its marker, its local copy and its count.
	pending, err := syncer.scanDirtyCandidates(t.Context())
	if err != nil || len(pending) != 1 || !pending[0].hash.EqualVT(bad.GetHash()) {
		t.Fatalf("dirty after flush = %d blocks, %v; want only the corrupt block", len(pending), err)
	}
	if state := readPersistedPending(t, syncer.store); state.GetCount() != 1 {
		t.Fatalf("pending count = %d, want 1", state.GetCount())
	}
	if found, err := inner.GetBlockExists(t.Context(), bad); err != nil || !found {
		t.Fatalf("corrupt block local copy: found = %v, err = %v", found, err)
	}

	// A later flush holds it without reading it: the store now reads good data,
	// but nothing wrote the hash.
	syncer.upper = &corruptUpperStore{StoreOps: inner, bad: nil}
	if err := syncer.FlushNow(t.Context()); !errors.As(err, &held) || checkpoints.Load() != 0 {
		t.Fatalf("second flush = %v, checkpoints = %d; want held unread", err, checkpoints.Load())
	}

	// Remove the block and write it again: the write clears its entry, so
	// the next flush reads the rewritten block, uploads it and publishes.
	if err := inner.RmBlock(t.Context(), bad); err != nil {
		t.Fatal(err)
	}
	if _, _, err := inner.PutBlock(t.Context(), []byte("corrupt block"), nil); err != nil {
		t.Fatal(err)
	}
	if err := syncer.MarkDirty(t.Context(), marks[1:]); err != nil {
		t.Fatal(err)
	}
	if err := syncer.FlushNow(t.Context()); err != nil {
		t.Fatalf("flush after rewrite: %v", err)
	}
	if checkpoints.Load() != 1 || host.pending != nil {
		t.Fatalf("checkpoints = %d, pending = %v; want one published", checkpoints.Load(), host.pending)
	}
	if pending, err := syncer.scanDirtyCandidates(t.Context()); err != nil || len(pending) != 0 {
		t.Fatalf("dirty after rewrite = %d blocks, %v; want none", len(pending), err)
	}
}

// TestCloudPublicationMissingBlockCannotPublish keeps checkpoint visibility
// behind the complete dirty-block fence even when all state metadata is already
// durable.
func TestCloudPublicationMissingBlockCannotPublish(t *testing.T) {
	// Mark a block dirty that the store cannot supply.
	syncer := newDirtySyncExecuteTestController(t, nil, nil)
	ref, err := block.BuildBlockRef([]byte("unavailable dependency"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncer.MarkDirty(t.Context(), []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: 22}}); err != nil {
		t.Fatal(err)
	}

	// A pending publication cannot flush past the missing block.
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
		persistVerifiedStateCache: func(ctx context.Context, cache *api.VerifiedSOStateCache, changes []*sobject.SOConfigChange) error {
			return account.writeVerifiedSOStateCache(ctx, testSharedObjectID, cache, changes)
		},
	}

	// Write a second operation while the first publication is in flight.
	if err := writeLocalOperation(t, host, key); err != nil {
		t.Fatal(err)
	}
	sent := host.pendingPublication()
	sentAt := time.Now()
	done := make(chan error, 1)
	go func() { done <- host.publishCheckpoint(t.Context(), sent, sentAt) }()
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

	// The acknowledgment keeps the concurrent operation, and dates it from the
	// snapshot, which precedes the operation's write.
	cache, err := account.loadVerifiedSOStateCache(t.Context(), testSharedObjectID)
	if err != nil {
		t.Fatal(err)
	}
	pending := cache.GetPendingPublication()
	if len(pending.GetOperations()) != 1 {
		t.Fatal("acknowledgment lost concurrent work")
	}
	if want := max(sent.GetFirstPendingUnixMilli(), sentAt.UnixMilli()); pending.GetFirstPendingUnixMilli() != want {
		t.Fatalf("first pending time %d, want %d from the snapshot", pending.GetFirstPendingUnixMilli(), want)
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

// TestCloudConfigChangeRefusedWhileHeld checks that a configuration change
// on a store holding a corrupt dirty block returns the held error, posts
// nothing, and leaves no lock held: the write lock and the flush are both
// free afterwards.
func TestCloudConfigChangeRefusedWhileHeld(t *testing.T) {
	// Count every request but block pushes. A refused change posts nothing.
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/sync/push") {
			requests.Add(1)
		}
		w.Header().Set("Content-Type", "application/protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	// Hold the store's only dirty block as corrupt.
	initial, key := newTestGenesisState(t)
	peerID := mustPeerID(t, key)
	client := newTestPublicationClient(server, key, peerID)
	syncer := newDirtySyncExecuteTestController(t, client, nil)
	held, err := block.BuildBlockRef([]byte("dirty block"), nil)
	if err != nil {
		t.Fatal(err)
	}
	syncer.upper = &corruptUpperStore{StoreOps: syncer.upper, bad: held.GetHash()}
	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		client,
		testSharedObjectID,
		"",
		nil,
		key,
		peerID,
		nil,
		nil,
		cloudSOHostHooks{},
		syncer,
	)
	host.stateCtr.SetValue(initial)

	// Write a configuration change through the host's state lock.
	err = host.soHost.UpdateSOState(t.Context(), func(*sobject.SOState) error { return nil }, &sobject.SOConfigChange{})
	var heldErr *heldError
	if !errors.As(err, &heldErr) || !errors.Is(err, block.ErrBlockCorrupt) {
		t.Fatalf("config change = %v; want the held error", err)
	}
	if requests.Load() != 0 || !host.stateCtr.GetValue().EqualVT(initial) {
		t.Fatalf("requests = %d; want none and the state unchanged", requests.Load())
	}

	// The write lock and the flush are free.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	relLock, err := host.writeMu.Lock(ctx)
	if err != nil {
		t.Fatalf("write lock after refusal: %v", err)
	}
	relLock()
	if err := syncer.FlushNow(ctx); !errors.As(err, &heldErr) {
		t.Fatalf("flush after refusal = %v; want held, not blocked", err)
	}
}
