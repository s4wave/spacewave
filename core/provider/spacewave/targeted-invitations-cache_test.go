package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// TestTargetedInvitationsCache checks that the inbox is fetched once per
// invalidation and that a fetch racing an invalidation is not kept as current.
func TestTargetedInvitationsCache(t *testing.T) {
	// Serve an inbox recording the fetch count, holding the third fetch
	// open.
	var hits atomic.Int32
	thirdStarted := make(chan struct{})
	releaseThird := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/account/targeted-invitations" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			return
		}
		hit := hits.Add(1)
		if hit == 3 {
			close(thirdStarted)
			<-releaseThird
		}
		_, _ = w.Write(mustMarshalVT(t, &api.ListTargetedInvitationsResponse{
			Invitations: []*api.TargetedInvitationInfo{{Id: "inv-" + strconv.Itoa(int(hit))}},
		}))
	}))
	defer srv.Close()
	acc := NewTestProviderAccount(t, srv.URL)
	ctx := t.Context()

	// Two reads share one fetch.
	first, waitCh := mustGetTargetedInvitations(ctx, t, acc)
	again, _ := mustGetTargetedInvitations(ctx, t, acc)
	if first != again || hits.Load() != 1 {
		t.Fatalf("expected one shared fetch, got %d fetches", hits.Load())
	}

	// An invalidation wakes the watcher and the next read fetches again.
	acc.InvalidateTargetedInvitations()
	select {
	case <-waitCh:
	default:
		t.Fatal("expected the invalidation to close the wait channel")
	}
	if resp, _ := mustGetTargetedInvitations(ctx, t, acc); inboxID(resp) != "inv-2" {
		t.Fatalf("expected the refetched inbox inv-2, got %s", inboxID(resp))
	}

	// An invalidation during a fetch makes the read fetch once more.
	acc.InvalidateTargetedInvitations()
	done := make(chan *api.ListTargetedInvitationsResponse, 1)
	go func() {
		resp, _ := mustGetTargetedInvitations(ctx, t, acc)
		done <- resp
	}()
	<-thirdStarted
	acc.InvalidateTargetedInvitations()
	close(releaseThird)
	select {
	case resp := <-done:
		if inboxID(resp) != "inv-4" {
			t.Fatalf("expected the inbox fetched after the invalidation, got %s", inboxID(resp))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out reading the inbox")
	}
}

// mustGetTargetedInvitations reads the cached inbox or fails the test.
func mustGetTargetedInvitations(
	ctx context.Context,
	t *testing.T,
	acc *ProviderAccount,
) (*api.ListTargetedInvitationsResponse, <-chan struct{}) {
	resp, waitCh, err := acc.GetTargetedInvitations(ctx)
	if err != nil {
		t.Errorf("get targeted invitations: %v", err)
	}
	return resp, waitCh
}

// inboxID returns the id of the single invitation in resp.
func inboxID(resp *api.ListTargetedInvitationsResponse) string {
	if len(resp.GetInvitations()) != 1 {
		return ""
	}
	return resp.GetInvitations()[0].GetId()
}
