package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestSyncPendingMailboxEntries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sobject/so-1/invite-mailbox" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("status"); got != "pending" {
			t.Fatalf("unexpected status filter: %s", got)
		}
		body, err := (&api.GetMailboxResponse{
			Entries: []*api.MailboxEntry{{
				Id:        7,
				InviteId:  "inv-1",
				PeerId:    "peer-1",
				Status:    "pending",
				CreatedAt: 123,
				AccountId: "acct-1",
				EntityId:  "alice",
			}},
		}).MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	if err := acc.syncPendingMailboxEntries(context.Background(), "so-1"); err != nil {
		t.Fatalf("sync mailbox entries: %v", err)
	}

	entries, valid := acc.GetPendingMailboxEntriesSnapshot("so-1")
	if !valid {
		t.Fatal("expected mailbox snapshot to be valid")
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 mailbox entry, got %d", len(entries))
	}
	if entries[0].GetInviteId() != "inv-1" || entries[0].GetPeerId() != "peer-1" {
		t.Fatalf("unexpected mailbox entry: %+v", entries[0])
	}
	if entries[0].GetAccountId() != "acct-1" || entries[0].GetEntityId() != "alice" {
		t.Fatalf("unexpected mailbox identity: %+v", entries[0])
	}
}

func TestSyncPendingMailboxEntriesInsufficientRole(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		body, err := (&api.ErrorResponse{
			Code:    "insufficient_role",
			Message: "OWNER role required",
		}).MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	if err := acc.syncPendingMailboxEntries(context.Background(), "so-1"); err != nil {
		t.Fatalf("sync mailbox entries: %v", err)
	}

	entries, valid := acc.GetPendingMailboxEntriesSnapshot("so-1")
	if !valid {
		t.Fatal("expected mailbox snapshot to be valid")
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty mailbox entries, got %d", len(entries))
	}
}

func TestSyncPendingMailboxEntriesReadOnlyCloudError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		body, err := (&api.ErrorResponse{
			Code:    "account_read_only",
			Message: "Account is in a read-only lifecycle state",
		}).MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	if err := acc.syncPendingMailboxEntries(context.Background(), "so-1"); err != nil {
		t.Fatalf("sync mailbox entries: %v", err)
	}

	entries, valid := acc.GetPendingMailboxEntriesSnapshot("so-1")
	if !valid {
		t.Fatal("expected mailbox snapshot to be valid")
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty mailbox entries, got %d", len(entries))
	}
}

func TestGetPendingMailboxEntriesCachedReadOnlySkipsFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected mailbox fetch: %s", r.URL.Path)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.state.info = &api.AccountStateResponse{
		LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_CANCELED_GRACE_READONLY,
	}

	entries, err := acc.GetPendingMailboxEntriesCached(context.Background(), "so-1")
	if err != nil {
		t.Fatalf("get mailbox entries cached: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty mailbox entries, got %d", len(entries))
	}

	snapshot, valid := acc.GetPendingMailboxEntriesSnapshot("so-1")
	if !valid {
		t.Fatal("expected mailbox snapshot to be valid")
	}
	if len(snapshot) != 0 {
		t.Fatalf("expected empty mailbox snapshot, got %d", len(snapshot))
	}
}

func TestRemovePendingMailboxEntry(t *testing.T) {
	acc := &ProviderAccount{}
	acc.setPendingMailboxResponse("so-1", &api.GetMailboxResponse{
		Entries: []*api.MailboxEntry{{
			Id:        9,
			InviteId:  "inv-9",
			PeerId:    "peer-9",
			Status:    "pending",
			CreatedAt: 99,
		}},
	})

	acc.RemovePendingMailboxEntry("so-1", 9)

	entries, valid := acc.GetPendingMailboxEntriesSnapshot("so-1")
	if !valid {
		t.Fatal("expected mailbox snapshot to be valid")
	}
	if len(entries) != 0 {
		t.Fatalf("expected mailbox entry to be removed, got %d entries", len(entries))
	}
}

func TestApplyMailboxEntryEventPreservesIdentity(t *testing.T) {
	acc := &ProviderAccount{}

	acc.ApplyMailboxEntryEvent("so-1", &api.MailboxEntry{
		Id:        11,
		InviteId:  "inv-11",
		PeerId:    "peer-11",
		Status:    "pending",
		CreatedAt: 111,
		AccountId: "acct-11",
		EntityId:  "casey",
	}, 1)

	entries, valid := acc.GetPendingMailboxEntriesSnapshot("so-1")
	if !valid {
		t.Fatal("expected mailbox snapshot to be valid")
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 mailbox entry, got %d", len(entries))
	}
	if entries[0].GetAccountId() != "acct-11" || entries[0].GetEntityId() != "casey" {
		t.Fatalf("unexpected mailbox identity: %+v", entries[0])
	}
}
