package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestConfirmDesktopPasskeyReturnsAccountID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/passkey/confirm" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		resp := &api.PasskeyConfirmResponse{
			AccountId:     "acct-123",
			SessionPeerId: "session-456",
		}
		body, err := resp.MarshalVT()
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		if _, err := w.Write(body); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	resp, err := ConfirmDesktopPasskey(
		context.Background(),
		srv.Client(),
		srv.URL,
		&ConfirmDesktopPasskeyRequest{
			Nonce:            "nonce-1",
			Username:         "new-user",
			CredentialJSON:   `{"id":"cred-1"}`,
			WrappedEntityKey: "ZW50aXR5",
			EntityPeerID:     "entity-peer",
			SessionPeerID:    "session-peer",
		},
	)
	if err != nil {
		t.Fatalf("confirm desktop passkey: %v", err)
	}
	if resp == nil {
		t.Fatal("expected response")
	}
	if resp.AccountID != "acct-123" {
		t.Fatalf("expected account id, got %q", resp.AccountID)
	}
	if resp.SessionPeerID != "session-456" {
		t.Fatalf("expected session peer id, got %q", resp.SessionPeerID)
	}
}
