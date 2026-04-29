package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	provider "github.com/s4wave/spacewave/core/provider"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

func markReadOnlyAccount(acc *ProviderAccount) {
	acc.state.info = &api.AccountStateResponse{
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_CANCELED,
		LifecycleState:     api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_CANCELED_GRACE_READONLY,
	}
	acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY
}

func TestBuildOrgSyncRoutineReadOnlySkipsCloudCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected org sync request: %s", r.URL.Path)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	markReadOnlyAccount(acc)

	run, _ := acc.buildOrgSyncRoutine("org-1")
	if err := run(context.Background()); err != nil {
		t.Fatalf("run org sync routine: %v", err)
	}
}

func TestBuildPendingParticipantSyncRoutineReadOnlySkipsCloudCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected pending participant request: %s", r.URL.Path)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	markReadOnlyAccount(acc)

	run, _ := acc.buildPendingParticipantSyncRoutine(pendingParticipantSyncKey{
		soID:      "so-1",
		accountID: "acct-2",
	})
	if err := run(context.Background()); err != nil {
		t.Fatalf("run pending participant sync routine: %v", err)
	}
}

func TestBuildMemberSessionSyncRoutineReadOnlySkipsCloudCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected member session request: %s", r.URL.Path)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	markReadOnlyAccount(acc)

	run, _ := acc.buildMemberSessionSyncRoutine(memberSessionSyncKey{
		soID:          "so-1",
		sessionPeerID: "peer-2",
		accountID:     "acct-2",
		added:         false,
	})
	if err := run(context.Background()); err != nil {
		t.Fatalf("run member session sync routine: %v", err)
	}
}

func TestBootstrapOrgSharedObjectsReadOnlySkipsCloudCalls(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected org bootstrap request: %s", r.URL.Path)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	markReadOnlyAccount(acc)
	acc.orgListValid = true
	acc.orgList = []*api.OrgResponse{{
		Id:   "org-1",
		Role: "owner",
	}}
	acc.soListCtr.SetValue(&sobject.SharedObjectList{})

	acc.bootstrapOrgSharedObjects(context.Background())
}

func TestBootstrapOrgSharedObjectsConflictSkipsMount(t *testing.T) {
	var createCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/org/org-1":
			_, _ = w.Write(mustMarshalVT(t, &api.GetOrgResponse{
				Id:          "org-1",
				DisplayName: "Org One",
				Members: []*api.OrgMember{{
					SubjectId: "test-account",
					RoleId:    "org:owner",
				}},
			}))
		case "/api/sobject/org-1/create":
			createCalls++
			w.WriteHeader(http.StatusConflict)
		default:
			t.Fatalf("unexpected org bootstrap request: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.state.status = provider.ProviderAccountStatus_ProviderAccountStatus_READY
	acc.state.info = &api.AccountStateResponse{
		SubscriptionStatus: s4wave_provider_spacewave.BillingStatus_BillingStatus_ACTIVE,
	}
	acc.orgListValid = true
	acc.orgList = []*api.OrgResponse{{
		Id:   "org-1",
		Role: "org:owner",
	}}
	acc.soListCtr.SetValue(&sobject.SharedObjectList{})

	acc.bootstrapOrgSharedObjects(context.Background())

	if createCalls != 1 {
		t.Fatalf("expected one create attempt, got %d", createCalls)
	}
}
