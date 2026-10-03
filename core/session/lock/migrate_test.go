package session_lock

import (
	"bytes"
	"context"
	"crypto/rand"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
)

func TestCopyCredentialPreservesPINAcrossTransactionRetry(t *testing.T) {
	// Require a runtime that can complete full-cost session PIN derivation.
	skipGoScriptScryptCost(t)

	// Create source and destination stores with a new session private key.
	ctx := t.Context()
	source, destination := store_kvtx_inmem.NewStore(), store_kvtx_inmem.NewStore()
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Encode the session private key for PIN encryption.
	plain, err := keypem.MarshalPrivKeyPem(key)
	if err != nil {
		t.Fatal(err)
	}

	// Create and store the source session PIN credential.
	private, symmetric, config, err := CreatePINLock(plain, []byte("123456"))
	if err != nil {
		t.Fatal(err)
	}
	if err := WritePINLock(ctx, source, "source", private, symmetric, config); err != nil {
		t.Fatal(err)
	}

	// Store the source session recovery envelope and completed setup marker.
	if err := WriteEnvelope(ctx, source, "source", []byte("recovery envelope")); err != nil {
		t.Fatal(err)
	}
	if err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) { return source.NewTransaction(ctx, true) }, func(ctx context.Context, tx kvtx.Tx) error {
		return tx.Set(ctx, MakeKey("source", SuffixSetupDone), []byte{1})
	}); err != nil {
		t.Fatal(err)
	}

	// Copy the PIN credential through a destination transaction that fails before commit.
	fault := kvtest.NewFaultStore(destination, kvtest.FaultBeforeCommit)
	if err := CopyCredential(ctx, source, fault, "source", "destination", key, [32]byte{3}); err != nil {
		t.Fatal(err)
	}

	// Verify credential copying retries as one committed transaction.
	if fault.Opened() != 2 || fault.DelegatedCommits() != 1 {
		t.Fatal("credential copy did not replay as one transaction")
	}

	// Read the copied PIN credential from the destination store.
	gotPrivate, gotSymmetric, gotConfig, err := ReadPINLockFiles(ctx, destination, "destination")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the copied PIN credential unlocks and retains its encrypted records.
	got, err := UnlockPIN(gotPrivate, gotSymmetric, gotConfig, []byte("123456"))
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("PIN did not survive: %v", err)
	}
	if !bytes.Equal(gotPrivate, private) || !bytes.Equal(gotSymmetric, symmetric) || !gotConfig.EqualVT(config) {
		t.Fatal("PIN credential was replaced")
	}

	// Repeat the credential copy to verify the existing destination is accepted.
	if err := CopyCredential(ctx, source, destination, "source", "destination", key, [32]byte{3}); err != nil {
		t.Fatal(err)
	}

	// Open a destination read transaction to inspect the retained session metadata.
	read, err := destination.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()

	// Verify the credential copy retains the recovery envelope and setup marker.
	for suffix, expected := range map[string][]byte{string(SuffixEnvelope): []byte("recovery envelope"), string(SuffixSetupDone): {1}} {
		got, found, err := read.Get(ctx, MakeKey("destination", []byte(suffix)))
		if err != nil || !found || !bytes.Equal(got, expected) {
			t.Fatalf("missing retained field %s: %v", suffix, err)
		}
	}
}

func TestCopyCredentialRewrapsAutoUnlockAndRejectsConflictingKey(t *testing.T) {
	// Create source and destination stores with a new session private key.
	ctx := t.Context()
	source, destination := store_kvtx_inmem.NewStore(), store_kvtx_inmem.NewStore()
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Encode and encrypt the source session credential for auto-unlock.
	plain, err := keypem.MarshalPrivKeyPem(key)
	if err != nil {
		t.Fatal(err)
	}
	original, err := EncryptAutoUnlock([32]byte{1}, plain)
	if err != nil {
		t.Fatal(err)
	}

	// Store the source credential and copy it under the destination storage key.
	if err := WriteAutoUnlock(ctx, source, "source", original); err != nil {
		t.Fatal(err)
	}
	if err := CopyCredential(ctx, source, destination, "source", "destination", key, [32]byte{2}); err != nil {
		t.Fatal(err)
	}

	// Read the copied auto-unlock credential from the destination.
	encrypted, _, err := ReadAutoUnlockKey(ctx, destination, "destination")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the destination storage key unlocks the copied session credential.
	got, err := DecryptAutoUnlock([32]byte{2}, encrypted)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("destination key did not unlock: %v", err)
	}

	// Verify the source retains its original encrypted session credential.
	retained, _, err := ReadAutoUnlockKey(ctx, source, "source")
	if err != nil || !bytes.Equal(retained, original) {
		t.Fatal("source credential changed")
	}

	// Generate a conflicting session key and verify the destination rejects it.
	other, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := CopyCredential(ctx, source, destination, "source", "destination", other, [32]byte{2}); err == nil {
		t.Fatal("a different destination key was overwritten")
	}

	// Verify the rejected copy preserves the destination session credential.
	retained, _, err = ReadAutoUnlockKey(ctx, destination, "destination")
	if err != nil || !bytes.Equal(retained, encrypted) {
		t.Fatal("conflicting attempt changed the destination")
	}
}
