package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestGetOrganizationSnapshotCachesCloudFetches(t *testing.T) {
	var listCalls, infoCalls, inviteCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		var err error
		switch r.URL.Path {
		case "/api/org/list":
			listCalls++
			body, err = (&api.ListOrgsResponse{
				Organizations: []*api.OrgResponse{{
					Id:          "org-1",
					DisplayName: "Org One",
					Role:        "org:owner",
				}},
			}).MarshalVT()
		case "/api/org/org-1":
			infoCalls++
			body, err = (&api.GetOrgResponse{
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
			}).MarshalVT()
		case "/api/org/org-1/invites":
			inviteCalls++
			body, err = (&api.ListOrgInvitesResponse{
				Invites: []*api.OrgInviteResponse{{
					Id:        "invite-1",
					Type:      "link",
					Token:     "tok-1",
					Uses:      1,
					MaxUses:   5,
					ExpiresAt: 456,
				}},
			}).MarshalVT()
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)

	for range 2 {
		info, invites, roleID, err := acc.GetOrganizationSnapshot(
			context.Background(),
			"org-1",
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

	if listCalls != 1 || infoCalls != 1 || inviteCalls != 1 {
		t.Fatalf(
			"expected one fetch each, got list=%d info=%d invites=%d",
			listCalls,
			infoCalls,
			inviteCalls,
		)
	}
}

func TestInvalidateOrganizationStateRefetchesSnapshot(t *testing.T) {
	var infoCalls, inviteCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		var err error
		switch r.URL.Path {
		case "/api/org/list":
			body, err = (&api.ListOrgsResponse{
				Organizations: []*api.OrgResponse{{
					Id:          "org-1",
					DisplayName: "Org One",
					Role:        "owner",
				}},
			}).MarshalVT()
		case "/api/org/org-1":
			infoCalls++
			entityID := "alice"
			if infoCalls > 1 {
				entityID = "bob"
			}
			body, err = (&api.GetOrgResponse{
				Id:          "org-1",
				DisplayName: "Org One",
				Members: []*api.OrgMember{{
					Id:        "member-1",
					SubjectId: "acct-1",
					RoleId:    "owner",
					CreatedAt: 123,
					EntityId:  entityID,
				}},
			}).MarshalVT()
		case "/api/org/org-1/invites":
			inviteCalls++
			body, err = (&api.ListOrgInvitesResponse{}).MarshalVT()
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)

	info, _, _, err := acc.GetOrganizationSnapshot(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("initial snapshot: %v", err)
	}
	if len(info.GetMembers()) != 1 || info.GetMembers()[0].GetEntityId() != "alice" {
		t.Fatalf("unexpected initial members: %+v", info.GetMembers())
	}

	acc.InvalidateOrganizationState("org-1")

	info, _, _, err = acc.GetOrganizationSnapshot(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("refetched snapshot: %v", err)
	}
	if len(info.GetMembers()) != 1 || info.GetMembers()[0].GetEntityId() != "bob" {
		t.Fatalf("unexpected refetched members: %+v", info.GetMembers())
	}

	if infoCalls != 2 || inviteCalls != 2 {
		t.Fatalf(
			"expected snapshot refetch after invalidation, got info=%d invites=%d",
			infoCalls,
			inviteCalls,
		)
	}
}

func TestGetOrganizationSnapshotReturnsClones(t *testing.T) {
	var listCalls, infoCalls, inviteCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		var err error
		switch r.URL.Path {
		case "/api/org/list":
			listCalls++
			body, err = (&api.ListOrgsResponse{
				Organizations: []*api.OrgResponse{{
					Id:          "org-1",
					DisplayName: "Org One",
					Role:        "org:owner",
				}},
			}).MarshalVT()
		case "/api/org/org-1":
			infoCalls++
			body, err = (&api.GetOrgResponse{
				Id:          "org-1",
				DisplayName: "Org One",
				Members: []*api.OrgMember{{
					Id:       "member-1",
					EntityId: "alice",
				}},
			}).MarshalVT()
		case "/api/org/org-1/invites":
			inviteCalls++
			body, err = (&api.ListOrgInvitesResponse{
				Invites: []*api.OrgInviteResponse{{
					Id: "invite-1",
				}},
			}).MarshalVT()
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)

	info, invites, roleID, err := acc.GetOrganizationSnapshot(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("initial org snapshot: %v", err)
	}
	info.DisplayName = "mutated"
	info.Members[0].EntityId = "mutated-member"
	invites.Invites[0].Id = "mutated-invite"
	if roleID != "org:owner" {
		t.Fatalf("unexpected role id: %q", roleID)
	}

	info, invites, roleID, err = acc.GetOrganizationSnapshot(context.Background(), "org-1")
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
	if listCalls != 1 || infoCalls != 1 || inviteCalls != 1 {
		t.Fatalf(
			"expected cached reread without refetch, got list=%d info=%d invites=%d",
			listCalls,
			infoCalls,
			inviteCalls,
		)
	}
}
