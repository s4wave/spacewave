package resource_session

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ccontainer"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// testSOListProvider supplies catalog and targeted lookup responses.
type testSOListProvider struct {
	// ctr holds the current catalog, including its loading state.
	ctr *ccontainer.CContainer[*sobject.SharedObjectList]
	// refreshed counts explicit legacy catalog reloads.
	refreshed int
	// entry is published by a legacy refresh.
	entry *sobject.SharedObjectListEntry
	// lookup handles a supported targeted operation.
	lookup func(context.Context, string) (*sobject.SharedObjectListEntry, error)
}

type testAcceptedCloudInviteAccount struct {
	refreshed int
	err       error
}

func (a *testAcceptedCloudInviteAccount) RefreshSharedObjectList(context.Context) error {
	a.refreshed++
	return a.err
}

func (p *testSOListProvider) CreateSharedObject(
	context.Context,
	string,
	*sobject.SharedObjectMeta,
	string,
	string,
) (*sobject.SharedObjectRef, error) {
	panic("unexpected CreateSharedObject")
}

func (p *testSOListProvider) MountSharedObject(
	context.Context,
	*sobject.SharedObjectRef,
	func(),
) (sobject.SharedObject, func(), error) {
	panic("unexpected MountSharedObject")
}

func (p *testSOListProvider) DeleteSharedObject(context.Context, string) error {
	panic("unexpected DeleteSharedObject")
}

func (p *testSOListProvider) AccessSharedObjectList(
	context.Context,
	func(),
) (ccontainer.Watchable[*sobject.SharedObjectList], func(), error) {
	return p.ctr, func() {}, nil
}

func (p *testSOListProvider) RefreshSharedObjectList(context.Context) error {
	p.refreshed++
	p.ctr.SetValue(&sobject.SharedObjectList{
		SharedObjects: []*sobject.SharedObjectListEntry{p.entry},
	})
	return nil
}

// LookupSharedObject leaves the legacy fixture on catalog refresh.
func (p *testSOListProvider) LookupSharedObject(ctx context.Context, id string) (*sobject.SharedObjectListEntry, error) {
	if p.lookup != nil {
		return p.lookup(ctx, id)
	}
	return nil, sobject.ErrSharedObjectLookupUnsupported
}

type testInviteProviderAccount struct {
	feature sobject.SharedObjectProvider
}

func (a *testInviteProviderAccount) GetProviderAccountFeature(
	_ context.Context,
	feature provider.ProviderFeature,
) (provider.ProviderAccountFeature, error) {
	if feature != provider.ProviderFeature_ProviderFeature_SHARED_OBJECT {
		return nil, errors.New("unexpected provider feature")
	}
	return a.feature, nil
}

type testInviteSession struct {
	provider sobject.SharedObjectProvider
}

func (s *testInviteSession) GetBus() bus.Bus {
	panic("unexpected GetBus")
}

func (s *testInviteSession) GetSessionRef() *session.SessionRef {
	return &session.SessionRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			ProviderId:        "spacewave",
			ProviderAccountId: "account-1",
			Id:                "session-1",
		},
	}
}

func (s *testInviteSession) GetPeerId() peer.ID {
	panic("unexpected GetPeerId")
}

func (s *testInviteSession) GetPrivKey() crypto.PrivKey {
	panic("unexpected GetPrivKey")
}

func (s *testInviteSession) GetProviderAccount() provider.ProviderAccount {
	return &testInviteProviderAccount{feature: s.provider}
}

func (s *testInviteSession) AccessStateAtomStore(
	context.Context,
	string,
) (resource_state.StateAtomStore, error) {
	panic("unexpected AccessStateAtomStore")
}

func (s *testInviteSession) SnapshotStateAtomStoreIDs(context.Context) ([]string, error) {
	panic("unexpected SnapshotStateAtomStoreIDs")
}

func (s *testInviteSession) WatchStateAtomStoreIDs(
	context.Context,
	func([]string) error,
) error {
	panic("unexpected WatchStateAtomStoreIDs")
}

func (s *testInviteSession) GetLockState(
	context.Context,
) (session.SessionLockMode, bool, error) {
	panic("unexpected GetLockState")
}

func (s *testInviteSession) WatchLockState(
	context.Context,
	func(session.SessionLockMode, bool),
) error {
	panic("unexpected WatchLockState")
}

func (s *testInviteSession) UnlockSession(context.Context, []byte) error {
	panic("unexpected UnlockSession")
}

func (s *testInviteSession) SetLockMode(
	context.Context,
	session.SessionLockMode,
	[]byte,
) error {
	panic("unexpected SetLockMode")
}

func (s *testInviteSession) LockSession(context.Context) error {
	panic("unexpected LockSession")
}

// TestLookupSharedObjectListEntryRefreshesFeature checks that a missing entry
// refreshes the list once before the lookup gives up.
func TestLookupSharedObjectListEntryRefreshesFeature(t *testing.T) {
	// Serve the entry only after a refresh.
	entry := &sobject.SharedObjectListEntry{
		Ref: sobject.NewSharedObjectRef("spacewave", "account-1", "so-1", "so-1"),
	}
	provider := &testSOListProvider{
		ctr:   ccontainer.NewCContainer[*sobject.SharedObjectList](&sobject.SharedObjectList{}),
		entry: entry,
	}

	// Look up the entry and check it came from one refresh.
	got, err := lookupSharedObjectListEntry(
		context.Background(),
		provider,
		provider.ctr,
		"so-1",
	)
	if err != nil {
		t.Fatalf("lookupSharedObjectListEntry: %v", err)
	}
	if provider.refreshed != 1 {
		t.Fatalf("expected one refresh, got %d", provider.refreshed)
	}
	if got != entry {
		t.Fatalf("expected refreshed entry, got %#v", got)
	}
}

func TestCreateSpaceInviteReportsListEntryMiss(t *testing.T) {
	// Build a session whose SharedObject list misses the invite target.
	provider := &testSOListProvider{
		ctr: ccontainer.NewCContainer[*sobject.SharedObjectList](&sobject.SharedObjectList{}),
		entry: &sobject.SharedObjectListEntry{
			Ref: sobject.NewSharedObjectRef("spacewave", "account-1", "other-so", "other-so"),
		},
	}
	res := NewSessionResource(nil, nil, &testInviteSession{provider: provider})

	// Report the list-entry miss from CreateSpaceInvite.
	_, err := res.CreateSpaceInvite(context.Background(), &s4wave_session.CreateSpaceInviteRequest{
		SpaceId: "missing-so",
	})
	if !errors.Is(err, sobject.ErrSharedObjectNotFound) {
		t.Fatalf("expected ErrSharedObjectNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "mount invite host: lookup shared object list entry") {
		t.Fatalf("expected contextual invite mount error, got %q", err.Error())
	}
}

func TestAcceptedCloudInviteRefreshesSharedObjectList(t *testing.T) {
	// Use an account that records SharedObject list refreshes.
	acc := &testAcceptedCloudInviteAccount{}

	// Refresh the list and return the accepted invite's SharedObject.
	resp, err := acceptedCloudInviteJoinResponse(context.Background(), acc, "so-1")
	if err != nil {
		t.Fatalf("acceptedCloudInviteJoinResponse: %v", err)
	}
	if acc.refreshed != 1 {
		t.Fatalf("expected one SO list refresh, got %d", acc.refreshed)
	}
	if resp.GetSharedObjectId() != "so-1" ||
		resp.GetResult() != s4wave_session.JoinSpaceViaInviteResult_JoinSpaceViaInviteResult_ACCEPTED {
		t.Fatalf("unexpected accepted response: %#v", resp)
	}
}

func TestAcceptedCloudInviteReportsRefreshError(t *testing.T) {
	// Use an account whose list refresh fails.
	acc := &testAcceptedCloudInviteAccount{err: errors.New("refresh boom")}

	// Report the refresh error after one refresh attempt.
	_, err := acceptedCloudInviteJoinResponse(context.Background(), acc, "so-1")
	if err == nil {
		t.Fatal("expected refresh error")
	}
	if acc.refreshed != 1 {
		t.Fatalf("expected one SO list refresh, got %d", acc.refreshed)
	}
	if !strings.Contains(err.Error(), "refresh shared object list after accepted invite") {
		t.Fatalf("expected contextual refresh error, got %q", err.Error())
	}
}

// TestResourcesListProjectionRejectsReplacedWorker checks that a late result
// cannot overwrite the current reference's type or restore a removed member.
func TestResourcesListProjectionRejectsReplacedWorker(t *testing.T) {
	// Retain the replacement generation and its already-published type.
	entry := &sobject.SharedObjectListEntry{
		Ref: sobject.NewSharedObjectRef("spacewave", "account-1", "so-1", "new-store"),
	}
	list := []*space.SpaceSoListEntry{{Entry: entry}}
	workers := map[string]resourcesListProjectionWorker{"so-1": {ref: entry.Ref, generation: 2}}
	types := map[string]string{"so-1": "new-type"}
	before := resourcesListResponse(list, types)

	// Deliver a result from the canceled generation after its replacement.
	stale := resourcesListProjectionEvent{id: "so-1", generation: 1, objectType: "stale-type"}
	if stale.apply(workers, types) {
		t.Fatal("accepted replaced worker")
	}
	if !resourcesListResponse(list, types).EqualVT(before) {
		t.Fatal("late result changed the visible response")
	}

	// Admit the replacement's next type independently of any other worker.
	current := resourcesListProjectionEvent{id: "so-1", generation: 2, objectType: "current-type"}
	if !current.apply(workers, types) || types["so-1"] != "current-type" {
		t.Fatal("current worker did not publish")
	}

	// Ignore a duplicate type before copying a whole resource list.
	if current.apply(workers, types) {
		t.Fatal("unchanged type produced an update")
	}

	// Refuse an in-flight result after membership removes that worker.
	delete(workers, "so-1")
	delete(types, "so-1")
	if current.apply(workers, types) || len(types) != 0 {
		t.Fatal("removed worker restored a type")
	}
}

// BenchmarkResourcesListResponse measures a 300-member response copy and wire
// encoding so the independent update cost can be estimated per watch.
func BenchmarkResourcesListResponse(b *testing.B) {
	// Keep a stable 300-member catalog with one projected type per member.
	list := make([]*space.SpaceSoListEntry, 300)
	types := make(map[string]string, len(list))
	for i := range list {
		id := strconv.Itoa(i)
		list[i] = &space.SpaceSoListEntry{Entry: &sobject.SharedObjectListEntry{Ref: sobject.NewSharedObjectRef("local", "account", id, id)}}
		types[id] = "canvas"
	}
	previous := resourcesListResponse(list, types)
	b.ReportAllocs()
	b.ResetTimer()

	// Copy, compare and encode the same response the Session sends.
	for b.Loop() {
		response := resourcesListResponse(list, types)
		response.EqualVT(previous)
		if _, err := response.MarshalVT(); err != nil {
			b.Fatal(err)
		}
	}
}

// TestMountLookupBypassesLoadingCatalog resolves a miss without waiting for the
// first catalog value, and never refreshes after a supported route's refusal.
func TestMountLookupBypassesLoadingCatalog(t *testing.T) {
	for _, loaded := range []bool{false, true} {
		// Leave the initial list nil or load a snapshot that lacks this ID.
		ctr := ccontainer.NewCContainer[*sobject.SharedObjectList](nil)
		if loaded {
			ctr.SetValue(&sobject.SharedObjectList{})
		}
		entry := &sobject.SharedObjectListEntry{Ref: sobject.NewSharedObjectRef("spacewave", "account-1", "so-1", "so-1")}
		calls := 0
		p := &testSOListProvider{ctr: ctr, lookup: func(context.Context, string) (*sobject.SharedObjectListEntry, error) {
			calls++
			return entry, nil
		}}

		// One targeted request resolves either miss while the catalog stays held.
		got, err := lookupMountSharedObjectListEntry(t.Context(), p, ctr, "so-1")
		if err != nil || got != entry || calls != 1 || p.refreshed != 0 {
			t.Fatalf("loaded=%v: %v %v, lookups=%d refreshes=%d", loaded, got, err, calls, p.refreshed)
		}

		// A supported refusal never reloads the catalog or invents an entry.
		refused := errors.New("unavailable to this account")
		p.lookup = func(context.Context, string) (*sobject.SharedObjectListEntry, error) { return nil, refused }
		got, err = lookupMountSharedObjectListEntry(t.Context(), p, ctr, "so-1")
		if got != nil || !errors.Is(err, refused) || p.refreshed != 0 {
			t.Fatalf("refusal: %v %v, refreshes=%d", got, err, p.refreshed)
		}

		// The catalog's offline descriptor avoids targeted lookup entirely.
		ctr.SetValue(&sobject.SharedObjectList{SharedObjects: []*sobject.SharedObjectListEntry{entry}})
		got, err = lookupMountSharedObjectListEntry(t.Context(), p, ctr, "so-1")
		if got != entry || err != nil || calls != 1 {
			t.Fatalf("cached hit: %v %v, lookups=%d", got, err, calls)
		}
	}
}
