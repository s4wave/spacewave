package provider_spacewave_handoff

import (
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestParseSSOResult(t *testing.T) {
	frame, err := (&api.WsAuthSessionServerFrame{
		Body: &api.WsAuthSessionServerFrame_SsoCallback{
			SsoCallback: &api.SsoCallbackResult{
				Linked:          true,
				Provider:        "google",
				Email:           "user@example.com",
				Sub:             "sub-123",
				AccountId:       "acct-1",
				EntityId:        "ent-1",
				EncryptedBlob:   "blob-1",
				PinWrapped:      true,
				DeviceEncrypted: true,
			},
		},
	}).MarshalVT()
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	result, err := parseSSOResult(frame)
	if err != nil {
		t.Fatalf("parseSSOResult() error = %v", err)
	}
	if !result.Linked {
		t.Fatal("expected linked result")
	}
	if result.Provider != "google" {
		t.Fatalf("expected provider google, got %q", result.Provider)
	}
	if result.Email != "user@example.com" {
		t.Fatalf("expected email user@example.com, got %q", result.Email)
	}
	if result.Sub != "sub-123" {
		t.Fatalf("expected sub sub-123, got %q", result.Sub)
	}
	if result.AccountID != "acct-1" {
		t.Fatalf("expected accountId acct-1, got %q", result.AccountID)
	}
	if result.EntityID != "ent-1" {
		t.Fatalf("expected entityId ent-1, got %q", result.EntityID)
	}
	if result.EncryptedBlob != "blob-1" {
		t.Fatalf("expected encryptedBlob blob-1, got %q", result.EncryptedBlob)
	}
	if !result.PinWrapped {
		t.Fatal("expected pinWrapped true")
	}
	if !result.DeviceEncrypted {
		t.Fatal("expected deviceEncrypted true")
	}
}

func TestParseEncryptedForDevice(t *testing.T) {
	enc, err := parseEncryptedForDevice(`{
		"ephemeralPublicKey": "epk",
		"iv": "ivv",
		"ciphertext": "ct"
	}`)
	if err != nil {
		t.Fatalf("parseEncryptedForDevice() error = %v", err)
	}
	if enc.EphemeralPublicKey != "epk" {
		t.Fatalf("expected ephemeralPublicKey epk, got %q", enc.EphemeralPublicKey)
	}
	if enc.IV != "ivv" {
		t.Fatalf("expected iv ivv, got %q", enc.IV)
	}
	if enc.Ciphertext != "ct" {
		t.Fatalf("expected ciphertext ct, got %q", enc.Ciphertext)
	}
}
