package session_lock

import (
	"bytes"
	crypto_rand "crypto/rand"
	"runtime"
	"testing"

	kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	"github.com/s4wave/spacewave/db/object"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/crypto"
)

func skipGoScriptScryptCost(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "js" {
		t.Skip("full-cost scrypt PIN lock test is too slow under GoScript")
	}
}

func TestDeriveStorageKey(t *testing.T) {
	// Generate a volume private key for storage-key derivation.
	priv, _, err := crypto.GenerateEd25519Key(crypto_rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Derive the session storage key from the volume private key.
	key1, err := DeriveStorageKey(priv)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the derived storage key contains key material.
	if key1 == [32]byte{} {
		t.Fatal("derived key is all zeros")
	}

	// Same key produces same result.
	key2, err := DeriveStorageKey(priv)
	if err != nil {
		t.Fatal(err)
	}

	// Verify repeated derivation produces the same storage key.
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

	// Verify a different volume private key produces a different storage key.
	if key1 == key3 {
		t.Fatal("different keys produced same result")
	}
}

func TestAutoUnlockRoundTrip(t *testing.T) {
	// Generate the volume private key for the auto-unlock round trip.
	priv, _, err := crypto.GenerateEd25519Key(crypto_rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Derive the storage key used to protect the session private key.
	storageKey, err := DeriveStorageKey(priv)
	if err != nil {
		t.Fatal(err)
	}

	// Encrypt the session private key with the storage key.
	plaintext := []byte("test session private key PEM data")
	encrypted, err := EncryptAutoUnlock(storageKey, plaintext)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the encrypted session bytes have the expected AES-GCM overhead.
	if bytes.Equal(encrypted, plaintext) {
		t.Fatal("encrypted data matches plaintext")
	}
	if got, want := len(encrypted)-len(plaintext), 12+16; got != want {
		t.Fatalf("encrypted overhead = %d, want %d", got, want)
	}

	// Decrypt the session private key with the original storage key.
	decrypted, err := DecryptAutoUnlock(storageKey, encrypted)
	if err != nil {
		t.Fatal(err)
	}

	// Verify auto-unlock restores the original session bytes.
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatal("decrypted data does not match original")
	}
}

func TestAutoUnlockWrongKey(t *testing.T) {
	// Generate distinct volume private keys for the wrong-key check.
	priv1, _, err := crypto.GenerateEd25519Key(crypto_rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	priv2, _, err := crypto.GenerateEd25519Key(crypto_rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Derive storage keys from both volume private keys.
	key1, err := DeriveStorageKey(priv1)
	if err != nil {
		t.Fatal(err)
	}
	key2, err := DeriveStorageKey(priv2)
	if err != nil {
		t.Fatal(err)
	}

	// Encrypt the session private key with the first storage key.
	plaintext := []byte("test session private key PEM data")
	encrypted, err := EncryptAutoUnlock(key1, plaintext)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the second storage key cannot decrypt the session private key.
	_, err = DecryptAutoUnlock(key2, encrypted)
	if err == nil {
		t.Fatal("expected error decrypting with wrong key")
	}
}

func TestPINLockRoundTrip(t *testing.T) {
	// Require a runtime that can complete full-cost session PIN derivation.
	skipGoScriptScryptCost(t)

	// Prepare the session private key and PIN for the lock round trip.
	plaintext := []byte("test session private key PEM data for PIN lock")
	pin := []byte("123456")

	// Create the PIN-encrypted session credential and derivation parameters.
	encPriv, encSymKey, config, err := CreatePINLock(plaintext, pin)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the PIN lock encrypts the private key and records its salt.
	if bytes.Equal(encPriv, plaintext) {
		t.Fatal("encrypted privkey matches plaintext")
	}
	if config == nil {
		t.Fatal("config is nil")
	}
	if len(config.Salt) != 16 {
		t.Fatalf("expected 16-byte salt, got %d", len(config.Salt))
	}

	// Unlock the session credential with the original PIN.
	decrypted, err := UnlockPIN(encPriv, encSymKey, config, pin)
	if err != nil {
		t.Fatal(err)
	}

	// Verify PIN unlock restores the original session private key.
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatal("decrypted data does not match original")
	}
}

func TestPINLockWrongPIN(t *testing.T) {
	// Require a runtime that can complete full-cost session PIN derivation.
	skipGoScriptScryptCost(t)

	// Prepare distinct PINs for the session unlock rejection check.
	plaintext := []byte("test session private key PEM data")
	pin := []byte("123456")
	wrongPin := []byte("654321")

	// Create the session credential protected by the original PIN.
	encPriv, encSymKey, config, err := CreatePINLock(plaintext, pin)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the wrong PIN cannot unlock the session credential.
	_, err = UnlockPIN(encPriv, encSymKey, config, wrongPin)
	if err == nil {
		t.Fatal("expected error with wrong PIN")
	}
}

func TestLockConfigMarshalRoundTrip(t *testing.T) {
	// Prepare the session PIN derivation parameters for serialization.
	config := &LockConfig{
		ScryptN: 18,
		Salt:    []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	}

	// Encode the PIN derivation parameters for storage.
	data, err := config.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	// Decode the stored PIN derivation parameters.
	config2 := &LockConfig{}
	if err := config2.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}

	// Verify serialization preserves the scrypt cost and salt.
	if config2.ScryptN != config.ScryptN {
		t.Fatalf("scryptN mismatch: got %d, want %d", config2.ScryptN, config.ScryptN)
	}
	if !bytes.Equal(config2.Salt, config.Salt) {
		t.Fatal("salt mismatch")
	}
}

func TestWritePINLockReplaysIdentically(t *testing.T) {
	// Prepare a runner that captures the persisted PIN credential and lock mode.
	type result struct {
		encPriv    []byte
		encSymKey  []byte
		configData []byte
		mode       SessionLockMode
	}
	run := func(t *testing.T, injectFault bool) (result, *kvtest.FaultStore) {
		// Create the session credential store with optional commit fault injection.
		t.Helper()
		ctx := t.Context()
		backend := store_kvtx_inmem.NewStore()
		var store object.ObjectStore = backend
		var faultStore *kvtest.FaultStore
		if injectFault {
			faultStore = kvtest.NewFaultStore(backend, kvtest.FaultBeforeCommit)
			store = faultStore
		}

		// Write the same PIN credential through either transaction path.
		config := &LockConfig{ScryptN: 18, Salt: []byte("0123456789abcdef")}
		if err := WritePINLock(
			ctx,
			store,
			"replay-session",
			[]byte("encrypted-private-key"),
			[]byte("encrypted-symmetric-key"),
			config,
		); err != nil {
			t.Fatal(err)
		}

		// Read the persisted PIN credential from the underlying store.
		encPriv, encSymKey, storedConfig, err := ReadPINLockFiles(
			ctx,
			backend,
			"replay-session",
		)
		if err != nil {
			t.Fatal(err)
		}

		// Encode the persisted PIN parameters for comparison between transaction paths.
		configData, err := storedConfig.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}

		// Read the session lock mode after the PIN credential write.
		mode, err := ReadLockMode(ctx, backend, "replay-session")
		if err != nil {
			t.Fatal(err)
		}
		return result{
			encPriv:    encPriv,
			encSymKey:  encSymKey,
			configData: configData,
			mode:       mode,
		}, faultStore
	}

	// Run the PIN credential write with and without a pre-commit fault.
	want, _ := run(t, false)
	got, faultStore := run(t, true)

	// Verify transaction replay preserves the PIN credential and lock mode.
	if !bytes.Equal(got.encPriv, want.encPriv) {
		t.Fatalf("encrypted private key = %q, want %q", got.encPriv, want.encPriv)
	}
	if !bytes.Equal(got.encSymKey, want.encSymKey) {
		t.Fatalf("encrypted symmetric key = %q, want %q", got.encSymKey, want.encSymKey)
	}
	if !bytes.Equal(got.configData, want.configData) {
		t.Fatalf("lock config = %x, want %x", got.configData, want.configData)
	}
	if got.mode != want.mode {
		t.Fatalf("lock mode = %v, want %v", got.mode, want.mode)
	}

	// Verify the fault causes two transactions and one delegated commit.
	if got := faultStore.Opened(); got != 2 {
		t.Fatalf("opened transactions = %d, want 2", got)
	}
	if got := faultStore.DelegatedCommits(); got != 1 {
		t.Fatalf("delegated commits = %d, want 1", got)
	}
}

// TestLockConfigRoundTrip proves the persisted PIN-lock configuration
// survives a schema-codec encode and decode with its fields intact.
func TestLockConfigRoundTrip(t *testing.T) {
	// Encode the session PIN derivation parameters with the schema codec.
	config := &LockConfig{ScryptN: 18, Salt: []byte("0123456789abcdef")}
	data, err := config.MarshalVT()
	if err != nil {
		t.Fatalf("marshal lock config: %v", err)
	}

	// Decode the stored PIN derivation parameters into a fresh record.
	decoded := &LockConfig{}
	if err := decoded.UnmarshalVT(data); err != nil {
		t.Fatalf("unmarshal lock config: %v", err)
	}

	// Verify the schema codec preserves the scrypt cost and salt.
	if decoded.ScryptN != config.ScryptN {
		t.Fatalf("scrypt N = %d, want %d", decoded.ScryptN, config.ScryptN)
	}
	if !bytes.Equal(decoded.Salt, config.Salt) {
		t.Fatalf("salt = %x, want %x", decoded.Salt, config.Salt)
	}
}
