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

// TestVerifiedSOStateCacheRoundTrip checks that a written verified state cache
// loads back unchanged.
func TestVerifiedSOStateCacheRoundTrip(t *testing.T) {
	// Cache a verified state for one object.
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
			Epoch: 2,
			Grants: []*sobject.SOGrant{{
				PeerId: "peer-1",
			}},
		}},
	}

	// Write the cache with one history entry.
	change := &sobject.SOConfigChange{ConfigSeqno: 4}
	if err := acc.writeVerifiedSOStateCache(context.Background(), "so-1", cache, []*sobject.SOConfigChange{change}); err != nil {
		t.Fatalf("write verified SO state cache: %v", err)
	}

	// It loads back unchanged.
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

	// The history entry reads back by its hash.
	hash, err := sobject.HashSOConfigChange(change)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := acc.readVerifiedSOHistoryEntry(context.Background(), "so-1", hash)
	if err != nil || !retained.EqualVT(change) {
		t.Fatalf("history entry = %v, %v", retained, err)
	}
}

// TestNewCloudSOHostHydratesVerifiedStateCache checks that a new cloud host
// adopts its verified state cache.
func TestNewCloudSOHostHydratesVerifiedStateCache(t *testing.T) {
	// Hydrate a host from a verified cache.
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
			Epoch: 3,
		}},
	}
	host := newCloudSOHost(
		nil,
		NewSessionClient(http.DefaultClient, "http://example.com", DefaultSigningEnvPrefix, priv, pid.String()),
		"so-1",
		"",
		newTestWSTracker(nil, func() *SessionClient { return nil }),
		priv,
		pid,
		nil,
		cache,
		cloudSOHostHooks{},
		nil,
	)

	// The host adopts the cached head, key epochs, and config.
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

// TestShouldSyncVerifiedConfigChain checks when a verified head must sync to
// the current head.
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

// TestInvalidateVerifiedChainHitPath checks that invalidating removes an
// existing record and its history, so the next mount re-verifies through
// /config-chain.
func TestInvalidateVerifiedChainHitPath(t *testing.T) {
	// Persist a record with one retained change.
	objStore := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	acc := &ProviderAccount{objStore: objStore}
	cache := &api.VerifiedSOStateCache{
		GenesisHash:              []byte("genesis"),
		VerifiedConfigChainHash:  []byte("head"),
		VerifiedConfigChainSeqno: 4,
	}
	change := &sobject.SOConfigChange{ConfigSeqno: 4}
	if err := acc.writeVerifiedSOStateCache(context.Background(), "so-1", cache, []*sobject.SOConfigChange{change}); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Invalidating leaves no record.
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

	// The retained history goes with the record.
	hash, err := sobject.HashSOConfigChange(change)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := acc.readVerifiedSOHistoryEntry(context.Background(), "so-1", hash)
	if err != nil || retained != nil {
		t.Fatalf("history after invalidate = %v, %v", retained, err)
	}
}

// TestInvalidateVerifiedChainMissPath checks that invalidating an absent
// record succeeds, so callers need not probe for it first.
func TestInvalidateVerifiedChainMissPath(t *testing.T) {
	// Invalidate a record that was never written.
	objStore := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	acc := &ProviderAccount{objStore: objStore}
	if err := acc.InvalidateVerifiedChain(context.Background(), "so-missing"); err != nil {
		t.Fatalf("invalidate miss: %v", err)
	}
}

// TestVerifiedSOStateCacheKeyIsolation checks that SharedObjects keep
// independent records in one account store, so invalidating one leaves the
// others intact.
func TestVerifiedSOStateCacheKeyIsolation(t *testing.T) {
	// Persist records for two SharedObjects.
	objStore := hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())
	acc := &ProviderAccount{objStore: objStore}
	cacheA := &api.VerifiedSOStateCache{VerifiedConfigChainHash: []byte("head-a")}
	cacheB := &api.VerifiedSOStateCache{VerifiedConfigChainHash: []byte("head-b")}
	if err := acc.writeVerifiedSOStateCache(context.Background(), "so-a", cacheA, nil); err != nil {
		t.Fatalf("write so-a: %v", err)
	}
	if err := acc.writeVerifiedSOStateCache(context.Background(), "so-b", cacheB, nil); err != nil {
		t.Fatalf("write so-b: %v", err)
	}

	// Invalidate the first one.
	if err := acc.InvalidateVerifiedChain(context.Background(), "so-a"); err != nil {
		t.Fatalf("invalidate so-a: %v", err)
	}

	// Only the first record is gone.
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

// TestPullStateSkipsConfigChainOnWarmMount checks that a host hydrated with a
// verified head matching the served state does not signal the config-chain
// verifier. The server fails the test on any /config-chain request.
func TestPullStateSkipsConfigChainOnWarmMount(t *testing.T) {
	// Serve a state at the cached head.
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

	// Pull through a host hydrated at that head.
	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
		soID,
		"",
		newTestWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
		priv,
		pid,
		nil,
		&api.VerifiedSOStateCache{
			VerifiedConfigChainHash:  append([]byte(nil), warmHash...),
			VerifiedConfigChainSeqno: 5,
		},
		cloudSOHostHooks{},
		nil,
	)
	if err := host.pullState(context.Background(), SeedReasonColdSeed); err != nil {
		t.Fatalf("pullState: %v", err)
	}

	// Only /state was fetched and the verifier stays idle.
	if got := stateHits.Load(); got != 1 {
		t.Fatalf("/state hits = %d, want 1", got)
	}
	if got := configChainHits.Load(); got != 0 {
		t.Fatalf("/config-chain hits = %d, want 0 (warm cache should short-circuit)", got)
	}
	if host.configChangedRoutine.Pending() {
		t.Fatal("warm mount signaled config verifier; verifier would have fetched /config-chain")
	}
}

// TestPullStateTriggersConfigChainOnColdMount checks that a host without a
// hydrated cache signals the config-chain verifier after its first pull, so
// the warm-mount skip never suppresses cold verification.
func TestPullStateTriggersConfigChainOnColdMount(t *testing.T) {
	// Serve only /state.
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

	// Pulling through a cold host signals the verifier.
	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
		soID,
		"",
		newTestWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
		priv,
		pid,
		nil,
		nil, // cold cache: no hydration
		cloudSOHostHooks{},
		nil,
	)
	if err := host.pullState(context.Background(), SeedReasonColdSeed); err != nil {
		t.Fatalf("pullState: %v", err)
	}
	if !host.configChangedRoutine.Pending() {
		t.Fatal("cold mount did not signal config verifier; verifier would not run")
	}
}

// TestHandleSONotifyDeferToConfigChainOnInlineDelta checks that an inline
// state whose config chain hash differs from the verified head signals the
// config-chain verifier and not the pull routine: the inline state already
// arrived, and the next inline event after the chain syncs carries it forward.
// The server fails the test on any HTTP request.
func TestHandleSONotifyDeferToConfigChainOnInlineDelta(t *testing.T) {
	// Open a host at a verified head with no reachable endpoints.
	const soID = "so-bump"
	priv, pid := generateTestKeypair(t)
	verifiedHash := []byte("verified-config-chain-hash")
	bumpedHash := []byte("bumped-config-chain-hash")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected HTTP call while applying inline state: %s", r.URL.Path)
	}))
	defer srv.Close()
	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()),
		soID,
		"",
		newTestWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
		priv,
		pid,
		nil,
		&api.VerifiedSOStateCache{
			VerifiedConfigChainHash:  append([]byte(nil), verifiedHash...),
			VerifiedConfigChainSeqno: 5,
		},
		cloudSOHostHooks{},
		nil,
	)

	// Notify an inline state at a newer chain head.
	snapshot := &sobject.SOState{
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
	queueAndApplySONotify(host, payload)

	// The verifier is signaled and the pull routine is not.
	if !host.configChangedRoutine.Pending() {
		t.Fatal("config verifier was not signaled; config-chain verifier would not run")
	}
	if host.pullRoutine.Pending() {
		t.Fatal("pull routine was signaled; this is the redundant GET /state regression")
	}
}

// TestHandleSONotifyIgnoresMetadataOnly checks that a metadata-only event
// signals neither the verifier nor the pull routine.
func TestHandleSONotifyIgnoresMetadataOnly(t *testing.T) {
	// Notify a metadata-only change.
	priv, pid := generateTestKeypair(t)
	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		NewSessionClient(http.DefaultClient, "http://example.invalid", DefaultSigningEnvPrefix, priv, pid.String()),
		"so-1",
		"",
		newTestWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
		priv,
		pid,
		nil,
		nil,
		cloudSOHostHooks{},
		nil,
	)
	queueAndApplySONotify(host, &api.SONotifyEventPayload{
		ChangeType: "metadata",
		Metadata: &api.SpaceMetadataResponse{
			DisplayName: "Renamed Space",
			ObjectType:  "space",
		},
	})

	// Neither routine is signaled.
	if host.configChangedRoutine.Pending() {
		t.Fatal("metadata-only notify should not signal config verifier")
	}
	if host.pullRoutine.Pending() {
		t.Fatal("metadata-only notify should not signal pull routine")
	}
}

// TestHandleSONotifyConfigChangedPullsState checks that a configuration change,
// which carries no state, pulls the state that holds the new participant list.
func TestHandleSONotifyConfigChangedPullsState(t *testing.T) {
	// Notify a config change and expect a pending pull.
	priv, pid := generateTestKeypair(t)
	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		NewSessionClient(http.DefaultClient, "http://example.invalid", DefaultSigningEnvPrefix, priv, pid.String()),
		"so-1",
		"",
		newTestWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
		priv,
		pid,
		nil,
		nil,
		cloudSOHostHooks{},
		nil,
	)
	queueAndApplySONotify(host, &api.SONotifyEventPayload{ChangeType: "configChanged"})
	if !host.pullRoutine.Pending() {
		t.Fatal("config change notify should signal the pull routine")
	}
}

// TestApplyConfigMutationPersistsVerifiedStateCache checks that applying a
// config change persists the new verified head with only that change.
func TestApplyConfigMutationPersistsVerifiedStateCache(t *testing.T) {
	// Open a host with a cached verified head.
	priv, pid := generateTestKeypair(t)
	var persisted *api.VerifiedSOStateCache
	var persistedChanges []*sobject.SOConfigChange
	host := newCloudSOHost(
		logrus.New().WithField("test", t.Name()),
		NewSessionClient(http.DefaultClient, "http://example.com", DefaultSigningEnvPrefix, priv, pid.String()),
		"so-1",
		"",
		newTestWSTracker(logrus.New().WithField("test", t.Name()), func() *SessionClient { return nil }),
		priv,
		pid,
		nil,
		&api.VerifiedSOStateCache{
			GenesisHash:              []byte("genesis"),
			VerifiedConfigChainHash:  []byte("old-hash"),
			VerifiedConfigChainSeqno: 3,
		},
		cloudSOHostHooks{
			persistVerifiedStateCache: func(_ context.Context, cache *api.VerifiedSOStateCache, changes []*sobject.SOConfigChange) error {
				persisted = cache.CloneVT()
				persistedChanges = changes
				return nil
			},
		},
		nil,
	)

	// Hold the owner at that head and sign a change.
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
	currentConfig := host.stateCtr.GetValue().GetConfig()
	entry, err := sobject.BuildSOConfigChange("so-1", currentConfig, currentConfig, sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_ADD_PARTICIPANT, priv, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Applying persists the new verified head.
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

	// The head is the entry's hash and the entry is the only retained change.
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
	if len(persistedChanges) != 1 || !persistedChanges[0].EqualVT(entry) {
		t.Fatalf("persisted changes = %v, want only the applied entry", persistedChanges)
	}
}

// attachTestVerifiedStore backs host's verified state and history with an
// in-memory account store and returns that account.
func attachTestVerifiedStore(host *cloudSOHost) *ProviderAccount {
	acc := &ProviderAccount{objStore: hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())}
	host.persistVerifiedStateCache = func(ctx context.Context, cache *api.VerifiedSOStateCache, changes []*sobject.SOConfigChange) error {
		return acc.writeVerifiedSOStateCache(ctx, host.soID, cache, changes)
	}
	host.readRetainedConfigChange = func(ctx context.Context, hash []byte) (*sobject.SOConfigChange, error) {
		return acc.readVerifiedSOHistoryEntry(ctx, host.soID, hash)
	}
	return acc
}
