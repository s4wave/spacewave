package provider_spacewave

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	provider "github.com/s4wave/spacewave/core/provider"
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
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	"github.com/sirupsen/logrus"
)

func TestTryRecoverMissingSharedObjectPeer(t *testing.T) {
	const (
		soID      = "so-rejoin"
		accountID = "test-account"
	)

	entityPriv, entityPID := generateTestKeypair(t)
	ownerPriv, ownerPID := generateTestKeypair(t)
	newPriv, newPID := generateTestKeypair(t)

	state, chainResp, envResp, keypairResp := buildRejoinTestFixtures(
		t,
		soID,
		accountID,
		ownerPriv,
		ownerPID,
		entityPriv,
		3,
	)

	stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
	chainJSON := mustMarshalVT(t, chainResp)
	envJSON := mustMarshalVT(t, envResp)
	keypairJSON := mustMarshalVT(t, keypairResp)

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
				t.Fatalf("read config-state body: %v", err)
			}
			req := &api.PostConfigStateRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal config-state request: %v", err)
			}
			posted = req
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

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
	host.soHost.SetContext(context.Background())
	so := &SharedObject{
		tkr:      &sobjectTracker{a: acc, id: soID},
		host:     host,
		privKey:  newPriv,
		localPid: newPID,
	}
	ref := sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)

	err := so.tkr.tryRecoverMissingSharedObjectPeer(
		context.Background(),
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

func TestTryRecoverMissingSharedObjectPeerRepairsMissingGrant(t *testing.T) {
	const (
		soID      = "so-rejoin"
		accountID = "test-account"
	)

	entityPriv, entityPID := generateTestKeypair(t)
	ownerPriv, ownerPID := generateTestKeypair(t)
	newPriv, newPID := generateTestKeypair(t)

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

	stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
	chainJSON := mustMarshalVT(t, chainResp)
	envJSON := mustMarshalVT(t, envResp)
	keypairJSON := mustMarshalVT(t, keypairResp)

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
				t.Fatalf("read key-epoch body: %v", err)
			}
			req := &api.PostKeyEpochRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal key-epoch request: %v", err)
			}
			posted = req
			w.WriteHeader(http.StatusOK)
		case "/api/sobject/" + soID + "/config-state":
			t.Fatal("unexpected config-state write for missing-grant repair")
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

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
	host.soHost.SetContext(context.Background())
	so := &SharedObject{
		tkr:      &sobjectTracker{a: acc, id: soID},
		host:     host,
		privKey:  newPriv,
		localPid: newPID,
	}
	ref := sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)

	if err := so.tkr.tryRecoverMissingSharedObjectPeer(
		context.Background(),
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

func TestTryRecoverMissingSharedObjectPeerRequiresCredential(t *testing.T) {
	const (
		soID      = "so-rejoin"
		accountID = "test-account"
	)

	entityPriv, _ := generateTestKeypair(t)
	ownerPriv, ownerPID := generateTestKeypair(t)
	newPriv, newPID := generateTestKeypair(t)

	state, chainResp, envResp, _ := buildRejoinTestFixtures(
		t,
		soID,
		accountID,
		ownerPriv,
		ownerPID,
		entityPriv,
		1,
	)

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
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

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
	host.soHost.SetContext(context.Background())
	so := &SharedObject{
		tkr:      &sobjectTracker{a: acc, id: soID},
		host:     host,
		privKey:  newPriv,
		localPid: newPID,
	}
	ref := sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)

	err := so.tkr.tryRecoverMissingSharedObjectPeer(
		context.Background(),
		ref,
		so,
		acc.sessionClient,
	)
	if !errors.Is(err, sobject.ErrSharedObjectRecoveryCredentialRequired) {
		t.Fatalf("expected credential-required error, got %v", err)
	}
}

func TestTryRecoverMissingSharedObjectPeerRemovedEntity(t *testing.T) {
	const (
		soID      = "so-rejoin"
		accountID = "test-account"
	)

	entityPriv, entityPID := generateTestKeypair(t)
	ownerPriv, ownerPID := generateTestKeypair(t)
	newPriv, newPID := generateTestKeypair(t)

	state, chainResp, _, _ := buildRejoinTestFixtures(
		t,
		soID,
		"other-account",
		ownerPriv,
		ownerPID,
		entityPriv,
		1,
	)

	stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
	chainJSON := mustMarshalVT(t, chainResp)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/state":
			_, _ = w.Write(stateJSON)
		case "/api/sobject/" + soID + "/config-chain":
			_, _ = w.Write(chainJSON)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

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
	host.soHost.SetContext(context.Background())
	so := &SharedObject{
		tkr:      &sobjectTracker{a: acc, id: soID},
		host:     host,
		privKey:  newPriv,
		localPid: newPID,
	}
	ref := sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)

	err := so.tkr.tryRecoverMissingSharedObjectPeer(
		context.Background(),
		ref,
		so,
		acc.sessionClient,
	)
	if !errors.Is(err, sobject.ErrNotParticipant) {
		t.Fatalf("expected not-participant error, got %v", err)
	}
}

// TestTryRecoverMissingSharedObjectPeerSkipsWhenEnrolled covers Phase 9
// iter 1: a hydrated verified cache that already contains a grant for our
// local peer in the current key epoch must short-circuit the rejoin sweep
// before any HTTP call. The httptest server fails the test if any path is
// hit; tryRecoverMissingSharedObjectPeer must return nil immediately.
func TestTryRecoverMissingSharedObjectPeerSkipsWhenEnrolled(t *testing.T) {
	const (
		soID      = "so-enrolled"
		accountID = "test-account"
	)

	_, pid := generateTestKeypair(t)
	priv, _ := generateTestKeypair(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call from rejoin gate: %s", r.URL.Path)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.sessionClient = NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		acc.sessionClient,
		soID,
		accountID,
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return acc.sessionClient }),
		priv,
		pid,
		acc.sfs,
		&api.VerifiedSOStateCache{
			VerifiedConfigChainHash:  []byte("verified-head"),
			VerifiedConfigChainSeqno: 4,
			KeyEpochs: []*sobject.SOKeyEpoch{{
				Epoch:      2,
				SeqnoStart: 1,
				Grants: []*sobject.SOGrant{{
					PeerId: pid.String(),
				}},
			}},
		},
		nil,
		nil,
	)
	host.soHost.SetContext(context.Background())
	so := &SharedObject{
		tkr:      &sobjectTracker{a: acc, id: soID},
		host:     host,
		privKey:  priv,
		localPid: pid,
	}
	ref := sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)

	if err := so.tkr.tryRecoverMissingSharedObjectPeer(
		context.Background(),
		ref,
		so,
		acc.sessionClient,
	); err != nil {
		t.Fatalf("tryRecoverMissingSharedObjectPeer: %v", err)
	}
}

func TestTryRecoverMissingSharedObjectPeerAllowsReadOnlyLifecycle(t *testing.T) {
	const (
		soID      = "so-readonly"
		accountID = "test-account"
	)

	priv, pid := generateTestKeypair(t)
	var requested bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sobject/"+soID+"/state" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		requested = true
		http.Error(w, "stop after read-only gate", http.StatusTeapot)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.state.info = &api.AccountStateResponse{
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_CANCELED,
		LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_CANCELED_GRACE_READONLY,
	}
	acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY
	acc.sessionClient = NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		acc.sessionClient,
		soID,
		accountID,
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return acc.sessionClient }),
		priv,
		pid,
		acc.sfs,
		nil,
		nil,
		nil,
	)
	host.soHost.SetContext(context.Background())
	so := &SharedObject{
		tkr:      &sobjectTracker{a: acc, id: soID},
		host:     host,
		privKey:  priv,
		localPid: pid,
	}
	ref := sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)

	if err := so.tkr.tryRecoverMissingSharedObjectPeer(
		context.Background(),
		ref,
		so,
		acc.sessionClient,
	); err == nil {
		t.Fatal("expected initial state pull to fail after read-only gate")
	}
	if !requested {
		t.Fatal("expected read-only lifecycle to attempt self-enrollment")
	}
}

func TestTryRecoverMissingSharedObjectPeerSkipsPendingDeleteLifecycle(t *testing.T) {
	const (
		soID      = "so-pending-delete"
		accountID = "test-account"
	)

	priv, pid := generateTestKeypair(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected rejoin request during pending delete lifecycle: %s", r.URL.Path)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.state.info = &api.AccountStateResponse{
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_CANCELED,
		LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_PENDING_DELETE_READONLY,
	}
	acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY
	acc.sessionClient = NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		acc.sessionClient,
		soID,
		accountID,
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return acc.sessionClient }),
		priv,
		pid,
		acc.sfs,
		nil,
		nil,
		nil,
	)
	host.soHost.SetContext(context.Background())
	so := &SharedObject{
		tkr:      &sobjectTracker{a: acc, id: soID},
		host:     host,
		privKey:  priv,
		localPid: pid,
	}
	ref := sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)

	if err := so.tkr.tryRecoverMissingSharedObjectPeer(
		context.Background(),
		ref,
		so,
		acc.sessionClient,
	); err != nil {
		t.Fatalf("tryRecoverMissingSharedObjectPeer: %v", err)
	}
}

// rejoinScenario builds a complete rejoin fixture set (state, chain, envelope,
// keypairs) and a path-counting httptest server. Tests vary cache prepopulation
// and assert the exact set of HTTP paths hit by tryRecoverMissingSharedObjectPeer.
type rejoinScenario struct {
	soID      string
	accountID string

	state       *sobject.SOState
	chainResp   *sobject.SOConfigChainResponse
	envResp     *api.GetSORecoveryEnvelopeResponse
	keypairResp *api.ListSORecoveryEntityKeypairsResponse

	srv  *httptest.Server
	hits map[string]int
	mu   sync.Mutex

	acc  *ProviderAccount
	host *cloudSOHost
	so   *SharedObject
	ref  *sobject.SharedObjectRef
	priv crypto.PrivKey
	pid  peer.ID
}

func newRejoinScenario(t *testing.T) *rejoinScenario {
	t.Helper()

	const (
		soID      = "so-rejoin"
		accountID = "test-account"
	)

	entityPriv, entityPID := generateTestKeypair(t)
	ownerPriv, ownerPID := generateTestKeypair(t)
	newPriv, newPID := generateTestKeypair(t)

	state, chainResp, envResp, keypairResp := buildRejoinTestFixtures(
		t,
		soID,
		accountID,
		ownerPriv,
		ownerPID,
		entityPriv,
		3,
	)

	stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)
	chainJSON := mustMarshalVT(t, chainResp)
	envJSON := mustMarshalVT(t, envResp)
	keypairJSON := mustMarshalVT(t, keypairResp)

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
				t.Fatalf("read config-state body: %v", err)
			}
			req := &api.PostConfigStateRequest{}
			if err := req.UnmarshalVT(body); err != nil {
				t.Fatalf("unmarshal config-state request: %v", err)
			}
			_ = req
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	t.Cleanup(sc.srv.Close)

	sc.acc = NewTestProviderAccount(t, sc.srv.URL)
	sc.acc.objStore = hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	sc.acc.sessionClient = NewSessionClient(http.DefaultClient, sc.srv.URL, DefaultSigningEnvPrefix, newPriv, newPID.String())
	sc.acc.GetEntityKeyStore().Unlock(entityPID, entityPriv)

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
	sc.host.soHost.SetContext(context.Background())
	sc.so = &SharedObject{
		tkr:      &sobjectTracker{a: sc.acc, id: soID},
		host:     sc.host,
		privKey:  newPriv,
		localPid: newPID,
	}
	sc.ref = sobject.NewSharedObjectRef("spacewave", accountID, soID, soID)
	return sc
}

func (sc *rejoinScenario) primeEnvelopeCache(t *testing.T) {
	t.Helper()
	if err := sc.acc.writeRecoveryEnvelopeCache(
		context.Background(),
		sc.soID,
		sc.envResp.GetEnvelope(),
	); err != nil {
		t.Fatalf("prime envelope cache: %v", err)
	}
}

func (sc *rejoinScenario) primeKeypairCache(t *testing.T) {
	t.Helper()
	for _, entity := range sc.keypairResp.GetEntities() {
		if err := sc.acc.writeRecoveryEntityKeypairsCache(
			context.Background(),
			entity,
		); err != nil {
			t.Fatalf("prime keypair cache: %v", err)
		}
	}
}

func (sc *rejoinScenario) run(t *testing.T) {
	t.Helper()
	if err := sc.so.tkr.tryRecoverMissingSharedObjectPeer(
		context.Background(),
		sc.ref,
		sc.so,
		sc.acc.sessionClient,
	); err != nil {
		t.Fatalf("tryRecoverMissingSharedObjectPeer: %v", err)
	}
}

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
// warm-both branch of the Phase 9 iter 2 cache-aware classifier. A complete
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
		context.Background(),
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

	cached, err := sc.acc.loadRecoveryEnvelopeCache(context.Background(), sc.soID)
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

	return &sobject.SOState{
			Config:     cfg,
			Root:       root,
			RootGrants: []*sobject.SOGrant{ownerGrant},
		}, &sobject.SOConfigChainResponse{
			ConfigChanges: []*sobject.SOConfigChange{genesisEntry},
			KeyEpochs: []*sobject.SOKeyEpoch{{
				Epoch:      keyEpoch,
				SeqnoStart: 1,
				Grants:     []*sobject.SOGrant{ownerGrant},
			}},
		}, &api.GetSORecoveryEnvelopeResponse{
			Envelope: recoveryEnv,
		}, &api.ListSORecoveryEntityKeypairsResponse{
			Entities: []*api.SORecoveryEntityKeypairs{{
				EntityId: accountID,
				Keypairs: []*session.EntityKeypair{{
					PeerId: entityPID.String(),
				}},
			}},
		}
}

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

	return &sobject.SOState{
			Config:     currentCfg,
			Root:       root,
			RootGrants: []*sobject.SOGrant{ownerGrant},
		}, &sobject.SOConfigChainResponse{
			ConfigChanges: []*sobject.SOConfigChange{genesisEntry, selfEnrollEntry},
			KeyEpochs: []*sobject.SOKeyEpoch{{
				Epoch:      keyEpoch,
				SeqnoStart: 1,
				Grants:     []*sobject.SOGrant{ownerGrant},
			}},
		}, &api.GetSORecoveryEnvelopeResponse{
			Envelope: recoveryEnv,
		}, &api.ListSORecoveryEntityKeypairsResponse{
			Entities: []*api.SORecoveryEntityKeypairs{{
				EntityId: accountID,
				Keypairs: []*session.EntityKeypair{{
					PeerId: entityPID.String(),
				}},
			}},
		}
}
