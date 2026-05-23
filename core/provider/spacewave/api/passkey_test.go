package provider_spacewave_api

import "testing"

func TestDesktopPasskeyRegisterResultRoundTrip(t *testing.T) {
	msg := &DesktopPasskeyRegisterResult{
		Username:       "alice",
		CredentialJson: `{"id":"cred-1"}`,
		PrfCapable:     true,
		PrfSalt:        "salt-1",
		PrfOutput:      "output-1",
	}
	data, err := msg.MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT: %v", err)
	}
	var roundTrip DesktopPasskeyRegisterResult
	if err := roundTrip.UnmarshalVT(data); err != nil {
		t.Fatalf("UnmarshalVT: %v", err)
	}
	if !msg.EqualVT(&roundTrip) {
		t.Fatal("desktop passkey register result did not round-trip")
	}
}

func TestDesktopPasskeyRegisterRelayResultRoundTrip(t *testing.T) {
	msg := &DesktopPasskeyRegisterRelayResult{
		Nonce: "nonce-123",
		Register: &DesktopPasskeyRegisterResult{
			Username:       "alice",
			CredentialJson: `{"id":"cred-1"}`,
			PrfCapable:     true,
			PrfSalt:        "salt-1",
			PrfOutput:      "output-1",
		},
	}
	data, err := msg.MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT: %v", err)
	}
	var roundTrip DesktopPasskeyRegisterRelayResult
	if err := roundTrip.UnmarshalVT(data); err != nil {
		t.Fatalf("UnmarshalVT: %v", err)
	}
	if !msg.EqualVT(&roundTrip) {
		t.Fatal("desktop passkey register relay result did not round-trip")
	}
}

func TestDesktopPasskeyReauthResultRoundTrip(t *testing.T) {
	msg := &DesktopPasskeyReauthResult{
		EncryptedBlob: "blob-1",
		PrfCapable:    true,
		PrfSalt:       "salt-1",
		AuthParams:    "auth-1",
		PinWrapped:    true,
		PrfOutput:     "output-1",
	}
	data, err := msg.MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT: %v", err)
	}
	var roundTrip DesktopPasskeyReauthResult
	if err := roundTrip.UnmarshalVT(data); err != nil {
		t.Fatalf("UnmarshalVT: %v", err)
	}
	if !msg.EqualVT(&roundTrip) {
		t.Fatal("desktop passkey reauth result did not round-trip")
	}
}

func TestDesktopPasskeyReauthRelayResultRoundTrip(t *testing.T) {
	msg := &DesktopPasskeyReauthRelayResult{
		Nonce: "nonce-123",
		Reauth: &DesktopPasskeyReauthResult{
			EncryptedBlob: "blob-1",
			PrfCapable:    true,
			PrfSalt:       "salt-1",
			AuthParams:    "auth-1",
			PinWrapped:    true,
			PrfOutput:     "output-1",
		},
	}
	data, err := msg.MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT: %v", err)
	}
	var roundTrip DesktopPasskeyReauthRelayResult
	if err := roundTrip.UnmarshalVT(data); err != nil {
		t.Fatalf("UnmarshalVT: %v", err)
	}
	if !msg.EqualVT(&roundTrip) {
		t.Fatal("desktop passkey reauth relay result did not round-trip")
	}
}
