package auth_method_pem

import (
	"testing"

	"github.com/aperturerobotics/bifrost/keypem"
	"github.com/aperturerobotics/bifrost/peer"
)

// TestPemMethod_Authenticate_Success verifies successful authentication with a valid PEM keypair.
func TestPemMethod_Authenticate_Success(t *testing.T) {
	privPem, pubPem, err := GenerateBackupKey()
	if err != nil {
		t.Fatal(err)
	}

	method := NewPemMethod()
	if method.GetMethodID() != MethodID {
		t.Fatalf("expected method ID %q, got %q", MethodID, method.GetMethodID())
	}

	params, err := method.UnmarshalParameters(pubPem)
	if err != nil {
		t.Fatalf("UnmarshalParameters: %v", err)
	}

	privKey, err := method.Authenticate(params, privPem)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}

	pid, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		t.Fatalf("IDFromPrivateKey: %v", err)
	}
	t.Logf("authenticated peer ID: %s", pid.String())

	// Verify the authenticated key matches the generated one.
	origPriv, err := keypem.ParsePrivKeyPem(privPem)
	if err != nil {
		t.Fatalf("ParsePrivKeyPem: %v", err)
	}
	origPID, err := peer.IDFromPrivateKey(origPriv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey (orig): %v", err)
	}
	if pid != origPID {
		t.Fatalf("peer IDs do not match: got %s, want %s", pid.String(), origPID.String())
	}
}

// TestPemMethod_Authenticate_InvalidPEM verifies authentication fails with invalid PEM data.
func TestPemMethod_Authenticate_InvalidPEM(t *testing.T) {
	_, pubPem, err := GenerateBackupKey()
	if err != nil {
		t.Fatal(err)
	}

	method := NewPemMethod()
	params, err := method.UnmarshalParameters(pubPem)
	if err != nil {
		t.Fatalf("UnmarshalParameters: %v", err)
	}

	// Empty secret data.
	_, err = method.Authenticate(params, nil)
	if err == nil {
		t.Fatal("expected error for nil auth secret data")
	}

	// Invalid PEM bytes.
	_, err = method.Authenticate(params, []byte("not-a-pem-file"))
	if err == nil {
		t.Fatal("expected error for invalid PEM data")
	}
}

// TestPemMethod_UnmarshalParameters_Invalid verifies UnmarshalParameters fails with invalid data.
func TestPemMethod_UnmarshalParameters_Invalid(t *testing.T) {
	method := NewPemMethod()

	_, err := method.UnmarshalParameters(nil)
	if err == nil {
		t.Fatal("expected error for nil parameters")
	}

	_, err = method.UnmarshalParameters([]byte("not-a-pem"))
	if err == nil {
		t.Fatal("expected error for invalid PEM parameters")
	}
}

// TestGenerateBackupKey verifies GenerateBackupKey produces valid PEM key pair.
func TestGenerateBackupKey(t *testing.T) {
	privPem, pubPem, err := GenerateBackupKey()
	if err != nil {
		t.Fatalf("GenerateBackupKey: %v", err)
	}

	if len(privPem) == 0 {
		t.Fatal("private key PEM is empty")
	}
	if len(pubPem) == 0 {
		t.Fatal("public key PEM is empty")
	}

	// Verify the PEM can be parsed back.
	privKey, err := keypem.ParsePrivKeyPem(privPem)
	if err != nil {
		t.Fatalf("ParsePrivKeyPem: %v", err)
	}
	if privKey == nil {
		t.Fatal("parsed private key is nil")
	}

	pubKey, err := keypem.ParsePubKeyPem(pubPem)
	if err != nil {
		t.Fatalf("ParsePubKeyPem: %v", err)
	}
	if pubKey == nil {
		t.Fatal("parsed public key is nil")
	}

	// Verify the public key from the private key matches the standalone pub key.
	privPub := privKey.GetPublic()
	privPubRaw, err := privPub.Raw()
	if err != nil {
		t.Fatalf("privPub.Raw: %v", err)
	}
	pubRaw, err := pubKey.Raw()
	if err != nil {
		t.Fatalf("pubKey.Raw: %v", err)
	}
	if len(privPubRaw) != len(pubRaw) {
		t.Fatalf("public key length mismatch: %d vs %d", len(privPubRaw), len(pubRaw))
	}
	for i := range privPubRaw {
		if privPubRaw[i] != pubRaw[i] {
			t.Fatal("public keys do not match")
		}
	}

	// Verify a second call produces a different keypair.
	privPem2, pubPem2, err := GenerateBackupKey()
	if err != nil {
		t.Fatalf("GenerateBackupKey (2nd): %v", err)
	}
	if string(privPem) == string(privPem2) {
		t.Fatal("two calls produced identical private keys")
	}
	if string(pubPem) == string(pubPem2) {
		t.Fatal("two calls produced identical public keys")
	}
}
