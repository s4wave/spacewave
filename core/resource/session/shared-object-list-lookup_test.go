package resource_session

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ccontainer"
	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

type testSOListProvider struct {
	ctr       *ccontainer.CContainer[*sobject.SharedObjectList]
	refreshed int
	entry     *sobject.SharedObjectListEntry
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

func TestLookupSharedObjectListEntryRefreshesFeature(t *testing.T) {
	entry := &sobject.SharedObjectListEntry{
		Ref: sobject.NewSharedObjectRef("spacewave", "account-1", "so-1", "so-1"),
	}
	provider := &testSOListProvider{
		ctr:   ccontainer.NewCContainer[*sobject.SharedObjectList](&sobject.SharedObjectList{}),
		entry: entry,
	}

	got, err := (&SessionResource{}).lookupSharedObjectListEntry(
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
	provider := &testSOListProvider{
		ctr: ccontainer.NewCContainer[*sobject.SharedObjectList](&sobject.SharedObjectList{}),
		entry: &sobject.SharedObjectListEntry{
			Ref: sobject.NewSharedObjectRef("spacewave", "account-1", "other-so", "other-so"),
		},
	}
	res := NewSessionResource(nil, nil, &testInviteSession{provider: provider})

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
	acc := &testAcceptedCloudInviteAccount{}

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
	acc := &testAcceptedCloudInviteAccount{err: errors.New("refresh boom")}

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
