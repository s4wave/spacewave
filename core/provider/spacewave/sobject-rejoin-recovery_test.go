package provider_spacewave

import (
	"fmt"
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
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestTryRecoverMissingSharedObjectPeer restores a participant using an unlocked entity credential.
func TestTryRecoverMissingSharedObjectPeer(t *testing.T) {
	// Recover the missing participant.
	sc := newRejoinScenario(t, rejoinOptions{})
	sc.run(t)

	// Check the posted enrollment.
	posted := &api.PostConfigStateRequest{}
	sc.decodePosted(t, "/config-state", posted)
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
		posted.GetRecoveryEnvelopes()[0].GetEntityId() != sc.accountID {
		t.Fatalf("expected recovery envelope for %s", sc.accountID)
	}

	// Check the cached participant state.
	if got := participantConfigForPeer(sc.host.stateCtr.GetValue().GetConfig(), sc.pid.String()); got == nil {
		t.Fatal("expected recovered peer in cached config")
	}
}

// TestTryRecoverMissingSharedObjectPeerRepairsMissingGrant restores the grant without changing existing membership.
func TestTryRecoverMissingSharedObjectPeerRepairsMissingGrant(t *testing.T) {
	// Repair the enrolled peer's missing grant without a membership change.
	sc := newRejoinScenario(t, rejoinOptions{enrolled: true})
	sc.run(t)
	sc.assertHit(t, "/config-state", 0)

	// Check the posted key epoch.
	posted := &api.PostKeyEpochRequest{}
	sc.decodePosted(t, "/key-epoch", posted)
	if posted.GetKeyEpoch() == nil || len(posted.GetKeyEpoch().GetGrants()) != 2 {
		t.Fatalf("expected 2 grants in posted key epoch, got %#v", posted.GetKeyEpoch())
	}
	if len(posted.GetRecoveryEnvelopes()) != 1 ||
		posted.GetRecoveryEnvelopes()[0].GetEntityId() != sc.accountID {
		t.Fatalf("expected recovery envelope for %s", sc.accountID)
	}

	// Require the repaired grant in both the state and key epoch caches.
	cachedState := sc.host.stateCtr.GetValue()
	if got := participantConfigForPeer(cachedState.GetConfig(), sc.pid.String()); got == nil {
		t.Fatal("expected enrolled peer in cached config")
	}
	if cachedState.CurrentKeyEpoch().FindGrant(sc.pid.String()) == nil {
		t.Fatal("expected recovered peer grant in cached state grants")
	}
	if !peerEnrolledInCurrentEpoch(sc.host.GetKeyEpochs(), sc.pid.String()) {
		t.Fatal("expected recovered peer grant in cached key epochs")
	}
}

// TestTryRecoverMissingSharedObjectPeerRequiresCredential rejects recovery without an unlocked entity key.
func TestTryRecoverMissingSharedObjectPeerRequiresCredential(t *testing.T) {
	// Recover with the entity key locked.
	sc := newRejoinScenario(t, rejoinOptions{locked: true})
	if err := sc.try(t); !errors.Is(err, sobject.ErrSharedObjectRecoveryCredentialRequired) {
		t.Fatalf("expected credential-required error, got %v", err)
	}

	// Expect no key lookup or write.
	sc.assertHit(t, "/recovery-entity-keypairs", 0)
	sc.assertHit(t, "/config-state", 0)
}

// TestTryRecoverMissingSharedObjectPeerRemovedEntity rejects an entity absent from the signed configuration.
func TestTryRecoverMissingSharedObjectPeerRemovedEntity(t *testing.T) {
	// Recover an object whose signed configuration references another entity.
	sc := newRejoinScenario(t, rejoinOptions{memberEntity: "other-account"})
	if err := sc.try(t); !errors.Is(err, sobject.ErrNotParticipant) {
		t.Fatalf("expected not-participant error, got %v", err)
	}

	// Expect no recovery material read or write.
	sc.assertHit(t, "/recovery-envelope", 0)
	sc.assertHit(t, "/recovery-entity-keypairs", 0)
	sc.assertHit(t, "/config-state", 0)
}

// TestTryRecoverMissingSharedObjectPeerRecoveryCaches checks that a rejoin
// fetches the recovery envelope and the entity keypairs only when their cache
// is cold, in each combination.
func TestTryRecoverMissingSharedObjectPeerRecoveryCaches(t *testing.T) {
	for _, tc := range []struct {
		name     string
		envelope bool
		keypairs bool
	}{
		{"warm-both", true, true},
		{"warm-envelope", true, false},
		{"warm-keypairs", false, true},
		{"cold-both", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Rejoin with the selected caches primed.
			sc := newRejoinScenario(t, rejoinOptions{})
			if tc.envelope {
				sc.primeEnvelopeCache(t)
			}
			if tc.keypairs {
				sc.primeKeypairCache(t)
			}
			sc.run(t)

			// Each cold cache was fetched once, and each warm one not at all.
			fetches := func(warm bool) int {
				if warm {
					return 0
				}
				return 1
			}
			sc.assertHit(t, "/recovery-envelope", fetches(tc.envelope))
			sc.assertHit(t, "/recovery-entity-keypairs", fetches(tc.keypairs))
			sc.assertHit(t, "/config-state", 1)
		})
	}
}

// TestTryRecoverMissingSharedObjectPeerStaleEnvelopeRefetches covers the
// decrypt-failure recovery path: a cached envelope whose key_epoch does not
// match the current epoch is treated as cache-invalid, the stale entry is
// dropped, and a fresh /recovery-envelope fetch lands. The post-success
// persist then writes the fresh envelope back into the cache so a subsequent
// rejoin attempt would be warm.
func TestTryRecoverMissingSharedObjectPeerStaleEnvelopeRefetches(t *testing.T) {
	// Cache an envelope from another key epoch.
	sc := newRejoinScenario(t, rejoinOptions{})
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

	// Rejoining refetches only the envelope.
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

// rejoinOptions selects the recovery case a rejoinScenario serves.
type rejoinOptions struct {
	// enrolled signs the recovering peer into the configuration without its
	// decryption grant.
	enrolled bool
	// memberEntity is the entity the signed configuration references, when it
	// is not the recovering account.
	memberEntity string
	// locked leaves the entity key locked.
	locked bool
}

// rejoinScenario holds signed recovery fixtures, an httptest server that serves
// every recovery route and counts each request, and a shared object mounted as
// the recovering participant.
type rejoinScenario struct {
	// soID identifies the shared object addressed by every fixture route.
	soID string
	// accountID identifies the entity recovering membership.
	accountID string
	// envResp carries encrypted recovery material for the entity.
	envResp *api.GetSORecoveryEnvelopeResponse
	// keypairResp lists the keys authorized to decrypt the envelope.
	keypairResp *api.ListSORecoveryEntityKeypairsResponse

	// srv serves the isolated recovery endpoints.
	srv *httptest.Server
	// mu guards hits and posted across HTTP handlers and assertions.
	mu sync.Mutex
	// hits counts requests by path.
	hits map[string]int
	// posted holds the last request body posted to each path.
	posted map[string][]byte

	// acc retains the client's credentials and recovery cache.
	acc *ProviderAccount
	// host caches signed state observed during recovery.
	host *cloudSOHost
	// so is the client's mounted shared-object facade.
	so *SharedObject
	// ref selects the shared object for recovery.
	ref *sobject.SharedObjectRef
	// pid is the new participant's public identity.
	pid peer.ID
}

// newRejoinScenario retains a signed recovery fixture and its HTTP service through the test.
func newRejoinScenario(t *testing.T, opts rejoinOptions) *rejoinScenario {
	// Select one isolated shared object and entity.
	t.Helper()
	const (
		soID      = "so-rejoin"
		accountID = "test-account"
	)
	memberEntity := accountID
	if opts.memberEntity != "" {
		memberEntity = opts.memberEntity
	}

	// Give the entity and both participants independent keys.
	entityPriv, entityPID := generateTestKeypair(t)
	ownerPriv, ownerPID := generateTestKeypair(t)
	newPriv, newPID := generateTestKeypair(t)

	// Build the signed server state for this recovery case.
	var enrolledPriv crypto.PrivKey
	if opts.enrolled {
		enrolledPriv = newPriv
	}
	state, chainResp, envResp, keypairResp := buildRejoinFixtures(
		t,
		soID,
		memberEntity,
		ownerPriv,
		ownerPID,
		entityPriv,
		enrolledPriv,
		3,
	)

	// Serialize the signed fixtures through their normal codecs.
	prefix := "/api/sobject/" + soID
	responses := map[string][]byte{
		prefix + "/state":                    mustMarshalSOStateMessageSnapshotJSON(t, state),
		prefix + "/config-chain":             mustMarshalVT(t, chainResp),
		prefix + "/recovery-envelope":        mustMarshalVT(t, envResp),
		prefix + "/recovery-entity-keypairs": mustMarshalVT(t, keypairResp),
		prefix + "/config-state":             nil,
		prefix + "/key-epoch":                nil,
	}

	// Retain the recovery fixtures and request counters.
	sc := &rejoinScenario{
		soID:        soID,
		accountID:   accountID,
		envResp:     envResp,
		keypairResp: keypairResp,
		hits:        make(map[string]int),
		posted:      make(map[string][]byte),
		pid:         newPID,
	}

	// Serve each route, counting requests and keeping posted bodies.
	sc.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read %s body: %v", r.URL.Path, err)
			return
		}
		sc.mu.Lock()
		sc.hits[r.URL.Path]++
		if len(body) != 0 {
			sc.posted[r.URL.Path] = body
		}
		sc.mu.Unlock()
		resp, ok := responses[r.URL.Path]
		if !ok {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		_, _ = w.Write(resp)
	}))
	t.Cleanup(sc.srv.Close)

	// Prepare an authenticated account with an initially empty recovery cache.
	sc.acc = NewTestProviderAccount(t, sc.srv.URL)
	sc.acc.objStore = hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	sc.acc.ReplaceSessionClient("", newPriv, newPID)
	if !opts.locked {
		sc.acc.GetEntityKeyStore().Unlock(entityPID, entityPriv)
	}

	// Bind the shared-object host to the recovering participant.
	sc.host = newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		sc.acc.sessionClient,
		soID,
		accountID,
		newTestWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return sc.acc.sessionClient }),
		newPriv,
		newPID,
		sc.acc.sfs,
		nil,
		cloudSOHostHooks{},
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

// primeKeypairCache caches the known entity keypairs before rejoining.
func (sc *rejoinScenario) primeKeypairCache(t *testing.T) {
	t.Helper()
	sc.acc.recoveryKeypairs.set(sc.soID, sc.keypairResp)
}

// try recovers through the normal shared-object tracker.
func (sc *rejoinScenario) try(t *testing.T) error {
	t.Helper()
	return sc.so.tkr.tryRecoverMissingSharedObjectPeer(
		t.Context(),
		sc.ref,
		sc.so,
		sc.acc.sessionClient,
	)
}

// run recovers and requires success.
func (sc *rejoinScenario) run(t *testing.T) {
	t.Helper()
	if err := sc.try(t); err != nil {
		t.Fatalf("tryRecoverMissingSharedObjectPeer: %v", err)
	}
}

// assertHit checks a route count after the recovery request completes.
func (sc *rejoinScenario) assertHit(t *testing.T, suffix string, want int) {
	// Read the count under the scenario lock.
	t.Helper()
	sc.mu.Lock()
	got := sc.hits["/api/sobject/"+sc.soID+suffix]
	all := fmt.Sprint(sc.hits)
	sc.mu.Unlock()
	if got != want {
		t.Fatalf("hits[%s] = %d, want %d (full hit map: %s)", suffix, got, want, all)
	}
}

// decodePosted decodes the body last posted to a route into msg.
func (sc *rejoinScenario) decodePosted(t *testing.T, suffix string, msg interface{ UnmarshalVT([]byte) error }) {
	// Read the body under the scenario lock.
	t.Helper()
	sc.mu.Lock()
	body := sc.posted["/api/sobject/"+sc.soID+suffix]
	sc.mu.Unlock()
	if body == nil {
		t.Fatalf("expected %s write", suffix)
	}

	// Decode it.
	if err := msg.UnmarshalVT(body); err != nil {
		t.Fatalf("unmarshal %s request: %v", suffix, err)
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
	return buildRejoinFixtures(t, soID, accountID, ownerPriv, ownerPID, entityPriv, nil, keyEpoch)
}

// buildRejoinFixtures builds signed owner membership, the entity's recovery
// material and its keys. A non-nil enrolledPriv signs that peer into the
// configuration without its decryption grant.
func buildRejoinFixtures(
	t *testing.T,
	soID string,
	accountID string,
	ownerPriv crypto.PrivKey,
	ownerPID peer.ID,
	entityPriv crypto.PrivKey,
	enrolledPriv crypto.PrivKey,
	keyEpoch uint64,
) (
	*sobject.SOState,
	*sobject.SOConfigChainResponse,
	*api.GetSORecoveryEnvelopeResponse,
	*api.ListSORecoveryEntityKeypairsResponse,
) {
	// Build the encrypted block transform carried by participant grants.
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

	// Establish signed genesis membership for the original owner.
	cfg := &sobject.SharedObjectConfig{
		Participants: []*sobject.SOParticipantConfig{{
			PeerId:   ownerPID.String(),
			Role:     sobject.SOParticipantRole_SOParticipantRole_OWNER,
			EntityId: accountID,
		}},
	}
	genesisEntry, err := sobject.BuildSOConfigChange(
		soID,
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
	configChanges := []*sobject.SOConfigChange{genesisEntry}

	// Sign the enrolled peer into the configuration without its grant.
	if enrolledPriv != nil {
		enrolledPID, err := peer.IDFromPrivateKey(enrolledPriv)
		if err != nil {
			t.Fatalf("derive enrolled peer id: %v", err)
		}
		selfEnrollEntry, err := sobject.BuildSelfEnrollPeerConfigChange(
			soID,
			cfg,
			enrolledPriv,
			enrolledPID.String(),
			accountID,
			sobject.SOParticipantRole_SOParticipantRole_OWNER,
		)
		if err != nil {
			t.Fatalf("build self-enroll entry: %v", err)
		}
		cfg, err = configWithConfigChangeHash(selfEnrollEntry)
		if err != nil {
			t.Fatalf("build current config: %v", err)
		}
		configChanges = append(configChanges, selfEnrollEntry)
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

	// Sign the genesis checkpoint.
	checkpoint, err := sobject.BuildGenesisSOCheckpoint(ownerPriv, soID, genesisHash, []byte("state"))
	if err != nil {
		t.Fatalf("build genesis checkpoint: %v", err)
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
		Checkpoint: checkpoint,
		KeyEpochs: []*sobject.SOKeyEpoch{{
			Epoch:  keyEpoch,
			Grants: []*sobject.SOGrant{ownerGrant},
		}},
	}
	chain := &sobject.SOConfigChainResponse{
		ConfigChanges: configChanges,
		KeyEpochs: []*sobject.SOKeyEpoch{{
			Epoch:  keyEpoch,
			Grants: []*sobject.SOGrant{ownerGrant},
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
