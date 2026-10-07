package s4wave_appconnector_world_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	s4wave_appconnector "github.com/s4wave/spacewave/sdk/appconnector"
	s4wave_secret "github.com/s4wave/spacewave/sdk/secret"
)

const (
	// connectorKey is the object key of the test AppConnector.
	connectorKey = "connectors/hyperline"
	// statsPath is the admin path that serves statsBody.
	statsPath = "/admin/stats"
	// bigPath is the admin path whose body exceeds maxBodyBytes.
	bigPath = "/admin/big"
	// statsBody is the body served at statsPath.
	statsBody = `{"users":3}`
	// apiToken is the bearer token the fake admin API accepts.
	apiToken = "hyperline-admin-token"
	// maxBodyBytes is the connector's response body limit.
	maxBodyBytes = 32
)

// TestConnectorKeepsSnapshotCurrent checks a good fetch, a revoked token that
// keeps the last good body, and the process ending with its binding.
func TestConnectorKeepsSnapshotCurrent(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Start the connector under a context that stands for its binding.
	h := newConnectorHarness(ctx, t)
	execCtx, revokeBinding := context.WithCancel(ctx)
	defer revokeBinding()
	done := h.execute(execCtx, t, h.sessionPeerID)

	// The snapshot records the good response with its content type.
	good := h.awaitSnapshot(ctx, t, func(reads map[string]*s4wave_appconnector.AppReadSnapshot) bool {
		return reads["stats"].GetFetchedAt() != nil
	})["stats"]
	if string(good.GetBody()) != statsBody || good.GetContentType() != "application/json" || good.GetStatus() != http.StatusOK || good.GetError() != "" {
		t.Fatalf("good read = %+v", good)
	}

	// A revoked token records an error and leaves the last good body.
	h.api.revoked.Store(true)
	failed := h.awaitSnapshot(ctx, t, func(reads map[string]*s4wave_appconnector.AppReadSnapshot) bool {
		return reads["stats"].GetError() != ""
	})["stats"]
	if failed.GetError() != "unexpected status 401" {
		t.Fatalf("error = %q", failed.GetError())
	}
	if string(failed.GetBody()) != statsBody || !failed.GetFetchedAt().AsTime().Equal(good.GetFetchedAt().AsTime()) {
		t.Fatalf("failed read lost the last good response: %+v", failed)
	}

	// Revoking the binding ends the process and the polling.
	revokeBinding()
	if err := <-done; err == nil {
		t.Fatal("process ended without an error")
	}
	requests := h.api.requests.Load()
	time.Sleep(200 * time.Millisecond)
	if got := h.api.requests.Load(); got != requests {
		t.Fatalf("requests after revocation = %d, want %d", got, requests)
	}
}

// TestConnectorLimitsBodySize checks that an oversized body is an error and
// is never stored.
func TestConnectorLimitsBodySize(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Run the connector against a body larger than the limit.
	h := newConnectorHarness(ctx, t)
	h.execute(ctx, t, h.sessionPeerID)

	// The oversized read records an error and stores no body.
	reads := h.awaitSnapshot(ctx, t, func(reads map[string]*s4wave_appconnector.AppReadSnapshot) bool {
		return reads["big"].GetError() != ""
	})
	big := reads["big"]
	if big.GetError() != "response exceeds 32 bytes" || len(big.GetBody()) != 0 {
		t.Fatalf("big read = %+v", big)
	}
}

// TestConnectorCannotReadTokenWithoutGrant checks that a session peer outside
// the token Secret's grants sends no request and records an error.
func TestConnectorCannotReadTokenWithoutGrant(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Run the connector as a peer the token Secret does not grant.
	h := newConnectorHarness(ctx, t)
	_, _, ungrantedPeerID := makePeer(t)
	h.execute(ctx, t, ungrantedPeerID)

	// The denial is recorded and the API is never called.
	reads := h.awaitSnapshot(ctx, t, func(reads map[string]*s4wave_appconnector.AppReadSnapshot) bool {
		return reads["stats"].GetError() != ""
	})
	if !strings.Contains(reads["stats"].GetError(), s4wave_secret.ErrPayloadAccessDenied.Error()) {
		t.Fatalf("error = %q", reads["stats"].GetError())
	}
	if got := h.api.requests.Load(); got != 0 {
		t.Fatalf("requests without a grant = %d, want 0", got)
	}
}
