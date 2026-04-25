package provider_spacewave

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/sirupsen/logrus"
)

func TestVerifiedSOStateCacheRoundTrip(t *testing.T) {
	objStore := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	acc := &ProviderAccount{objStore: objStore}
	cache := &api.VerifiedSOStateCache{
		GenesisHash:              []byte("genesis"),
		VerifiedConfigChainHash:  []byte("head"),
		VerifiedConfigChainSeqno: 4,
		CurrentConfig: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{{
				EntityId: "acct-1",
				Role:     sobject.SOParticipantRole_SOParticipantRole_READER,
			}},
		},
		KeyEpochs: []*sobject.SOKeyEpoch{{
			Epoch:      2,
			SeqnoStart: 5,
			Grants: []*sobject.SOGrant{{
				PeerId: "peer-1",
			}},
		}},
	}

	if err := acc.writeVerifiedSOStateCache(context.Background(), "so-1", cache); err != nil {
		t.Fatalf("write verified SO state cache: %v", err)
	}

	loaded, err := acc.loadVerifiedSOStateCache(context.Background(), "so-1")
	if err != nil {
		t.Fatalf("load verified SO state cache: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected verified SO state cache")
	}
	if !loaded.EqualVT(cache) {
		t.Fatalf("loaded cache mismatch: got %+v want %+v", loaded, cache)
	}
}

func TestNewCloudSOHostHydratesVerifiedStateCache(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	cache := &api.VerifiedSOStateCache{
		GenesisHash:              []byte("genesis"),
		VerifiedConfigChainHash:  []byte("head"),
		VerifiedConfigChainSeqno: 7,
		CurrentConfig: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{{
				EntityId: "acct-1",
				Role:     sobject.SOParticipantRole_SOParticipantRole_WRITER,
			}},
		},
		KeyEpochs: []*sobject.SOKeyEpoch{{
			Epoch:      3,
			SeqnoStart: 8,
		}},
	}

	host := newCloudSOHost(
		nil,
		NewSessionClient(http.DefaultClient, "http://example.com", DefaultSigningEnvPrefix, priv, pid.String()),
		"so-1",
		"",
		newWSTracker(nil, func() *SessionClient { return nil }),
		priv,
		pid,
		nil,
		cache,
		nil,
		nil,
	)

	if string(host.genesisHash) != "genesis" {
		t.Fatalf("unexpected genesis hash: %q", host.genesisHash)
	}
	if string(host.lastConfigChainHash) != "head" {
		t.Fatalf("unexpected verified config head hash: %q", host.lastConfigChainHash)
	}
	if host.verifiedConfigChainSeqno != 7 {
		t.Fatalf("unexpected verified config head seqno: %d", host.verifiedConfigChainSeqno)
	}
	if len(host.keyEpochs) != 1 || host.keyEpochs[0].GetEpoch() != 3 {
		t.Fatalf("unexpected hydrated key epochs: %+v", host.keyEpochs)
	}
	if got := readableParticipantRoleForEntity(host.verifiedConfig, "acct-1"); got != sobject.SOParticipantRole_SOParticipantRole_WRITER {
		t.Fatalf("unexpected hydrated config role: %s", got)
	}
}

func TestShouldSyncVerifiedConfigChain(t *testing.T) {
	tests := []struct {
		name          string
		currentHash   []byte
		currentSeqno  uint64
		verifiedHash  []byte
		verifiedSeqno uint64
		want          bool
	}{
		{
			name:          "missing current hash",
			currentSeqno:  4,
			verifiedHash:  []byte("head"),
			verifiedSeqno: 4,
			want:          false,
		},
		{
			name:         "missing verified chain",
			currentHash:  []byte("head"),
			currentSeqno: 4,
			want:         true,
		},
		{
			name:          "verified behind by seqno",
			currentHash:   []byte("new"),
			currentSeqno:  5,
			verifiedHash:  []byte("old"),
			verifiedSeqno: 4,
			want:          true,
		},
		{
			name:          "verified current",
			currentHash:   []byte("head"),
			currentSeqno:  4,
			verifiedHash:  []byte("head"),
			verifiedSeqno: 4,
			want:          false,
		},
		{
			name:          "verified ahead",
			currentHash:   []byte("old"),
			currentSeqno:  3,
			verifiedHash:  []byte("new"),
			verifiedSeqno: 4,
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSyncVerifiedConfigChain(
				tt.currentHash,
				tt.currentSeqno,
				tt.verifiedHash,
				tt.verifiedSeqno,
			); got != tt.want {
				t.Fatalf("shouldSyncVerifiedConfigChain() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestInvalidateVerifiedChainHitPath covers iter 5: an existing persisted
// record is removed so the next load returns nil, forcing the next mount
// to re-verify via /config-chain.
func TestInvalidateVerifiedChainHitPath(t *testing.T) {
	objStore := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	acc := &ProviderAccount{objStore: objStore}
	cache := &api.VerifiedSOStateCache{
		GenesisHash:              []byte("genesis"),
		VerifiedConfigChainHash:  []byte("head"),
		VerifiedConfigChainSeqno: 4,
	}

	if err := acc.writeVerifiedSOStateCache(context.Background(), "so-1", cache); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := acc.InvalidateVerifiedChain(context.Background(), "so-1"); err != nil {
		t.Fatalf("invalidate: %v", err)
	}
	loaded, err := acc.loadVerifiedSOStateCache(context.Background(), "so-1")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil after invalidate, got %+v", loaded)
	}
}

// TestInvalidateVerifiedChainMissPath covers iter 5: invalidating an
// already-absent record is a no-op (no error). Callers can invalidate
// defensively without first probing for presence.
func TestInvalidateVerifiedChainMissPath(t *testing.T) {
	objStore := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	acc := &ProviderAccount{objStore: objStore}
	if err := acc.InvalidateVerifiedChain(context.Background(), "so-missing"); err != nil {
		t.Fatalf("invalidate miss: %v", err)
	}
}

// TestVerifiedSOStateCacheKeyIsolation covers iter 4: separate SO ids
// produce independent records within the same account ObjectStore so
// invalidating one SO does not affect any other SO's verified state.
func TestVerifiedSOStateCacheKeyIsolation(t *testing.T) {
	objStore := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	acc := &ProviderAccount{objStore: objStore}

	cacheA := &api.VerifiedSOStateCache{VerifiedConfigChainHash: []byte("head-a")}
	cacheB := &api.VerifiedSOStateCache{VerifiedConfigChainHash: []byte("head-b")}
	if err := acc.writeVerifiedSOStateCache(context.Background(), "so-a", cacheA); err != nil {
		t.Fatalf("write so-a: %v", err)
	}
	if err := acc.writeVerifiedSOStateCache(context.Background(), "so-b", cacheB); err != nil {
		t.Fatalf("write so-b: %v", err)
	}

	if err := acc.InvalidateVerifiedChain(context.Background(), "so-a"); err != nil {
		t.Fatalf("invalidate so-a: %v", err)
	}

	loadedA, err := acc.loadVerifiedSOStateCache(context.Background(), "so-a")
	if err != nil {
		t.Fatalf("load so-a: %v", err)
	}
	if loadedA != nil {
		t.Fatalf("expected so-a invalidated, got %+v", loadedA)
	}

	loadedB, err := acc.loadVerifiedSOStateCache(context.Background(), "so-b")
	if err != nil {
		t.Fatalf("load so-b: %v", err)
	}
	if loadedB == nil || string(loadedB.GetVerifiedConfigChainHash()) != "head-b" {
		t.Fatalf("expected so-b intact, got %+v", loadedB)
	}
}

// TestPullStateSkipsConfigChainOnWarmMount covers iter 2 (hydrate-on-host-
// start) end-to-end: when newCloudSOHost is constructed with a verified
// cache whose hash matches the SO state the cloud serves, pullState must
// not signal the config-chain verifier. The HTTP server fails the test if
// /config-chain is hit at all.
func TestPullStateSkipsConfigChainOnWarmMount(t *testing.T) {
	const soID = "so-warm"
	priv, pid := generateTestKeypair(t)
	warmHash := []byte("warm-config-chain-hash")

	state := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			ConfigChainHash:  append([]byte(nil), warmHash...),
			ConfigChainSeqno: 5,
		},
	}
	stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)

	var (
		stateHits       atomic.Int32
		configChainHits atomic.Int32
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sobject/" + soID + "/state":
			stateHits.Add(1)
			_, _ = w.Write(stateJSON)
		case "/api/sobject/" + soID + "/config-chain":
			configChainHits.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
		soID,
		"",
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
		priv,
		pid,
		nil,
		&api.VerifiedSOStateCache{
			VerifiedConfigChainHash:  append([]byte(nil), warmHash...),
			VerifiedConfigChainSeqno: 5,
		},
		nil,
		nil,
	)

	if err := host.pullState(context.Background(), SeedReasonColdSeed); err != nil {
		t.Fatalf("pullState: %v", err)
	}

	if got := stateHits.Load(); got != 1 {
		t.Fatalf("/state hits = %d, want 1", got)
	}
	if got := configChainHits.Load(); got != 0 {
		t.Fatalf("/config-chain hits = %d, want 0 (warm cache should short-circuit)", got)
	}
	select {
	case <-host.configChangedCh:
		t.Fatal("warm mount signaled configChangedCh; verifier would have fetched /config-chain")
	default:
	}
}

// TestPullStateTriggersConfigChainOnColdMount is the symmetric counterpart
// to TestPullStateSkipsConfigChainOnWarmMount: with no hydrated cache the
// config-chain verifier MUST be signaled so the first /state pull is
// followed by a /config-chain fetch on the verifier goroutine. This
// guards against a regression where the warm-skip logic also accidentally
// suppresses cold-mount verification.
func TestPullStateTriggersConfigChainOnColdMount(t *testing.T) {
	const soID = "so-cold"
	priv, pid := generateTestKeypair(t)

	state := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			ConfigChainHash:  []byte("server-hash"),
			ConfigChainSeqno: 5,
		},
	}
	stateJSON := mustMarshalSOStateMessageSnapshotJSON(t, state)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sobject/"+soID+"/state" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_, _ = w.Write(stateJSON)
	}))
	defer srv.Close()

	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
		soID,
		"",
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
		priv,
		pid,
		nil,
		nil, // cold cache: no hydration
		nil,
		nil,
	)

	if err := host.pullState(context.Background(), SeedReasonColdSeed); err != nil {
		t.Fatalf("pullState: %v", err)
	}
	select {
	case <-host.configChangedCh:
	default:
		t.Fatal("cold mount did not signal configChangedCh; verifier would not run")
	}
}

// TestHandleSONotifyDeferToConfigChainOnInlineDelta covers the Phase 6
// iter 6 regression: an inline so_notify carrying a state whose
// config_chain_hash differs from the verified hash made handleStateDelta
// return errSOConfigChainChanged, after which handleSONotify previously
// fell through to triggerPull and fired a redundant GET /state.
//
// Contract: handleSONotify must signal the config-chain verifier (so it
// can fetch /config-chain) and must NOT signal pullCh (the inline state
// already arrived; the next inline event after the chain syncs carries
// it forward, or gap recovery rerun a full pull). The HTTP server fails
// the test if /state is hit at all.
func TestHandleSONotifyDeferToConfigChainOnInlineDelta(t *testing.T) {
	const soID = "so-bump"
	priv, pid := generateTestKeypair(t)
	verifiedHash := []byte("verified-config-chain-hash")
	bumpedHash := []byte("bumped-config-chain-hash")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call from handleSONotify: %s", r.URL.Path)
	}))
	defer srv.Close()

	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
		soID,
		"",
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
		priv,
		pid,
		nil,
		&api.VerifiedSOStateCache{
			VerifiedConfigChainHash:  append([]byte(nil), verifiedHash...),
			VerifiedConfigChainSeqno: 5,
		},
		nil,
		nil,
	)

	// Root must be non-nil so verifyPulledState exercises the config-chain
	// hash check (it short-circuits to nil when there is no root).
	snapshot := &sobject.SOState{
		Root: &sobject.SORoot{InnerSeqno: 10},
		Config: &sobject.SharedObjectConfig{
			ConfigChainHash:  append([]byte(nil), bumpedHash...),
			ConfigChainSeqno: 6,
		},
	}
	payload := &api.SONotifyEventPayload{
		Seqno:      1,
		ChangeType: "op",
		StateMessage: &api.SOStateMessage{
			Seqno:   1,
			Content: &api.SOStateMessage_Snapshot{Snapshot: snapshot},
		},
	}

	host.handleSONotify(payload)

	select {
	case <-host.configChangedCh:
	default:
		t.Fatal("configChangedCh was not signaled; config-chain verifier would not run")
	}

	select {
	case <-host.pullCh:
		t.Fatal("pullCh was signaled; this is the Phase 6 iter 6 regression (redundant GET /state)")
	default:
	}
}

func TestHandleSONotifyIgnoresMetadataOnly(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		NewSessionClient(http.DefaultClient, "http://example.invalid", DefaultSigningEnvPrefix, priv, pid.String()),
		"so-1",
		"",
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
		priv,
		pid,
		nil,
		nil,
		nil,
		nil,
	)

	host.handleSONotify(&api.SONotifyEventPayload{
		ChangeType: "metadata",
		Metadata: &api.SpaceMetadataResponse{
			DisplayName: "Renamed Space",
			ObjectType:  "space",
		},
	})

	select {
	case <-host.configChangedCh:
		t.Fatal("metadata-only notify should not signal configChangedCh")
	default:
	}
	select {
	case <-host.pullCh:
		t.Fatal("metadata-only notify should not signal pullCh")
	default:
	}
}

func TestApplyConfigMutationPersistsVerifiedStateCache(t *testing.T) {
	priv, pid := generateTestKeypair(t)
	var persisted *api.VerifiedSOStateCache
	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		NewSessionClient(http.DefaultClient, "http://example.com", DefaultSigningEnvPrefix, priv, pid.String()),
		"so-1",
		"",
		newWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
		priv,
		pid,
		nil,
		&api.VerifiedSOStateCache{
			GenesisHash:              []byte("genesis"),
			VerifiedConfigChainHash:  []byte("old-hash"),
			VerifiedConfigChainSeqno: 3,
		},
		func(_ context.Context, cache *api.VerifiedSOStateCache) error {
			persisted = cache.CloneVT()
			return nil
		},
		nil,
	)

	host.stateCtr.SetValue(&sobject.SOState{
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{{
				PeerId: pid.String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_OWNER,
			}},
			ConfigChainHash:  []byte("old-hash"),
			ConfigChainSeqno: 3,
		},
	})
	entry := &sobject.SOConfigChange{
		ConfigSeqno: 4,
		Config: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{{
				PeerId: pid.String(),
				Role:   sobject.SOParticipantRole_SOParticipantRole_OWNER,
			}},
		},
		ChangeType:   sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT,
		PreviousHash: []byte("old-hash"),
	}

	if err := host.applyConfigMutation(context.Background(), entry, nil, nil); err != nil {
		t.Fatalf("apply config mutation: %v", err)
	}
	if persisted == nil {
		t.Fatal("expected verified SO cache persistence callback")
	}
	if string(persisted.GetGenesisHash()) != "genesis" {
		t.Fatalf("unexpected persisted genesis hash: %q", persisted.GetGenesisHash())
	}
	if persisted.GetVerifiedConfigChainSeqno() != 4 {
		t.Fatalf("unexpected persisted verified seqno: %d", persisted.GetVerifiedConfigChainSeqno())
	}
	hash, err := sobject.HashSOConfigChange(entry)
	if err != nil {
		t.Fatalf("hash config change: %v", err)
	}
	if !bytes.Equal(persisted.GetVerifiedConfigChainHash(), hash) {
		t.Fatalf("unexpected persisted verified hash: %x", persisted.GetVerifiedConfigChainHash())
	}
	if persisted.GetCurrentConfig() == nil {
		t.Fatal("expected current config in persisted verified cache")
	}
}
