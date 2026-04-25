package provider_spacewave

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/net/crypto"
)

// TestSSOCodeExchange_Linked verifies SSOCodeExchange parses a linked account
// response.
func TestSSOCodeExchange_Linked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/api/auth/sso/code/exchange") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		var req api.SSOCodeExchangeRequest
		if err := req.UnmarshalVT(body); err != nil {
			t.Fatalf("unmarshal request: %v", err)
		}
		if req.GetProvider() != "google" {
			t.Errorf("unexpected provider: %s", req.GetProvider())
		}
		if req.GetCode() != "auth-code-123" {
			t.Errorf("unexpected code: %s", req.GetCode())
		}
		if req.GetRedirectUri() != "https://app.example.com/callback" {
			t.Errorf("unexpected redirect_uri: %s", req.GetRedirectUri())
		}

		// No auth headers should be present (unauthenticated endpoint).
		if r.Header.Get("X-Peer-ID") != "" {
			t.Error("X-Peer-ID should not be set on unauthenticated endpoint")
		}

		resp := &api.SSOCodeExchangeResponse{
			Linked:        true,
			AccountId:     "acct-sso-1",
			EntityId:      "alice",
			EncryptedBlob: "base64blob==",
			PinWrapped:    false,
			AuthParams:    "base64params==",
		}
		data, _ := resp.MarshalVT()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewEntityClientDirect(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid)
	resp, err := cli.SSOCodeExchange(context.Background(), "google", "auth-code-123", "https://app.example.com/callback")
	if err != nil {
		t.Fatalf("SSOCodeExchange: %v", err)
	}
	if !resp.GetLinked() {
		t.Fatal("expected linked=true")
	}
	if resp.GetAccountId() != "acct-sso-1" {
		t.Errorf("unexpected account_id: %s", resp.GetAccountId())
	}
	if resp.GetEntityId() != "alice" {
		t.Errorf("unexpected entity_id: %s", resp.GetEntityId())
	}
	if resp.GetEncryptedBlob() != "base64blob==" {
		t.Errorf("unexpected encrypted_blob: %s", resp.GetEncryptedBlob())
	}
	if resp.GetPinWrapped() {
		t.Error("expected pin_wrapped=false")
	}
}

// TestSSOCodeExchange_Unlinked verifies SSOCodeExchange parses an unlinked
// response.
func TestSSOCodeExchange_Unlinked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := &api.SSOCodeExchangeResponse{
			Linked:   false,
			Provider: "github",
			Email:    "user@example.com",
		}
		data, _ := resp.MarshalVT()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewEntityClientDirect(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid)
	resp, err := cli.SSOCodeExchange(context.Background(), "github", "code-456", "https://app/cb")
	if err != nil {
		t.Fatalf("SSOCodeExchange: %v", err)
	}
	if resp.GetLinked() {
		t.Fatal("expected linked=false")
	}
	if resp.GetProvider() != "github" {
		t.Errorf("unexpected provider: %s", resp.GetProvider())
	}
	if resp.GetEmail() != "user@example.com" {
		t.Errorf("unexpected email: %s", resp.GetEmail())
	}
}

// TestSSOCodeExchange_ServerError verifies SSOCodeExchange returns error for
// non-2xx.
func TestSSOCodeExchange_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":"invalid_code","message":"bad oauth code"}`))
	}))
	defer srv.Close()

	priv, pid := generateTestKeypair(t)
	cli := NewEntityClientDirect(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid)
	_, err := cli.SSOCodeExchange(context.Background(), "google", "bad-code", "https://app/cb")
	if err == nil {
		t.Fatal("expected error for 400 status")
	}
}

// TestLinkSSO_Success verifies LinkSSO sends a multi-sig request with SSOLinkAction.
func TestLinkSSO_Success(t *testing.T) {
	priv, pid := generateTestKeypair(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		expectedPath := "/api/account/acct-sso-1/sso/link"
		if r.URL.Path != expectedPath {
			t.Errorf("unexpected path: got %s, want %s", r.URL.Path, expectedPath)
		}
		if r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Errorf("unexpected content type: %s", r.Header.Get("Content-Type"))
		}

		body, _ := io.ReadAll(r.Body)
		req := &api.MultiSigRequest{}
		if err := req.UnmarshalVT(body); err != nil {
			t.Fatalf("unmarshal multi-sig request: %v", err)
		}
		if len(req.GetEnvelope()) == 0 {
			t.Fatal("envelope is empty")
		}
		if len(req.GetSignatures()) != 1 {
			t.Fatalf("expected 1 signature, got %d", len(req.GetSignatures()))
		}
		if req.GetSignatures()[0].GetPeerId() != pid.String() {
			t.Errorf("unexpected signer: %s", req.GetSignatures()[0].GetPeerId())
		}

		env := &api.MultiSigActionEnvelope{}
		if err := env.UnmarshalVT(req.GetEnvelope()); err != nil {
			t.Fatalf("unmarshal envelope: %v", err)
		}
		if env.GetAccountId() != "acct-sso-1" {
			t.Errorf("unexpected envelope account_id: %s", env.GetAccountId())
		}
		if env.GetKind() != api.MultiSigActionKind_MULTI_SIG_ACTION_KIND_SSO_LINK {
			t.Errorf("unexpected envelope kind: %v", env.GetKind())
		}
		if env.GetMethod() != http.MethodPost {
			t.Errorf("unexpected envelope method: %s", env.GetMethod())
		}
		if env.GetPath() != expectedPath {
			t.Errorf("unexpected envelope path: %s", env.GetPath())
		}

		// Decode SSOLinkAction from envelope payload (proto binary).
		action := &api.SSOLinkAction{}
		if err := action.UnmarshalVT(env.GetPayload()); err != nil {
			t.Fatalf("unmarshal sso link action: %v", err)
		}
		if action.GetProvider() != "google" {
			t.Errorf("unexpected provider: %s", action.GetProvider())
		}
		if action.GetCode() != "oauth-code" {
			t.Errorf("unexpected code: %s", action.GetCode())
		}
		if action.GetPeerId() == "" {
			t.Error("peer_id should not be empty")
		}
		if len(action.GetEncryptedPrivkey()) == 0 {
			t.Error("encrypted_privkey should not be empty")
		}

		// Verify signature over the envelope bytes.
		pub := priv.GetPublic()
		payload := BuildMultiSigPayload(req.GetSignatures()[0].GetSignedAt(), req.GetEnvelope())
		ok, err := pub.Verify(payload, req.GetSignatures()[0].GetSignature())
		if err != nil {
			t.Fatalf("verify signature: %v", err)
		}
		if !ok {
			t.Fatal("signature verification failed")
		}

		respBody, err := (&api.MultiSigActionResponse{
			Result: &api.MultiSigActionResponse_SsoLink{
				SsoLink: &api.SsoLinkResult{Linked: true, Provider: "google"},
			},
		}).MarshalVT()
		if err != nil {
			t.Fatalf("marshal response: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(respBody)
	}))
	defer srv.Close()

	cli := NewEntityClientDirect(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid)

	action := &api.SSOLinkAction{
		Provider:         "google",
		Code:             "oauth-code",
		RedirectUri:      "https://app/cb",
		EncryptedPrivkey: []byte("encrypted-pem-data"),
		PeerId:           "12D3KooWTest",
		AuthParams:       []byte("params"),
	}

	result, err := cli.LinkSSO(context.Background(), "acct-sso-1", action, []crypto.PrivKey{priv}, []string{pid.String()})
	if err != nil {
		t.Fatalf("LinkSSO: %v", err)
	}
	if !result.GetLinked() || result.GetProvider() != "google" {
		t.Fatalf("unexpected SSO link result: %+v", result)
	}
}

// TestLinkSSO_ServerError verifies LinkSSO returns error for non-2xx.
func TestLinkSSO_ServerError(t *testing.T) {
	priv, pid := generateTestKeypair(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":"insufficient_signatures","message":"need 2 sigs"}`))
	}))
	defer srv.Close()

	cli := NewEntityClientDirect(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid)
	action := &api.SSOLinkAction{
		Provider: "google",
		Code:     "code",
		PeerId:   "peer",
	}
	_, err := cli.LinkSSO(context.Background(), "acct-1", action, []crypto.PrivKey{priv}, []string{pid.String()})
	if err == nil {
		t.Fatal("expected error for 403 status")
	}
}
