package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/aperturerobotics/util/keyed"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestPublishCreatedOrganizationUpsertsValidCacheAndClones(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")
	acc.orgListValid = true
	acc.orgList = []*api.OrgResponse{
		{Id: "org-1", DisplayName: "Old Org", Role: "org:member", SpaceIds: []string{"old-space"}},
		{Id: "org-2", DisplayName: "Other Org", Role: "org:owner", SpaceIds: []string{"other-space"}},
	}

	created := &api.OrgResponse{
		Id:          "org-1",
		DisplayName: "Created Org",
		Role:        "org:owner",
		SpaceIds:    []string{"new-space"},
	}
	acc.PublishCreatedOrganization(created)
	created.DisplayName = "mutated create response"
	created.Role = "org:member"
	created.SpaceIds[0] = "mutated-space"

	list, err := acc.GetOrganizationList(context.Background())
	if err != nil {
		t.Fatalf("get org list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected cached list to preserve two orgs, got %d: %#v", len(list), list)
	}
	assertOrgSummary(t, list[0], "org-1", "Created Org", "org:owner", []string{"new-space"})
	assertOrgSummary(t, list[1], "org-2", "Other Org", "org:owner", []string{"other-space"})

	cached := acc.GetCachedOrganization("org-1")
	assertOrgSummary(t, cached, "org-1", "Created Org", "org:owner", []string{"new-space"})
	cached.DisplayName = "mutated cached clone"
	cached.Role = "org:member"
	cached.SpaceIds[0] = "mutated-cached-space"

	cachedAgain := acc.GetCachedOrganization("org-1")
	assertOrgSummary(t, cachedAgain, "org-1", "Created Org", "org:owner", []string{"new-space"})
}

func TestPublishCreatedOrganizationLeavesInvalidCacheCold(t *testing.T) {
	var listCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/org/list" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		listCalls++
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(mustMarshalVT(t, &api.ListOrgsResponse{
			Organizations: []*api.OrgResponse{
				{Id: "org-new", DisplayName: "Cloud Authoritative Org", Role: "org:owner"},
				{Id: "org-other", DisplayName: "Cloud Other Org", Role: "org:member"},
			},
		}))
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.orgListValid = false
	acc.PublishCreatedOrganization(&api.OrgResponse{
		Id:          "org-new",
		DisplayName: "Created Org",
		Role:        "org:owner",
	})
	if cached := acc.GetCachedOrganization("org-new"); cached != nil {
		t.Fatalf("invalid org cache should not expose created singleton, got %#v", cached)
	}

	list, err := acc.GetOrganizationList(context.Background())
	if err != nil {
		t.Fatalf("get org list: %v", err)
	}
	if listCalls != 1 {
		t.Fatalf("expected first org-list read to fetch cloud list, got %d calls", listCalls)
	}
	if len(list) != 2 {
		t.Fatalf("expected authoritative cloud list, got %d orgs: %#v", len(list), list)
	}
	assertOrgSummary(t, list[0], "org-new", "Cloud Authoritative Org", "org:owner", nil)
	assertOrgSummary(t, list[1], "org-other", "Cloud Other Org", "org:member", nil)
}

func TestQueueOrganizationSyncGuardsAndRegistersKey(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")

	acc.QueueOrganizationSync("org-before-owner")

	var constructed []string
	acc.orgSyncs = keyed.NewKeyed(func(key string) (keyed.Routine, struct{}) {
		constructed = append(constructed, key)
		return nil, struct{}{}
	})

	acc.QueueOrganizationSync("")
	acc.QueueOrganizationSync("org-1")
	acc.QueueOrganizationSync("org-1")

	if !reflect.DeepEqual(constructed, []string{"org-1"}) {
		t.Fatalf("expected one keyed routine construction for org-1, got %v", constructed)
	}
	keys := acc.orgSyncs.GetKeys()
	if !reflect.DeepEqual(keys, []string{"org-1"}) {
		t.Fatalf("expected org sync key to be registered once, got %v", keys)
	}
}

func assertOrgSummary(
	t *testing.T,
	org *api.OrgResponse,
	id string,
	displayName string,
	role string,
	spaceIDs []string,
) {
	t.Helper()
	if org == nil {
		t.Fatalf("expected org %q, got nil", id)
	}
	if org.GetId() != id {
		t.Fatalf("expected org id %q, got %q", id, org.GetId())
	}
	if org.GetDisplayName() != displayName {
		t.Fatalf("expected org %s display name %q, got %q", id, displayName, org.GetDisplayName())
	}
	if org.GetRole() != role {
		t.Fatalf("expected org %s role %q, got %q", id, role, org.GetRole())
	}
	if !reflect.DeepEqual(org.GetSpaceIds(), spaceIDs) {
		t.Fatalf("expected org %s spaces %v, got %v", id, spaceIDs, org.GetSpaceIds())
	}
}
