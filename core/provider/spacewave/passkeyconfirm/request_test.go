package passkeyconfirm

import (
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestMarshalRequest(t *testing.T) {
	body, err := MarshalRequest(&Request{
		CredentialJSON:   `{"id":"cred-1"}`,
		Username:         "new-user",
		WrappedEntityKey: "ZW50aXR5",
		EntityPeerID:     "entity-peer",
		SessionPeerID:    "session-peer",
		PinWrapped:       true,
		PrfCapable:       true,
		PrfSalt:          "salt",
		AuthParams:       "auth",
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var got api.PasskeyConfirmRequest
	if err := got.UnmarshalVT(body); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	if got.GetCredentialJson() != `{"id":"cred-1"}` {
		t.Fatalf("credential json = %q", got.GetCredentialJson())
	}
	if got.GetUsername() != "new-user" {
		t.Fatalf("username = %q", got.GetUsername())
	}
	if got.GetWrappedEntityKey() != "ZW50aXR5" {
		t.Fatalf("wrapped entity key = %q", got.GetWrappedEntityKey())
	}
	if got.GetEntityPeerId() != "entity-peer" {
		t.Fatalf("entity peer id = %q", got.GetEntityPeerId())
	}
	if got.GetSessionPeerId() != "session-peer" {
		t.Fatalf("session peer id = %q", got.GetSessionPeerId())
	}
	if !got.GetPinWrapped() || !got.GetPrfCapable() {
		t.Fatalf("passkey flags = pinWrapped:%v prfCapable:%v", got.GetPinWrapped(), got.GetPrfCapable())
	}
	if got.GetPrfSalt() != "salt" || got.GetAuthParams() != "auth" {
		t.Fatalf("passkey params = salt:%q auth:%q", got.GetPrfSalt(), got.GetAuthParams())
	}
}

func TestParseDesktopResponse(t *testing.T) {
	body, err := (&api.PasskeyConfirmResponse{
		AccountId:     "acct-123",
		SessionPeerId: "session-456",
	}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	resp, err := ParseDesktopResponse(body)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.AccountID != "acct-123" {
		t.Fatalf("account id = %q", resp.AccountID)
	}
	if resp.SessionPeerID != "session-456" {
		t.Fatalf("session peer id = %q", resp.SessionPeerID)
	}
}

func TestHTTPContractConstants(t *testing.T) {
	if Method != "POST" {
		t.Fatalf("method = %q", Method)
	}
	if Path != "/api/auth/passkey/confirm" {
		t.Fatalf("path = %q", Path)
	}
	if ContentType != "application/octet-stream" {
		t.Fatalf("content type = %q", ContentType)
	}
}
