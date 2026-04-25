package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestProcessPendingMailboxEntriesReadOnlySkipsFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected mailbox fetch: %s", r.URL.Path)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.state.info = &api.AccountStateResponse{
		LifecycleState: api.AccountLifecycleState_ACCOUNT_LIFECYCLE_STATE_CANCELED_GRACE_READONLY,
	}

	if err := acc.processPendingMailboxEntries(context.Background(), "so-1"); err != nil {
		t.Fatalf("process pending mailbox entries: %v", err)
	}
}

func TestProcessPendingMailboxEntriesUsesCachedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected mailbox fetch: %s", r.URL.Path)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.setPendingMailboxResponse("so-1", &api.GetMailboxResponse{})

	if err := acc.processPendingMailboxEntries(context.Background(), "so-1"); err != nil {
		t.Fatalf("process pending mailbox entries: %v", err)
	}
}

func TestProcessMailboxEntryUsesCachedResponse(t *testing.T) {
	var postCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			t.Fatalf("unexpected mailbox fetch: %s", r.URL.Path)
		}
		if r.URL.Path != "/api/sobject/so-1/invite-mailbox/process" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		postCount++
		body, err := (&api.ProcessMailboxEntryResponse{Status: "rejected"}).MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	acc := NewTestProviderAccount(t, srv.URL)
	acc.setPendingMailboxResponse("so-1", &api.GetMailboxResponse{
		Entries: []*api.MailboxEntry{{
			Id:        7,
			InviteId:  "inv-1",
			PeerId:    "peer-1",
			Status:    "pending",
			CreatedAt: 123,
		}},
	})

	if err := acc.ProcessMailboxEntry(context.Background(), "so-1", 7, false); err != nil {
		t.Fatalf("process mailbox entry: %v", err)
	}
	if postCount != 1 {
		t.Fatalf("expected 1 process POST, got %d", postCount)
	}
}
