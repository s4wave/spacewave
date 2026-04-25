package provider_spacewave_handoff

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	websocket "github.com/aperturerobotics/go-websocket"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestValidatePasskeyOpenURL(t *testing.T) {
	accountEndpoint := "https://account.spacewave.test"

	if err := validatePasskeyOpenURL(
		"https://account.spacewave.test/passkey/login?nonce=abc",
		accountEndpoint,
	); err != nil {
		t.Fatalf("expected account-hosted open_url to validate, got %v", err)
	}
}

func TestValidatePasskeyOpenURLRejectsLegacyPublicAppURL(t *testing.T) {
	err := validatePasskeyOpenURL(
		"https://spacewave.test/#/auth/passkey/desktop?nonce=abc",
		"https://account.spacewave.test",
	)
	if err == nil {
		t.Fatal("expected legacy public-app open_url to be rejected")
	}
	if !strings.Contains(err.Error(), `account_endpoint host "account.spacewave.test"`) {
		t.Fatalf("expected account_endpoint host error, got %v", err)
	}
}

func TestValidatePasskeyOpenURLRequiresAccountEndpoint(t *testing.T) {
	err := validatePasskeyOpenURL(
		"https://account.spacewave.test/passkey/login?nonce=abc",
		"",
	)
	if err == nil {
		t.Fatal("expected missing account_endpoint to be rejected")
	}
	if !strings.Contains(err.Error(), "valid account_endpoint") {
		t.Fatalf("expected account_endpoint configuration error, got %v", err)
	}
}

func TestValidatePasskeyOpenURLRejectsWrongHost(t *testing.T) {
	err := validatePasskeyOpenURL(
		"https://evil.spacewave.test/passkey/register?nonce=abc",
		"https://account.spacewave.test",
	)
	if err == nil {
		t.Fatal("expected wrong host to be rejected")
	}
	if !strings.Contains(err.Error(), `account_endpoint host "account.spacewave.test"`) {
		t.Fatalf("expected account_endpoint host error, got %v", err)
	}
}

func TestValidatePasskeyOpenURLRejectsWrongScheme(t *testing.T) {
	err := validatePasskeyOpenURL(
		"http://account.spacewave.test/passkey/register?nonce=abc",
		"https://account.spacewave.test",
	)
	if err == nil {
		t.Fatal("expected wrong scheme to be rejected")
	}
	if !strings.Contains(err.Error(), `scheme must be https`) {
		t.Fatalf("expected scheme error, got %v", err)
	}
}

func TestValidatePasskeyOpenURLRejectsMissingHost(t *testing.T) {
	err := validatePasskeyOpenURL(
		"https:///passkey/register?nonce=abc",
		"https://account.spacewave.test",
	)
	if err == nil {
		t.Fatal("expected missing host to be rejected")
	}
	if !strings.Contains(err.Error(), "no host") {
		t.Fatalf("expected missing host error, got %v", err)
	}
}

func TestWaitForDesktopPasskeyRegister(t *testing.T) {
	oldBrowserOpener := browserOpener
	browserOpener = func(rawURL string) error {
		if rawURL != "https://account.spacewave.test/passkey/register?nonce=nonce-123" {
			return errors.Errorf("unexpected browser url %q", rawURL)
		}
		return nil
	}
	defer func() {
		browserOpener = oldBrowserOpener
	}()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/session/ws":
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				t.Fatalf("accept websocket: %v", err)
			}
			defer conn.Close(websocket.StatusNormalClosure, "")

			payload := &api.DesktopPasskeyRegisterRelayResult{
				Nonce: "nonce-123",
				Register: &api.DesktopPasskeyRegisterResult{
					Username:       "alice",
					CredentialJson: `{"id":"cred-1"}`,
					PrfCapable:     true,
					PrfSalt:        "salt-1",
					PrfOutput:      "output-1",
				},
			}
			msg, err := (&api.WsAuthSessionServerFrame{
				Body: &api.WsAuthSessionServerFrame_PasskeyRelay{
					PasskeyRelay: &api.PasskeyRelay{
						Relay: &api.PasskeyRelay_RegisterRelay{
							RegisterRelay: payload,
						},
					},
				},
			}).MarshalVT()
			if err != nil {
				t.Fatalf("marshal relay payload: %v", err)
			}
			if err := conn.Write(context.Background(), websocket.MessageBinary, msg); err != nil {
				t.Fatalf("write relay payload: %v", err)
			}
		case "/api/auth/session/delete":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	result, err := WaitForDesktopPasskeyRegister(
		context.Background(),
		srv.Client(),
		srv.URL,
		"https://account.spacewave.test",
		"nonce-123",
		"ticket-123",
		"https://account.spacewave.test/passkey/register?nonce=nonce-123",
	)
	if err != nil {
		t.Fatalf("wait for desktop passkey register: %v", err)
	}
	if result == nil {
		t.Fatal("expected register result")
	}
	if result.GetUsername() != "alice" {
		t.Fatalf("unexpected username: %q", result.GetUsername())
	}
	if result.GetCredentialJson() != `{"id":"cred-1"}` {
		t.Fatalf("unexpected credential json: %q", result.GetCredentialJson())
	}
	if !result.GetPrfCapable() {
		t.Fatal("expected prf capable result")
	}
	if result.GetPrfSalt() != "salt-1" {
		t.Fatalf("unexpected prf salt: %q", result.GetPrfSalt())
	}
	if result.GetPrfOutput() != "output-1" {
		t.Fatalf("unexpected prf output: %q", result.GetPrfOutput())
	}
}

func TestWaitForDesktopPasskeyReauth(t *testing.T) {
	oldBrowserOpener := browserOpener
	browserOpener = func(rawURL string) error {
		if rawURL != "https://account.spacewave.test/passkey/reauth?nonce=nonce-123" {
			return errors.Errorf("unexpected browser url %q", rawURL)
		}
		return nil
	}
	defer func() {
		browserOpener = oldBrowserOpener
	}()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/session/ws":
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				t.Fatalf("accept websocket: %v", err)
			}
			defer conn.Close(websocket.StatusNormalClosure, "")

			payload := &api.DesktopPasskeyReauthRelayResult{
				Nonce: "nonce-123",
				Reauth: &api.DesktopPasskeyReauthResult{
					EncryptedBlob: "blob-1",
					PrfCapable:    true,
					PrfSalt:       "salt-1",
					AuthParams:    "auth-1",
					PinWrapped:    true,
					PrfOutput:     "output-1",
				},
			}
			msg, err := (&api.WsAuthSessionServerFrame{
				Body: &api.WsAuthSessionServerFrame_PasskeyRelay{
					PasskeyRelay: &api.PasskeyRelay{
						Relay: &api.PasskeyRelay_ReauthRelay{
							ReauthRelay: payload,
						},
					},
				},
			}).MarshalVT()
			if err != nil {
				t.Fatalf("marshal relay payload: %v", err)
			}
			if err := conn.Write(context.Background(), websocket.MessageBinary, msg); err != nil {
				t.Fatalf("write relay payload: %v", err)
			}
		case "/api/auth/session/delete":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	result, err := WaitForDesktopPasskeyReauth(
		context.Background(),
		srv.Client(),
		srv.URL,
		"https://account.spacewave.test",
		"nonce-123",
		"ticket-123",
		"https://account.spacewave.test/passkey/reauth?nonce=nonce-123",
	)
	if err != nil {
		t.Fatalf("wait for desktop passkey reauth: %v", err)
	}
	if result == nil {
		t.Fatal("expected reauth result")
	}
	if result.GetEncryptedBlob() != "blob-1" {
		t.Fatalf("unexpected encrypted blob: %q", result.GetEncryptedBlob())
	}
	if !result.GetPrfCapable() {
		t.Fatal("expected prf capable result")
	}
	if result.GetPrfSalt() != "salt-1" {
		t.Fatalf("unexpected prf salt: %q", result.GetPrfSalt())
	}
	if result.GetAuthParams() != "auth-1" {
		t.Fatalf("unexpected auth params: %q", result.GetAuthParams())
	}
	if !result.GetPinWrapped() {
		t.Fatal("expected pin wrapped result")
	}
	if result.GetPrfOutput() != "output-1" {
		t.Fatalf("unexpected prf output: %q", result.GetPrfOutput())
	}
}
