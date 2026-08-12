package provider_spacewave_handoff

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"strings"
	"testing"

	"golang.org/x/crypto/hkdf"

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

func TestDecryptDeviceEncrypted(t *testing.T) {
	devicePrivate, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("entity-private-key")
	blob := encryptDeviceKeyForTest(t, devicePrivate.PublicKey(), plaintext)
	got, err := DecryptDeviceEncrypted(devicePrivate.Bytes(), blob)
	if err != nil {
		t.Fatalf("DecryptDeviceEncrypted() error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("DecryptDeviceEncrypted() = %q, want %q", got, plaintext)
	}
}

func TestDecryptDesktopDeviceEncrypted(t *testing.T) {
	devicePrivate, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("desktop-entity-private-key")
	blob := encryptDeviceKeyForTest(t, devicePrivate.PublicKey(), plaintext)
	encoded, err := base64.StdEncoding.DecodeString(blob)
	if err != nil {
		t.Fatal(err)
	}
	var encrypted api.DeviceEncryptedKey
	if err := encrypted.UnmarshalVT(encoded); err != nil {
		t.Fatal(err)
	}
	legacyEnvelope := `{"ephemeralPublicKey":"` + base64.StdEncoding.EncodeToString(encrypted.GetEphemeralPublicKey()) +
		`","iv":"` + base64.StdEncoding.EncodeToString(encrypted.GetNonce()) +
		`","ciphertext":"` + base64.StdEncoding.EncodeToString(encrypted.GetCiphertext()) + `"}`
	got, err := decryptDesktopDeviceEncrypted(devicePrivate.Bytes(), legacyEnvelope)
	if err != nil {
		t.Fatalf("decryptDesktopDeviceEncrypted() error = %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("decryptDesktopDeviceEncrypted() = %q, want %q", got, plaintext)
	}
}

func TestDecryptDesktopDeviceEncryptedRejectsOversizedFields(t *testing.T) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		envelope string
	}{
		{name: "envelope", envelope: strings.Repeat("A", base64.StdEncoding.EncodedLen(deviceEncryptedKeyMaxLen)+257)},
		{name: "ephemeral public key", envelope: `{"ephemeralPublicKey":"` + base64.StdEncoding.EncodeToString(make([]byte, 33)) + `","iv":"","ciphertext":""}`},
		{name: "nonce", envelope: `{"ephemeralPublicKey":"","iv":"` + base64.StdEncoding.EncodeToString(make([]byte, 13)) + `","ciphertext":""}`},
		{name: "ciphertext", envelope: `{"ephemeralPublicKey":"","iv":"","ciphertext":"` + strings.Repeat("A", base64.StdEncoding.EncodedLen(deviceEncryptedKeyMaxLen)+4) + `"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := decryptDesktopDeviceEncrypted(privateKey.Bytes(), test.envelope); err == nil {
				t.Fatal("decryptDesktopDeviceEncrypted() unexpectedly succeeded")
			}
		})
	}
}

func TestDecryptDeviceEncryptedRejectsMalformedInput(t *testing.T) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	validPeer, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		key  []byte
		msg  *api.DeviceEncryptedKey
	}{
		{name: "private key", key: []byte{1}, msg: &api.DeviceEncryptedKey{}},
		{name: "ephemeral public key", key: privateKey.Bytes(), msg: &api.DeviceEncryptedKey{Nonce: make([]byte, 12), Ciphertext: make([]byte, 16)}},
		{name: "nonce", key: privateKey.Bytes(), msg: &api.DeviceEncryptedKey{EphemeralPublicKey: validPeer.PublicKey().Bytes(), Nonce: make([]byte, 11), Ciphertext: make([]byte, 16)}},
		{name: "short ciphertext", key: privateKey.Bytes(), msg: &api.DeviceEncryptedKey{EphemeralPublicKey: validPeer.PublicKey().Bytes(), Nonce: make([]byte, 12), Ciphertext: make([]byte, 15)}},
		{name: "oversized ciphertext", key: privateKey.Bytes(), msg: &api.DeviceEncryptedKey{EphemeralPublicKey: validPeer.PublicKey().Bytes(), Nonce: make([]byte, 12), Ciphertext: make([]byte, deviceEncryptedKeyMaxLen+1)}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, err := test.msg.MarshalVT()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := DecryptDeviceEncrypted(test.key, base64.StdEncoding.EncodeToString(encoded)); err == nil {
				t.Fatal("DecryptDeviceEncrypted() unexpectedly succeeded")
			}
		})
	}
	if _, err := DecryptDeviceEncrypted(privateKey.Bytes(), strings.Repeat("A", base64.StdEncoding.EncodedLen(deviceEncryptedKeyMaxLen)+4)); err == nil {
		t.Fatal("DecryptDeviceEncrypted() accepted an oversized envelope")
	}
}

func encryptDeviceKeyForTest(t *testing.T, publicKey *ecdh.PublicKey, plaintext []byte) string {
	t.Helper()
	ephemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := ephemeral.ECDH(publicKey)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, shared, make([]byte, 32), []byte(deviceEncryptInfo)), key); err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	msg := &api.DeviceEncryptedKey{
		EphemeralPublicKey: ephemeral.PublicKey().Bytes(),
		Nonce:              nonce,
		Ciphertext:         gcm.Seal(nil, nonce, plaintext, nil),
	}
	encoded, err := msg.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(encoded)
}
