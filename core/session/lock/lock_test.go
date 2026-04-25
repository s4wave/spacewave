package session_lock

import (
	"bytes"
	"testing"

	crypto_rand "crypto/rand"
	"github.com/s4wave/spacewave/net/crypto"
)

func TestDeriveStorageKey(t *testing.T) {
	priv, _, err := crypto.GenerateEd25519Key(crypto_rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	key1, err := DeriveStorageKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	if key1 == [32]byte{} {
		t.Fatal("derived key is all zeros")
	}

	// Same key produces same result.
	key2, err := DeriveStorageKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	if key1 != key2 {
		t.Fatal("same key produced different results")
	}

	// Different key produces different result.
	priv2, _, err := crypto.GenerateEd25519Key(crypto_rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key3, err := DeriveStorageKey(priv2)
	if err != nil {
		t.Fatal(err)
	}
	if key1 == key3 {
		t.Fatal("different keys produced same result")
	}
}

func TestAutoUnlockRoundTrip(t *testing.T) {
	priv, _, err := crypto.GenerateEd25519Key(crypto_rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	storageKey, err := DeriveStorageKey(priv)
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("test session private key PEM data")
	encrypted, err := EncryptAutoUnlock(storageKey, plaintext)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(encrypted, plaintext) {
		t.Fatal("encrypted data matches plaintext")
	}

	decrypted, err := DecryptAutoUnlock(storageKey, encrypted)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatal("decrypted data does not match original")
	}
}

func TestAutoUnlockWrongKey(t *testing.T) {
	priv1, _, err := crypto.GenerateEd25519Key(crypto_rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	priv2, _, err := crypto.GenerateEd25519Key(crypto_rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	key1, err := DeriveStorageKey(priv1)
	if err != nil {
		t.Fatal(err)
	}
	key2, err := DeriveStorageKey(priv2)
	if err != nil {
		t.Fatal(err)
	}

	plaintext := []byte("test session private key PEM data")
	encrypted, err := EncryptAutoUnlock(key1, plaintext)
	if err != nil {
		t.Fatal(err)
	}

	_, err = DecryptAutoUnlock(key2, encrypted)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestPINLockRoundTrip(t *testing.T) {
	plaintext := []byte("test session private key PEM data for PIN lock")
	pin := []byte("123456")

	encPriv, encSymKey, config, err := CreatePINLock(plaintext, pin)
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Equal(encPriv, plaintext) {
		t.Fatal("encrypted privkey matches plaintext")
	}
	if config == nil {
		t.Fatal("config is nil")
	}
	if len(config.Salt) != 16 {
		t.Fatalf("expected 16-byte salt, got %d", len(config.Salt))
	}

	decrypted, err := UnlockPIN(encPriv, encSymKey, config, pin)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatal("decrypted data does not match original")
	}
}

func TestPINLockWrongPIN(t *testing.T) {
	plaintext := []byte("test session private key PEM data")
	pin := []byte("123456")
	wrongPin := []byte("654321")

	encPriv, encSymKey, config, err := CreatePINLock(plaintext, pin)
	if err != nil {
		t.Fatal(err)
	}

	_, err = UnlockPIN(encPriv, encSymKey, config, wrongPin)
	if err == nil {
		t.Fatal("expected error with wrong PIN")
	}
}

func TestLockConfigMarshalRoundTrip(t *testing.T) {
	config := &LockConfig{
		ScryptN: 18,
		Salt:    []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	}

	data, err := config.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	config2 := &LockConfig{}
	if err := config2.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}

	if config2.ScryptN != config.ScryptN {
		t.Fatalf("scryptN mismatch: got %d, want %d", config2.ScryptN, config.ScryptN)
	}
	if !bytes.Equal(config2.Salt, config.Salt) {
		t.Fatal("salt mismatch")
	}
}
