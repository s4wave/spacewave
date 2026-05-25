package orgstatecache

import (
	"context"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestStoreCachesCloudFetches(t *testing.T) {
	fetcher := &testFetcher{
		info: &api.GetOrgResponse{
			Id:               "org-1",
			DisplayName:      "Org One",
			BillingAccountId: "ba-1",
			Members: []*api.OrgMember{{
				Id:        "member-1",
				SubjectId: "acct-1",
				RoleId:    "org:owner",
				CreatedAt: 123,
				EntityId:  "alice",
			}},
			Spaces: []*api.OrgSpaceEntry{{
				Id:          "space-1",
				DisplayName: "Main Space",
				ObjectType:  "space",
			}},
		},
		invites: &api.ListOrgInvitesResponse{
			Invites: []*api.OrgInviteResponse{{
				Id:        "invite-1",
				Type:      "link",
				Token:     "tok-1",
				Uses:      1,
				MaxUses:   5,
				ExpiresAt: 456,
			}},
		},
		role: "org:owner",
	}
	store := NewStore(nil)

	for range 2 {
		info, invites, roleID, err := store.Get(
			context.Background(),
			"org-1",
			fetcher,
			fetcher.lookupRole,
		)
		if err != nil {
			t.Fatalf("get organization snapshot: %v", err)
		}
		if info.GetDisplayName() != "Org One" {
			t.Fatalf("unexpected org info: %+v", info)
		}
		if len(info.GetMembers()) != 1 || info.GetMembers()[0].GetSubjectId() != "acct-1" {
			t.Fatalf("unexpected org members: %+v", info.GetMembers())
		}
		if info.GetMembers()[0].GetEntityId() != "alice" {
			t.Fatalf("unexpected org member entity id: %+v", info.GetMembers())
		}
		if roleID != "org:owner" {
			t.Fatalf("unexpected org role: %q", roleID)
		}
		if len(invites.GetInvites()) != 1 || invites.GetInvites()[0].GetId() != "invite-1" {
			t.Fatalf("unexpected invites: %+v", invites.GetInvites())
		}
	}

	if fetcher.roleCalls != 1 || fetcher.infoCalls != 1 || fetcher.inviteCalls != 1 {
		t.Fatalf(
			"expected one fetch each, got role=%d info=%d invites=%d",
			fetcher.roleCalls,
			fetcher.infoCalls,
			fetcher.inviteCalls,
		)
	}
}

func TestInvalidateRefetchesSnapshot(t *testing.T) {
	fetcher := &testFetcher{
		info: &api.GetOrgResponse{
			Id:          "org-1",
			DisplayName: "Org One",
			Members: []*api.OrgMember{{
				Id:        "member-1",
				SubjectId: "acct-1",
				RoleId:    "owner",
				CreatedAt: 123,
				EntityId:  "alice",
			}},
		},
		invites: &api.ListOrgInvitesResponse{},
		role:    "owner",
	}
	store := NewStore(nil)

	info, _, _, err := store.Get(context.Background(), "org-1", fetcher, fetcher.lookupRole)
	if err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}
	if len(info.GetMembers()) != 1 || info.GetMembers()[0].GetEntityId() != "alice" {
		t.Fatalf("unexpected initial members: %+v", info.GetMembers())
	}

	fetcher.info.Members[0].EntityId = "bob"
	store.Invalidate("org-1")

	info, _, _, err = store.Get(context.Background(), "org-1", fetcher, fetcher.lookupRole)
	if err != nil {
		t.Fatalf("refetched snapshot: %v", err)
	}
	if len(info.GetMembers()) != 1 || info.GetMembers()[0].GetEntityId() != "bob" {
		t.Fatalf("unexpected refetched members: %+v", info.GetMembers())
	}

	if fetcher.infoCalls != 2 || fetcher.inviteCalls != 2 {
		t.Fatalf(
			"expected snapshot refetch after invalidation, got info=%d invites=%d",
			fetcher.infoCalls,
			fetcher.inviteCalls,
		)
	}
}

func TestStoreReturnsClones(t *testing.T) {
	fetcher := &testFetcher{
		info: &api.GetOrgResponse{
			Id:          "org-1",
			DisplayName: "Org One",
			Members: []*api.OrgMember{{
				Id:       "member-1",
				EntityId: "alice",
			}},
		},
		invites: &api.ListOrgInvitesResponse{
			Invites: []*api.OrgInviteResponse{{Id: "invite-1"}},
		},
		role: "org:owner",
	}
	store := NewStore(nil)

	info, invites, roleID, err := store.Get(context.Background(), "org-1", fetcher, fetcher.lookupRole)
	if err != nil {
		t.Fatalf("initial org snapshot: %v", err)
	}
	info.DisplayName = "mutated"
	info.Members[0].EntityId = "mutated-member"
	invites.Invites[0].Id = "mutated-invite"
	if roleID != "org:owner" {
		t.Fatalf("unexpected role id: %q", roleID)
	}

	info, invites, roleID, err = store.Get(context.Background(), "org-1", fetcher, fetcher.lookupRole)
	if err != nil {
		t.Fatalf("cached org snapshot: %v", err)
	}
	if info.GetDisplayName() != "Org One" {
		t.Fatalf("expected cloned org info, got %+v", info)
	}
	if info.GetMembers()[0].GetEntityId() != "alice" {
		t.Fatalf("expected cloned org member, got %+v", info.GetMembers())
	}
	if invites.GetInvites()[0].GetId() != "invite-1" {
		t.Fatalf("expected cloned invite list, got %+v", invites.GetInvites())
	}
	if roleID != "org:owner" {
		t.Fatalf("unexpected cached role id: %q", roleID)
	}
	if fetcher.roleCalls != 1 || fetcher.infoCalls != 1 || fetcher.inviteCalls != 1 {
		t.Fatalf(
			"expected cached reread without refetch, got role=%d info=%d invites=%d",
			fetcher.roleCalls,
			fetcher.infoCalls,
			fetcher.inviteCalls,
		)
	}
}

func TestStoreSkipsInvitesForNonOwner(t *testing.T) {
	fetcher := &testFetcher{
		info: &api.GetOrgResponse{
			Id:          "org-1",
			DisplayName: "Org One",
		},
		role: "member",
	}
	store := NewStore(nil)

	_, invites, roleID, err := store.Get(context.Background(), "org-1", fetcher, fetcher.lookupRole)
	if err != nil {
		t.Fatalf("get member organization snapshot: %v", err)
	}
	if roleID != "member" {
		t.Fatalf("unexpected role id: %q", roleID)
	}
	if len(invites.GetInvites()) != 0 {
		t.Fatalf("expected empty invites for member role, got %+v", invites.GetInvites())
	}
	if fetcher.inviteCalls != 0 {
		t.Fatalf("expected no invite fetch for member role, got %d", fetcher.inviteCalls)
	}
}

type testFetcher struct {
	info    *api.GetOrgResponse
	invites *api.ListOrgInvitesResponse
	role    string

	infoCalls   int
	inviteCalls int
	roleCalls   int
}

func (f *testFetcher) GetOrganization(context.Context, string) ([]byte, error) {
	f.infoCalls++
	return f.info.MarshalVT()
}

func (f *testFetcher) ListOrgInvites(context.Context, string) ([]byte, error) {
	f.inviteCalls++
	return f.invites.MarshalVT()
}

func (f *testFetcher) lookupRole(context.Context, string) (string, error) {
	f.roleCalls++
	return f.role, nil
}
