package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/cdn"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	"github.com/s4wave/spacewave/net/crypto"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	"github.com/sirupsen/logrus"
)

func TestSelfEnrollmentEnumerator(t *testing.T) {
	// Build an account, a shared object list, and cached SO states.
	_, sessionPID, _ := generateEntityKey(t)
	acc := &ProviderAccount{
		accountID: "acct-1",
		objStore:  hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]()),
	}
	list := &sobject.SharedObjectList{SharedObjects: []*sobject.SharedObjectListEntry{
		selfEnrollmentListEntry("needs"),
		selfEnrollmentListEntry("missing-cache"),
		selfEnrollmentListEntry("no-participant"),
		selfEnrollmentListEntry("already-enrolled"),
		selfEnrollmentListEntry("cdn"),
	}}
	list.SharedObjects[4].Meta = &sobject.SharedObjectMeta{BodyType: "cdn"}
	writeSelfEnrollmentCache(t, acc, "needs", "acct-1", "", sobject.SOParticipantRole_SOParticipantRole_READER)
	writeSelfEnrollmentCache(t, acc, "no-participant", "other", "", sobject.SOParticipantRole_SOParticipantRole_READER)
	writeSelfEnrollmentCache(t, acc, "already-enrolled", "acct-1", sessionPID.String(), sobject.SOParticipantRole_SOParticipantRole_READER)
	writeSelfEnrollmentCache(t, acc, "cdn", "acct-1", "", sobject.SOParticipantRole_SOParticipantRole_READER)

	// Enumerate candidates and expect only the missing-cache and needs entries.
	got, err := acc.enumerateSelfEnrollmentCandidates(context.Background(), list, sessionPID, "acct-1")
	if err != nil {
		t.Fatalf("enumerate: %v", err)
	}
	if got.count != 2 {
		t.Fatalf("count = %d, want 2", got.count)
	}
	if len(got.ids) != 2 || got.ids[0] != "missing-cache" || got.ids[1] != "needs" {
		t.Fatalf("ids = %#v, want [missing-cache needs]", got.ids)
	}
	if !got.loaded {
		t.Fatal("loaded = false, want true when the list is known")
	}
	if got.generationKey == "" {
		t.Fatal("expected generation key")
	}
}

func TestSelfEnrollmentEnumeratorLoadedWhenAllEntriesEvaluated(t *testing.T) {
	// Build an account and cached SO states without excluded entries.
	_, sessionPID, _ := generateEntityKey(t)
	acc := &ProviderAccount{
		accountID: "acct-1",
		objStore:  hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]()),
	}
	list := &sobject.SharedObjectList{SharedObjects: []*sobject.SharedObjectListEntry{
		selfEnrollmentListEntry("needs"),
		selfEnrollmentListEntry("no-participant"),
		selfEnrollmentListEntry("already-enrolled"),
	}}
	writeSelfEnrollmentCache(t, acc, "needs", "acct-1", "", sobject.SOParticipantRole_SOParticipantRole_READER)
	writeSelfEnrollmentCache(t, acc, "no-participant", "other", "", sobject.SOParticipantRole_SOParticipantRole_READER)
	writeSelfEnrollmentCache(t, acc, "already-enrolled", "acct-1", sessionPID.String(), sobject.SOParticipantRole_SOParticipantRole_READER)

	// Enumerate candidates and expect a loaded summary with one entry.
	got, err := acc.enumerateSelfEnrollmentCandidates(context.Background(), list, sessionPID, "acct-1")
	if err != nil {
		t.Fatalf("enumerate: %v", err)
	}
	if !got.loaded {
		t.Fatal("loaded = false, want true")
	}
	if got.generationKey == "" {
		t.Fatal("expected generation key")
	}
	if got.count != 1 || len(got.ids) != 1 || got.ids[0] != "needs" {
		t.Fatalf("summary = %+v, want one needs entry", got)
	}
}

func TestSelfEnrollmentEnumeratorSkipsPublicCDNSpace(t *testing.T) {
	// Build a list with the CDN space and a regular space.
	_, sessionPID, _ := generateEntityKey(t)
	acc := &ProviderAccount{
		accountID: "acct-1",
		objStore:  hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]()),
	}
	list := &sobject.SharedObjectList{SharedObjects: []*sobject.SharedObjectListEntry{
		selfEnrollmentListEntry(cdn.SpaceID()),
		selfEnrollmentListEntry("needs"),
	}}
	writeSelfEnrollmentCache(t, acc, cdn.SpaceID(), "acct-1", "", sobject.SOParticipantRole_SOParticipantRole_READER)
	writeSelfEnrollmentCache(t, acc, "needs", "acct-1", "", sobject.SOParticipantRole_SOParticipantRole_READER)

	// Enumerate candidates and expect only the regular space.
	got, err := acc.enumerateSelfEnrollmentCandidates(context.Background(), list, sessionPID, "acct-1")
	if err != nil {
		t.Fatalf("enumerate: %v", err)
	}
	if got.count != 1 || len(got.ids) != 1 || got.ids[0] != "needs" {
		t.Fatalf("summary = %+v, want only regular space", got)
	}
}

func TestSelfEnrollmentEnumeratorEmptyList(t *testing.T) {
	// Enumerate an empty shared object list.
	_, sessionPID, _ := generateEntityKey(t)
	acc := &ProviderAccount{objStore: hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())}
	got, err := acc.enumerateSelfEnrollmentCandidates(
		context.Background(),
		&sobject.SharedObjectList{},
		sessionPID,
		"acct-1",
	)

	// The summary must be loaded and empty.
	if err != nil {
		t.Fatalf("enumerate: %v", err)
	}
	if got.count != 0 || got.generationKey != "" || len(got.ids) != 0 {
		t.Fatalf("unexpected summary: %+v", got)
	}
	if !got.loaded {
		t.Fatal("loaded = false, want true for empty list")
	}
}

func TestRefreshSelfEnrollmentSummaryLoadsEmptyList(t *testing.T) {
	// Serve an empty shared object list and count fetches.
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sobject/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		hits++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mustMarshalVT(t, &sobject.SharedObjectList{}))
	}))
	defer srv.Close()

	// Refresh the summary against the active account.
	acc := NewTestProviderAccount(t, srv.URL)
	acc.syncSharedObjectListAccess(
		s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
	)

	// The refresh must fetch the list once and cache an empty loaded summary.
	if err := acc.RefreshSelfEnrollmentSummary(context.Background()); err != nil {
		t.Fatalf("refresh self-enrollment summary: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected one shared object list fetch, got %d", hits)
	}
	got := acc.GetSelfEnrollmentSummary()
	if got == nil {
		t.Fatal("expected summary")
	}
	if got.count != 0 || got.generationKey != "" || len(got.ids) != 0 {
		t.Fatalf("summary = %+v, want loaded empty summary", got)
	}
	if !got.loaded {
		t.Fatal("loaded = false, want true for fetched empty list")
	}
}

func TestRefreshSelfEnrollmentSummaryInvalidatesOnUnauthError(t *testing.T) {
	// Serve an unauthenticated shared object list response.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sobject/list" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"unknown_session","message":"Session not found"}`))
	}))
	defer srv.Close()

	// Prime a stale summary before the refresh.
	acc := NewTestProviderAccount(t, srv.URL)
	acc.syncSharedObjectListAccess(
		s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
	)
	if err := acc.setSelfEnrollmentSummary(&SelfEnrollmentSummary{
		ids:           []string{"stale"},
		generationKey: "stale-key",
		count:         1,
		loaded:        true,
	}); err != nil {
		t.Fatalf("prime summary: %v", err)
	}

	// The refresh must fail with an unauth error and clear the summary.
	err := acc.RefreshSelfEnrollmentSummary(context.Background())
	if !isUnauthCloudError(err) {
		t.Fatalf("RefreshSelfEnrollmentSummary() = %v, want unauth cloud error", err)
	}
	if got := acc.GetSelfEnrollmentSummary(); got != nil {
		t.Fatalf("summary = %+v, want nil after unauth refresh failure", got)
	}
}

func TestSelfEnrollmentSummaryRefreshesOnCacheWrite(t *testing.T) {
	// Build an account with a pending missing-cache entry.
	priv, sessionPID, _ := generateEntityKey(t)
	acc := newSelfEnrollmentSummaryTestAccount(t, priv, sessionPID.String())
	acc.cacheSharedObjectListEntry(selfEnrollmentListEntry("needs"))

	// The initial summary must report the pending entry.
	if got := acc.GetSelfEnrollmentSummary(); got == nil || got.count != 1 || !got.loaded || len(got.ids) != 1 || got.ids[0] != "needs" {
		t.Fatalf("summary before cache write = %+v, want pending missing-cache summary", got)
	}

	// Write the enrollment cache and expect an account broadcast.
	var ch <-chan struct{}
	acc.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
	})
	writeSelfEnrollmentCache(t, acc, "needs", "acct-1", sessionPID.String(), sobject.SOParticipantRole_SOParticipantRole_READER)
	select {
	case <-ch:
	default:
		t.Fatal("expected account broadcast after summary changed")
	}

	// The summary must now be empty and loaded.
	got := acc.GetSelfEnrollmentSummary()
	if got == nil {
		t.Fatal("expected summary")
	}
	if got.count != 0 || got.generationKey != "" || !got.loaded || len(got.ids) != 0 {
		t.Fatalf("summary = %+v, want empty enrolled summary", got)
	}
}

func TestSelfEnrollmentSummaryRefreshesOnSessionPeerChange(t *testing.T) {
	// Build an account with a pending entry and capture its summary.
	priv, sessionPID, _ := generateEntityKey(t)
	acc := newSelfEnrollmentSummaryTestAccount(t, priv, sessionPID.String())
	acc.cacheSharedObjectListEntry(selfEnrollmentListEntry("needs"))
	writeSelfEnrollmentCache(t, acc, "needs", "acct-1", "", sobject.SOParticipantRole_SOParticipantRole_READER)

	// The first summary must carry a generation key.
	first := acc.GetSelfEnrollmentSummary()
	if first == nil || first.generationKey == "" {
		t.Fatalf("summary = %+v, want generation key", first)
	}

	// Replace the session client and expect a new generation key.
	nextPriv, nextPID, _ := generateEntityKey(t)
	acc.ReplaceSessionClient(NewSessionClient(http.DefaultClient, "http://example.invalid", DefaultSigningEnvPrefix, nextPriv, nextPID.String()))
	second := acc.GetSelfEnrollmentSummary()
	if second == nil || second.generationKey == "" {
		t.Fatalf("summary after peer change = %+v, want generation key", second)
	}
	if first.generationKey == second.generationKey {
		t.Fatal("expected generation key to change with session peer")
	}
}

func TestSelfEnrollmentSummaryClearsWhenPeerEnrolled(t *testing.T) {
	// Build an account with a pending entry before enrollment.
	priv, sessionPID, _ := generateEntityKey(t)
	acc := newSelfEnrollmentSummaryTestAccount(t, priv, sessionPID.String())
	acc.cacheSharedObjectListEntry(selfEnrollmentListEntry("needs"))
	writeSelfEnrollmentCache(t, acc, "needs", "acct-1", "", sobject.SOParticipantRole_SOParticipantRole_READER)

	// The pre-enrollment summary must report one pending entry.
	first := acc.GetSelfEnrollmentSummary()
	if first == nil || first.count != 1 {
		t.Fatalf("summary before enrollment = %+v, want one pending entry", first)
	}

	// Write the enrollment cache and expect the summary broadcast.
	var ch <-chan struct{}
	acc.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
	})
	writeSelfEnrollmentCache(t, acc, "needs", "acct-1", sessionPID.String(), sobject.SOParticipantRole_SOParticipantRole_READER)
	select {
	case <-ch:
	default:
		t.Fatal("expected account broadcast after enrollment summary cleared")
	}

	// The summary must be empty and loaded after enrollment.
	got := acc.GetSelfEnrollmentSummary()
	if got == nil {
		t.Fatal("expected empty summary")
	}
	if got.count != 0 || got.generationKey != "" || len(got.ids) != 0 {
		t.Fatalf("summary after enrollment = %+v, want empty summary", got)
	}
	if !got.loaded {
		t.Fatal("loaded = false, want true after enrollment cache write")
	}
}

func selfEnrollmentListEntry(id string) *sobject.SharedObjectListEntry {
	return &sobject.SharedObjectListEntry{
		Ref:  sobject.NewSharedObjectRef("spacewave", "acct-1", id, SobjectBlockStoreID(id)),
		Meta: &sobject.SharedObjectMeta{BodyType: "space"},
	}
}

func newSelfEnrollmentSummaryTestAccount(
	t *testing.T,
	priv crypto.PrivKey,
	peerID string,
) *ProviderAccount {
	t.Helper()
	return &ProviderAccount{
		accountID:     "acct-1",
		le:            logrus.New().WithField("test", t.Name()),
		objStore:      hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]()),
		sessionClient: NewSessionClient(http.DefaultClient, "http://example.invalid", DefaultSigningEnvPrefix, priv, peerID),
		soListCtr:     ccontainer.NewCContainer[*sobject.SharedObjectList](nil),
	}
}

func writeSelfEnrollmentCache(
	t *testing.T,
	acc *ProviderAccount,
	soID string,
	entityID string,
	grantPeerID string,
	role sobject.SOParticipantRole,
) {
	t.Helper()
	cache := &api.VerifiedSOStateCache{
		CurrentConfig: &sobject.SharedObjectConfig{
			Participants: []*sobject.SOParticipantConfig{{
				EntityId: entityID,
				Role:     role,
			}},
		},
		KeyEpochs: []*sobject.SOKeyEpoch{{
			Epoch: 1,
		}},
	}
	if grantPeerID != "" {
		cache.KeyEpochs[0].Grants = []*sobject.SOGrant{{PeerId: grantPeerID}}
	}
	if err := acc.writeVerifiedSOStateCache(context.Background(), soID, cache); err != nil {
		t.Fatalf("write cache %s: %v", soID, err)
	}
}
