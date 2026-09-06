package provider_spacewave

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestTryRecoverMissingSharedObjectPeer restores a participant using an unlocked entity credential.
func TestTryRecoverMissingSharedObjectPeer(t *testing.T) {
	const (
		soID      = "so-rejoin"
		accountID = "test-account"
	)

	// Give the entity and both participants independent keys.
	entityPriv, entityPID := generateTestKeypair(t)
	ownerPriv, ownerPID := generateTestKeypair(t)
	newPriv, newPID := generateTestKeypair(t)

	// Build the signed server state for this recovery case.
	state, chainResp, envResp, keypairResp := buildRejoinTestFixtures(
		t,
		soID,
		accountID,
		ownerPriv,
		ownerPID,
		entityPriv,
		3,
	)

	// Serialize the signed fixtures through their normal codecs.
	stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
	chainJSON := mustMarshalVT(t, chainResp)
	envJSON := mustMarshalVT(t, envResp)
	keypairJSON := mustMarshalVT(t, keypairResp)

	// Capture the client mutation accepted by the HTTP service.
	var posted *api.PostConfigStateRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/state":
			_, _ = w.Write(stateJSON)
		case "/api/sobject/" + soID + "/config-chain":
			_, _ = w.Write(chainJSON)
		case "/api/sobject/" + soID + "/recovery-envelope":
			_, _ = w.Write(envJSON)
		case "/api/sobject/" + soID + "/recovery-entity-keypairs":
			_, _ = w.Write(keypairJSON)
		case "/api/sobject/" + soID + "/config-state":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read config-state body: %v", err)
				return
			}
			req := &api.PostConfigStateRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Errorf("unmarshal config-state request: %v", err)
				return
			}
			posted = req
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
	}))
	t.Cleanup(srv.Close)

	// Retain a client authenticated as the recovering participant.
	acc := NewTestProviderAccount(t, srv.URL)
	acc.sessionClient = NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, newPriv, newPID.String())
	acc.GetEntityKeyStore().Unlock(entityPID, entityPriv)

	// Mount signed state under the recovering participant identity.
	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		acc.sessionClient,
		soID,
		accountID,
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return acc.sessionClient }),
		newPriv,
		newPID,
		acc.sfs,
		nil,
		nil,
		nil,
	)
	host.soHost.SetContext(t.Context())
	so := &SharedObject{
		tkr:      &sobjectTracker{a: acc, id: soID},
		host:     host,
		privKey:  newPriv,
		localPid: newPID,
	}
	ref := sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)

	// Attempt recovery and check the resulting participant state.
	err := so.tkr.tryRecoverMissingSharedObjectPeer(
		t.Context(),
		ref,
		so,
		acc.sessionClient,
	)
	if err != nil {
		t.Fatalf("tryRecoverMissingSharedObjectPeer: %v", err)
	}
	if posted == nil {
		t.Fatal("expected config-state write")
	}
	if posted.GetKeyEpoch() == nil || len(posted.GetKeyEpoch().GetGrants()) != 2 {
		t.Fatalf("expected 2 grants in posted key epoch, got %#v", posted.GetKeyEpoch())
	}
	change := &sobject.SOConfigChange{}
	if err := change.UnmarshalVT(posted.GetConfigChange()); err != nil {
		t.Fatalf("unmarshal posted config change: %v", err)
	}
	if change.GetChangeType() != sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_SELF_ENROLL_PEER {
		t.Fatalf("expected SELF_ENROLL_PEER, got %v", change.GetChangeType())
	}
	if len(posted.GetRecoveryEnvelopes()) != 1 ||
		posted.GetRecoveryEnvelopes()[0].GetEntityId() != accountID {
		t.Fatalf("expected recovery envelope for %s", accountID)
	}
	if got := participantConfigForPeer(
		host.stateCtr.GetValue().GetConfig(),
		newPID.String(),
	); got == nil {
		t.Fatal("expected recovered peer in cached config")
	}
}

// TestTryRecoverMissingSharedObjectPeerRepairsMissingGrant restores the grant without changing existing membership.
func TestTryRecoverMissingSharedObjectPeerRepairsMissingGrant(t *testing.T) {
	const (
		soID      = "so-rejoin"
		accountID = "test-account"
	)

	// Give the entity and both participants independent keys.
	entityPriv, entityPID := generateTestKeypair(t)
	ownerPriv, ownerPID := generateTestKeypair(t)
	newPriv, newPID := generateTestKeypair(t)

	// Build the signed server state for this recovery case.
	state, chainResp, envResp, keypairResp := buildRejoinMissingGrantFixtures(
		t,
		soID,
		accountID,
		ownerPriv,
		ownerPID,
		entityPriv,
		newPriv,
		newPID,
		3,
	)

	// Serialize the signed fixtures through their normal codecs.
	stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
	chainJSON := mustMarshalVT(t, chainResp)
	envJSON := mustMarshalVT(t, envResp)
	keypairJSON := mustMarshalVT(t, keypairResp)

	// Capture the client mutation accepted by the HTTP service.
	var posted *api.PostKeyEpochRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/state":
			_, _ = w.Write(stateJSON)
		case "/api/sobject/" + soID + "/config-chain":
			_, _ = w.Write(chainJSON)
		case "/api/sobject/" + soID + "/recovery-envelope":
			_, _ = w.Write(envJSON)
		case "/api/sobject/" + soID + "/recovery-entity-keypairs":
			_, _ = w.Write(keypairJSON)
		case "/api/sobject/" + soID + "/key-epoch":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read key-epoch body: %v", err)
				return
			}
			req := &api.PostKeyEpochRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Errorf("unmarshal key-epoch request: %v", err)
				return
			}
			posted = req
			w.WriteHeader(http.StatusOK)
		case "/api/sobject/" + soID + "/config-state":
			t.Error("unexpected config-state write for missing-grant repair")
			return
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
	}))
	t.Cleanup(srv.Close)

	// Retain a client authenticated as the recovering participant.
	acc := NewTestProviderAccount(t, srv.URL)
	acc.sessionClient = NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, newPriv, newPID.String())
	acc.GetEntityKeyStore().Unlock(entityPID, entityPriv)

	// Mount signed state under the recovering participant identity.
	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		acc.sessionClient,
		soID,
		accountID,
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return acc.sessionClient }),
		newPriv,
		newPID,
		acc.sfs,
		nil,
		nil,
		nil,
	)
	host.soHost.SetContext(t.Context())
	so := &SharedObject{
		tkr:      &sobjectTracker{a: acc, id: soID},
		host:     host,
		privKey:  newPriv,
		localPid: newPID,
	}
	ref := sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)

	// Repair the missing grant through the normal recovery path.
	if err := so.tkr.tryRecoverMissingSharedObjectPeer(
		t.Context(),
		ref,
		so,
		acc.sessionClient,
	); err != nil {
		t.Fatalf("tryRecoverMissingSharedObjectPeer: %v", err)
	}
	if posted == nil {
		t.Fatal("expected key-epoch write")
	}
	if posted.GetKeyEpoch() == nil || len(posted.GetKeyEpoch().GetGrants()) != 2 {
		t.Fatalf("expected 2 grants in posted key epoch, got %#v", posted.GetKeyEpoch())
	}
	if len(posted.GetRecoveryEnvelopes()) != 1 ||
		posted.GetRecoveryEnvelopes()[0].GetEntityId() != accountID {
		t.Fatalf("expected recovery envelope for %s", accountID)
	}

	// Require the repaired grant in both root and epoch caches.
	cachedState := host.stateCtr.GetValue()
	if got := participantConfigForPeer(cachedState.GetConfig(), newPID.String()); got == nil {
		t.Fatal("expected enrolled peer in cached config")
	}
	if !soGrantSliceHasPeerID(cachedState.GetRootGrants(), newPID.String()) {
		t.Fatal("expected recovered peer grant in cached root grants")
	}
	if !peerEnrolledInCurrentEpoch(host.GetKeyEpochs(), newPID.String()) {
		t.Fatal("expected recovered peer grant in cached key epochs")
	}
}

// TestTryRecoverMissingSharedObjectPeerRequiresCredential rejects recovery without an unlocked entity key.
func TestTryRecoverMissingSharedObjectPeerRequiresCredential(t *testing.T) {
	const (
		soID      = "so-rejoin"
		accountID = "test-account"
	)

	// Give the entity and both participants independent keys.
	entityPriv, _ := generateTestKeypair(t)
	ownerPriv, ownerPID := generateTestKeypair(t)
	newPriv, newPID := generateTestKeypair(t)

	// Build the signed server state for this recovery case.
	state, chainResp, envResp, _ := buildRejoinTestFixtures(
		t,
		soID,
		accountID,
		ownerPriv,
		ownerPID,
		entityPriv,
		1,
	)

	// Serialize the signed fixtures through their normal codecs.
	stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
	chainJSON := mustMarshalVT(t, chainResp)
	envJSON := mustMarshalVT(t, envResp)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/state":
			_, _ = w.Write(stateJSON)
		case "/api/sobject/" + soID + "/config-chain":
			_, _ = w.Write(chainJSON)
		case "/api/sobject/" + soID + "/recovery-envelope":
			_, _ = w.Write(envJSON)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
	}))
	t.Cleanup(srv.Close)

	// Retain a client authenticated as the recovering participant.
	acc := NewTestProviderAccount(t, srv.URL)
	acc.sessionClient = NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, newPriv, newPID.String())
	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		acc.sessionClient,
		soID,
		accountID,
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return acc.sessionClient }),
		newPriv,
		newPID,
		acc.sfs,
		nil,
		nil,
		nil,
	)
	host.soHost.SetContext(t.Context())
	so := &SharedObject{
		tkr:      &sobjectTracker{a: acc, id: soID},
		host:     host,
		privKey:  newPriv,
		localPid: newPID,
	}
	ref := sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)

	// Attempt recovery and check the resulting participant state.
	err := so.tkr.tryRecoverMissingSharedObjectPeer(
		t.Context(),
		ref,
		so,
		acc.sessionClient,
	)
	if !errors.Is(err, sobject.ErrSharedObjectRecoveryCredentialRequired) {
		t.Fatalf("expected credential-required error, got %v", err)
	}
}

// TestTryRecoverMissingSharedObjectPeerRemovedEntity rejects an entity absent from the signed configuration.
func TestTryRecoverMissingSharedObjectPeerRemovedEntity(t *testing.T) {
	const (
		soID      = "so-rejoin"
		accountID = "test-account"
	)

	// Give the entity and both participants independent keys.
	entityPriv, entityPID := generateTestKeypair(t)
	ownerPriv, ownerPID := generateTestKeypair(t)
	newPriv, newPID := generateTestKeypair(t)

	// Build the signed server state for this recovery case.
	state, chainResp, _, _ := buildRejoinTestFixtures(
		t,
		soID,
		"other-account",
		ownerPriv,
		ownerPID,
		entityPriv,
		1,
	)

	// Serialize the signed fixtures through their normal codecs.
	stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
	chainJSON := mustMarshalVT(t, chainResp)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/state":
			_, _ = w.Write(stateJSON)
		case "/api/sobject/" + soID + "/config-chain":
			_, _ = w.Write(chainJSON)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
	}))
	t.Cleanup(srv.Close)

	// Retain a client authenticated as the recovering participant.
	acc := NewTestProviderAccount(t, srv.URL)
	acc.sessionClient = NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, newPriv, newPID.String())
	acc.GetEntityKeyStore().Unlock(entityPID, entityPriv)
	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		acc.sessionClient,
		soID,
		accountID,
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return acc.sessionClient }),
		newPriv,
		newPID,
		acc.sfs,
		nil,
		nil,
		nil,
	)
	host.soHost.SetContext(t.Context())
	so := &SharedObject{
		tkr:      &sobjectTracker{a: acc, id: soID},
		host:     host,
		privKey:  newPriv,
		localPid: newPID,
	}
	ref := sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)

	// Attempt recovery and check the resulting participant state.
	err := so.tkr.tryRecoverMissingSharedObjectPeer(
		t.Context(),
		ref,
		so,
		acc.sessionClient,
	)
	if !errors.Is(err, sobject.ErrNotParticipant) {
		t.Fatalf("expected not-participant error, got %v", err)
	}
}

// rejoinScenario builds a complete rejoin fixture set (state, chain, envelope,
// keypairs) and a path-counting httptest server. Tests vary cache prepopulation
// and assert the exact set of HTTP paths hit by tryRecoverMissingSharedObjectPeer.
type rejoinScenario struct {
	// soID identifies the shared object addressed by every fixture route.
	soID string
	// accountID identifies the entity authorized to recover membership.
	accountID string
	// state is the signed shared-object snapshot before recovery.
	state *sobject.SOState
	// chainResp proves the snapshot's configuration lineage.
	chainResp *sobject.SOConfigChainResponse
	// envResp carries encrypted recovery material for the entity.
	envResp *api.GetSORecoveryEnvelopeResponse
	// keypairResp lists the keys authorized to decrypt the envelope.
	keypairResp *api.ListSORecoveryEntityKeypairsResponse

	// srv serves the isolated recovery endpoints.
	srv *httptest.Server
	// hits counts requests under mu.
	hits map[string]int
	// mu guards request counts across HTTP handlers and assertions.
	mu sync.Mutex

	// acc retains the client's credentials and recovery cache.
	acc *ProviderAccount
	// host caches signed state observed during recovery.
	host *cloudSOHost
	// so is the client's mounted shared-object facade.
	so *SharedObject
	// ref selects the shared object for recovery.
	ref *sobject.SharedObjectRef
	// priv is the new participant's private key.
	priv crypto.PrivKey
	// pid is the new participant's public identity.
	pid peer.ID
}

// newRejoinScenario retains a signed recovery fixture and its HTTP service through the test.
func newRejoinScenario(t *testing.T) *rejoinScenario {
	t.Helper()

	// Select one isolated shared object and entity.
	const (
		soID      = "so-rejoin"
		accountID = "test-account"
	)

	// Give the entity and both participants independent keys.
	entityPriv, entityPID := generateTestKeypair(t)
	ownerPriv, ownerPID := generateTestKeypair(t)
	newPriv, newPID := generateTestKeypair(t)

	// Build the signed server state for this recovery case.
	state, chainResp, envResp, keypairResp := buildRejoinTestFixtures(
		t,
		soID,
		accountID,
		ownerPriv,
		ownerPID,
		entityPriv,
		3,
	)

	// Serialize the signed fixtures through their normal codecs.
	stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
	chainJSON := mustMarshalVT(t, chainResp)
	envJSON := mustMarshalVT(t, envResp)
	keypairJSON := mustMarshalVT(t, keypairResp)

	// Retain the recovery fixtures and request counters.
	sc := &rejoinScenario{
		soID:        soID,
		accountID:   accountID,
		state:       state,
		chainResp:   chainResp,
		envResp:     envResp,
		keypairResp: keypairResp,
		hits:        make(map[string]int),
		priv:        newPriv,
		pid:         newPID,
	}

	// Serve recovery state while recording which caches missed.
	sc.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sc.mu.Lock()
		sc.hits[r.URL.Path]++
		sc.mu.Unlock()
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/state":
			_, _ = w.Write(stateJSON)
		case "/api/sobject/" + soID + "/config-chain":
			_, _ = w.Write(chainJSON)
		case "/api/sobject/" + soID + "/recovery-envelope":
			_, _ = w.Write(envJSON)
		case "/api/sobject/" + soID + "/recovery-entity-keypairs":
			_, _ = w.Write(keypairJSON)
		case "/api/sobject/" + soID + "/config-state":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read config-state body: %v", err)
				return
			}
			req := &api.PostConfigStateRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Errorf("unmarshal config-state request: %v", err)
				return
			}
			w.WriteHeader(http.StatusOK)
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
	}))
	t.Cleanup(sc.srv.Close)

	// Prepare an authenticated account with an initially empty recovery cache.
	sc.acc = NewTestProviderAccount(t, sc.srv.URL)
	sc.acc.objStore = hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	sc.acc.sessionClient = NewSessionClient(http.DefaultClient, sc.srv.URL, DefaultSigningEnvPrefix, newPriv, newPID.String())
	sc.acc.GetEntityKeyStore().Unlock(entityPID, entityPriv)

	// Bind the shared-object host to the recovering participant.
	sc.host = newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		sc.acc.sessionClient,
		soID,
		accountID,
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return sc.acc.sessionClient }),
		newPriv,
		newPID,
		sc.acc.sfs,
		nil,
		nil,
		nil,
	)
	sc.host.soHost.SetContext(t.Context())
	sc.so = &SharedObject{
		tkr:      &sobjectTracker{a: sc.acc, id: soID},
		host:     sc.host,
		privKey:  newPriv,
		localPid: newPID,
	}
	sc.ref = sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)
	return sc
}

// primeEnvelopeCache persists the valid recovery envelope before rejoining.
func (sc *rejoinScenario) primeEnvelopeCache(t *testing.T) {
	t.Helper()
	if err := sc.acc.writeRecoveryEnvelopeCache(
		t.Context(),
		sc.soID,
		sc.envResp.GetEnvelope(),
	); err != nil {
		t.Fatalf("prime envelope cache: %v", err)
	}
}

// primeKeypairCache persists the known entity keypairs before rejoining.
func (sc *rejoinScenario) primeKeypairCache(t *testing.T) {
	t.Helper()
	for _, entity := range sc.keypairResp.GetEntities() {
		if err := sc.acc.writeRecoveryEntityKeypairsCache(
			t.Context(),
			entity,
		); err != nil {
			t.Errorf("prime keypair cache: %v", err)
			return
		}
	}
}

// run performs recovery through the normal shared-object tracker.
func (sc *rejoinScenario) run(t *testing.T) {
	t.Helper()
	if err := sc.so.tkr.tryRecoverMissingSharedObjectPeer(
		t.Context(),
		sc.ref,
		sc.so,
		sc.acc.sessionClient,
	); err != nil {
		t.Fatalf("tryRecoverMissingSharedObjectPeer: %v", err)
	}
}

// assertHit checks a route count after the recovery request completes.
func (sc *rejoinScenario) assertHit(t *testing.T, suffix string, want int) {
	t.Helper()
	sc.mu.Lock()
	got := sc.hits["/api/sobject/"+sc.soID+suffix]
	sc.mu.Unlock()
	if got != want {
		t.Fatalf("hits[%s] = %d, want %d (full hit map: %v)", suffix, got, want, sc.hits)
	}
}

// TestTryRecoverMissingSharedObjectPeerWarmCachesSkipFetches covers the
// warm-cache recovery path. A complete
// envelope + keypair cache must satisfy the rejoin path with zero
// /recovery-envelope and zero /recovery-entity-keypairs fetches.
func TestTryRecoverMissingSharedObjectPeerWarmCachesSkipFetches(t *testing.T) {
	sc := newRejoinScenario(t)
	sc.primeEnvelopeCache(t)
	sc.primeKeypairCache(t)
	sc.run(t)
	sc.assertHit(t, "/recovery-envelope", 0)
	sc.assertHit(t, "/recovery-entity-keypairs", 0)
	sc.assertHit(t, "/config-state", 1)
}

// TestTryRecoverMissingSharedObjectPeerWarmEnvelopeColdKeypairs covers the
// envelope-warm/keypairs-cold branch: envelope lookup is satisfied from cache
// but the keypair fetch still fires because the per-entity cache is empty.
func TestTryRecoverMissingSharedObjectPeerWarmEnvelopeColdKeypairs(t *testing.T) {
	sc := newRejoinScenario(t)
	sc.primeEnvelopeCache(t)
	sc.run(t)
	sc.assertHit(t, "/recovery-envelope", 0)
	sc.assertHit(t, "/recovery-entity-keypairs", 1)
	sc.assertHit(t, "/config-state", 1)
}

// TestTryRecoverMissingSharedObjectPeerColdEnvelopeWarmKeypairs covers the
// envelope-cold/keypairs-warm branch: envelope must be fetched, but keypair
// resolution is satisfied entirely from the per-entity cache.
func TestTryRecoverMissingSharedObjectPeerColdEnvelopeWarmKeypairs(t *testing.T) {
	sc := newRejoinScenario(t)
	sc.primeKeypairCache(t)
	sc.run(t)
	sc.assertHit(t, "/recovery-envelope", 1)
	sc.assertHit(t, "/recovery-entity-keypairs", 0)
	sc.assertHit(t, "/config-state", 1)
}

// TestTryRecoverMissingSharedObjectPeerColdBothFetches covers the cold-both
// branch: a fresh ProviderAccount with no cache must fetch both endpoints
// exactly once and then POST /config-state.
func TestTryRecoverMissingSharedObjectPeerColdBothFetches(t *testing.T) {
	sc := newRejoinScenario(t)
	sc.run(t)
	sc.assertHit(t, "/recovery-envelope", 1)
	sc.assertHit(t, "/recovery-entity-keypairs", 1)
	sc.assertHit(t, "/config-state", 1)
}

// TestTryRecoverMissingSharedObjectPeerStaleEnvelopeRefetches covers the
// decrypt-failure recovery path: a cached envelope whose key_epoch does not
// match the current epoch is treated as cache-invalid, the stale entry is
// dropped, and a fresh /recovery-envelope fetch lands. The post-success
// persist then writes the fresh envelope back into the cache so a subsequent
// rejoin attempt would be warm.
func TestTryRecoverMissingSharedObjectPeerStaleEnvelopeRefetches(t *testing.T) {
	sc := newRejoinScenario(t)
	stale := sc.envResp.GetEnvelope().CloneVT()
	stale.KeyEpoch = sc.envResp.GetEnvelope().GetKeyEpoch() + 99
	if err := sc.acc.writeRecoveryEnvelopeCache(
		t.Context(),
		sc.soID,
		stale,
	); err != nil {
		t.Fatalf("prime stale envelope cache: %v", err)
	}
	sc.primeKeypairCache(t)
	sc.run(t)
	sc.assertHit(t, "/recovery-envelope", 1)
	sc.assertHit(t, "/recovery-entity-keypairs", 0)
	sc.assertHit(t, "/config-state", 1)

	// Verify successful recovery replaces the stale envelope cache.
	cached, err := sc.acc.loadRecoveryEnvelopeCache(t.Context(), sc.soID)
	if err != nil {
		t.Fatalf("load envelope cache after rejoin: %v", err)
	}
	if cached == nil {
		t.Fatal("expected envelope cache repopulated after successful rejoin")
	}
	if cached.GetKeyEpoch() != sc.envResp.GetEnvelope().GetKeyEpoch() {
		t.Fatalf("expected refreshed envelope key_epoch=%d, got %d",
			sc.envResp.GetEnvelope().GetKeyEpoch(), cached.GetKeyEpoch())
	}
}

// buildRejoinTestFixtures builds signed state before the new peer is enrolled.
func buildRejoinTestFixtures(
	t *testing.T,
	soID string,
	accountID string,
	ownerPriv crypto.PrivKey,
	ownerPID peer.ID,
	entityPriv crypto.PrivKey,
	keyEpoch uint64,
) (
	*sobject.SOState,
	*sobject.SOConfigChainResponse,
	*api.GetSORecoveryEnvelopeResponse,
	*api.ListSORecoveryEntityKeypairsResponse,
) {
	t.Helper()

	// Build the encrypted block transform carried by participant grants.
	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      []byte("0123456789abcdef0123456789abcdef"),
		},
	})
	if err != nil {
		t.Fatalf("build transform config: %v", err)
	}
	grantInner := &sobject.SOGrantInner{TransformConf: transformConf}

	// Establish signed genesis membership for the original owner.
	cfg := &sobject.SharedObjectConfig{
		Participants: []*sobject.SOParticipantConfig{{
			PeerId:   ownerPID.String(),
			Role:     sobject.SOParticipantRole_SOParticipantRole_OWNER,
			EntityId: accountID,
		}},
	}
	genesisEntry, err := sobject.BuildSOConfigChange(
		&sobject.SharedObjectConfig{},
		cfg,
		sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS,
		ownerPriv,
		nil,
	)
	if err != nil {
		t.Fatalf("build genesis entry: %v", err)
	}
	genesisHash, err := sobject.HashSOConfigChange(genesisEntry)
	if err != nil {
		t.Fatalf("hash genesis entry: %v", err)
	}
	cfg = cfg.CloneVT()
	cfg.ConfigChainSeqno = genesisEntry.GetConfigSeqno()
	cfg.ConfigChainHash = genesisHash

	// Encrypt the initial decryption grant for the original owner.
	ownerPub, err := ownerPID.ExtractPublicKey()
	if err != nil {
		t.Fatalf("extract owner public key: %v", err)
	}
	ownerGrant, err := sobject.EncryptSOGrant(
		ownerPriv,
		ownerPub,
		soID,
		grantInner,
	)
	if err != nil {
		t.Fatalf("encrypt owner grant: %v", err)
	}

	// Sign the initial shared-object root with its sequence number.
	rootInnerData, err := (&sobject.SORootInner{
		Seqno:     1,
		StateData: []byte("state"),
	}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal root inner: %v", err)
	}
	root := &sobject.SORoot{InnerSeqno: 1, Inner: rootInnerData}
	if err := root.SignInnerData(
		ownerPriv,
		soID,
		root.GetInnerSeqno(),
		hash.RecommendedHashType,
	); err != nil {
		t.Fatalf("sign root: %v", err)
	}

	// Encrypt recovery material for the entity credential.
	entityPID, err := peer.IDFromPrivateKey(entityPriv)
	if err != nil {
		t.Fatalf("derive entity peer id: %v", err)
	}
	recoveryEnv, err := sobject.BuildSOEntityRecoveryEnvelope(
		accountID,
		keyEpoch,
		cfg,
		&sobject.SOEntityRecoveryMaterial{
			EntityId:   accountID,
			Role:       sobject.SOParticipantRole_SOParticipantRole_OWNER,
			GrantInner: grantInner,
		},
		[]crypto.PubKey{entityPriv.GetPublic()},
	)
	if err != nil {
		t.Fatalf("build recovery envelope: %v", err)
	}

	// Return consistent state, history, recovery material, and entity keys.
	state := &sobject.SOState{
		Config:     cfg,
		Root:       root,
		RootGrants: []*sobject.SOGrant{ownerGrant},
	}
	chain := &sobject.SOConfigChainResponse{
		ConfigChanges: []*sobject.SOConfigChange{genesisEntry},
		KeyEpochs: []*sobject.SOKeyEpoch{{
			Epoch:      keyEpoch,
			SeqnoStart: 1,
			Grants:     []*sobject.SOGrant{ownerGrant},
		}},
	}
	envelope := &api.GetSORecoveryEnvelopeResponse{
		Envelope: recoveryEnv,
	}
	keypairs := &api.ListSORecoveryEntityKeypairsResponse{
		Entities: []*api.SORecoveryEntityKeypairs{{
			EntityId: accountID,
			Keypairs: []*session.EntityKeypair{{
				PeerId: entityPID.String(),
			}},
		}},
	}
	return state, chain, envelope, keypairs
}

// buildRejoinMissingGrantFixtures builds signed membership without the new peer's decryption grant.
func buildRejoinMissingGrantFixtures(
	t *testing.T,
	soID string,
	accountID string,
	ownerPriv crypto.PrivKey,
	ownerPID peer.ID,
	entityPriv crypto.PrivKey,
	newPriv crypto.PrivKey,
	newPID peer.ID,
	keyEpoch uint64,
) (
	*sobject.SOState,
	*sobject.SOConfigChainResponse,
	*api.GetSORecoveryEnvelopeResponse,
	*api.ListSORecoveryEntityKeypairsResponse,
) {
	t.Helper()

	// Build the encrypted block transform carried by participant grants.
	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      []byte("0123456789abcdef0123456789abcdef"),
		},
	})
	if err != nil {
		t.Fatalf("build transform config: %v", err)
	}
	grantInner := &sobject.SOGrantInner{TransformConf: transformConf}

	// Establish signed genesis membership for the original owner.
	cfg := &sobject.SharedObjectConfig{
		Participants: []*sobject.SOParticipantConfig{{
			PeerId:   ownerPID.String(),
			Role:     sobject.SOParticipantRole_SOParticipantRole_OWNER,
			EntityId: accountID,
		}},
	}
	genesisEntry, err := sobject.BuildSOConfigChange(
		&sobject.SharedObjectConfig{},
		cfg,
		sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_GENESIS,
		ownerPriv,
		nil,
	)
	if err != nil {
		t.Fatalf("build genesis entry: %v", err)
	}
	genesisHash, err := sobject.HashSOConfigChange(genesisEntry)
	if err != nil {
		t.Fatalf("hash genesis entry: %v", err)
	}
	cfg = cfg.CloneVT()
	cfg.ConfigChainSeqno = genesisEntry.GetConfigSeqno()
	cfg.ConfigChainHash = genesisHash

	// Add the recovering peer to the signed configuration without its grant.
	selfEnrollEntry, err := sobject.BuildSelfEnrollPeerConfigChange(
		cfg,
		newPriv,
		newPID.String(),
		accountID,
		sobject.SOParticipantRole_SOParticipantRole_OWNER,
	)
	if err != nil {
		t.Fatalf("build self-enroll entry: %v", err)
	}
	currentCfg, err := configWithConfigChangeHash(selfEnrollEntry)
	if err != nil {
		t.Fatalf("build current config: %v", err)
	}

	// Encrypt the initial decryption grant for the original owner.
	ownerPub, err := ownerPID.ExtractPublicKey()
	if err != nil {
		t.Fatalf("extract owner public key: %v", err)
	}
	ownerGrant, err := sobject.EncryptSOGrant(
		ownerPriv,
		ownerPub,
		soID,
		grantInner,
	)
	if err != nil {
		t.Fatalf("encrypt owner grant: %v", err)
	}

	// Sign the initial shared-object root with its sequence number.
	rootInnerData, err := (&sobject.SORootInner{
		Seqno:     1,
		StateData: []byte("state"),
	}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal root inner: %v", err)
	}
	root := &sobject.SORoot{InnerSeqno: 1, Inner: rootInnerData}
	if err := root.SignInnerData(
		ownerPriv,
		soID,
		root.GetInnerSeqno(),
		hash.RecommendedHashType,
	); err != nil {
		t.Fatalf("sign root: %v", err)
	}

	// Encrypt recovery material for the entity credential.
	entityPID, err := peer.IDFromPrivateKey(entityPriv)
	if err != nil {
		t.Fatalf("derive entity peer id: %v", err)
	}
	recoveryEnv, err := sobject.BuildSOEntityRecoveryEnvelope(
		accountID,
		keyEpoch,
		currentCfg,
		&sobject.SOEntityRecoveryMaterial{
			EntityId:   accountID,
			Role:       sobject.SOParticipantRole_SOParticipantRole_OWNER,
			GrantInner: grantInner,
		},
		[]crypto.PubKey{entityPriv.GetPublic()},
	)
	if err != nil {
		t.Fatalf("build recovery envelope: %v", err)
	}

	// Return consistent state, history, recovery material, and entity keys.
	state := &sobject.SOState{
		Config:     currentCfg,
		Root:       root,
		RootGrants: []*sobject.SOGrant{ownerGrant},
	}
	chain := &sobject.SOConfigChainResponse{
		ConfigChanges: []*sobject.SOConfigChange{genesisEntry, selfEnrollEntry},
		KeyEpochs: []*sobject.SOKeyEpoch{{
			Epoch:      keyEpoch,
			SeqnoStart: 1,
			Grants:     []*sobject.SOGrant{ownerGrant},
		}},
	}
	envelope := &api.GetSORecoveryEnvelopeResponse{
		Envelope: recoveryEnv,
	}
	keypairs := &api.ListSORecoveryEntityKeypairsResponse{
		Entities: []*api.SORecoveryEntityKeypairs{{
			EntityId: accountID,
			Keypairs: []*session.EntityKeypair{{
				PeerId: entityPID.String(),
			}},
		}},
	}
	return state, chain, envelope, keypairs
}
