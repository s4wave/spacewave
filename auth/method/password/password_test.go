package auth_method_password

import (
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

const testScryptN uint32 = 14

func buildTestParametersWithUsernamePassword(username string, password []byte) (*Parameters, crypto.PrivKey, error) {
	return buildParametersWithUsernamePassword(
		username,
		password,
		testScryptN,
		DefaultScryptR,
		DefaultScryptP,
	)
}

func TestBuildParametersWithUsernamePassword(t *testing.T) {
	// Build and validate the test parameters and expect a private key.
	params, priv, err := buildTestParametersWithUsernamePassword("alice", []byte("hunter2"))
	if err != nil {
		t.Fatal(err)
	}
	if err := params.Validate(); err != nil {
		t.Fatal(err)
	}
	if priv == nil {
		t.Fatal("expected private key")
	}

	// Derive the peer ID from the private key and expect it to be non-empty.
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	if pid.String() == "" {
		t.Fatal("expected non-empty peer ID")
	}
}

func TestDeterministic(t *testing.T) {
	// Build the same username and password twice and expect equal peer IDs.
	_, priv1, err := buildTestParametersWithUsernamePassword("bob", []byte("password123"))
	if err != nil {
		t.Fatal(err)
	}
	_, priv2, err := buildTestParametersWithUsernamePassword("bob", []byte("password123"))
	if err != nil {
		t.Fatal(err)
	}

	// Derive the peer IDs and expect them to be equal.
	pid1, _ := peer.IDFromPrivateKey(priv1)
	pid2, _ := peer.IDFromPrivateKey(priv2)
	if pid1 != pid2 {
		t.Fatalf("same username+password should produce same key: %s != %s", pid1, pid2)
	}
}

func TestDifferentPasswords(t *testing.T) {
	// Build the same username with two passwords and expect distinct peer IDs.
	_, priv1, err := buildTestParametersWithUsernamePassword("carol", []byte("pass1"))
	if err != nil {
		t.Fatal(err)
	}
	_, priv2, err := buildTestParametersWithUsernamePassword("carol", []byte("pass2"))
	if err != nil {
		t.Fatal(err)
	}

	// Derive the peer IDs and expect them to differ.
	pid1, _ := peer.IDFromPrivateKey(priv1)
	pid2, _ := peer.IDFromPrivateKey(priv2)
	if pid1 == pid2 {
		t.Fatal("different passwords should produce different keys")
	}
}

func TestDifferentUsernames(t *testing.T) {
	// Build two usernames with the same password and expect distinct peer IDs.
	_, priv1, err := buildTestParametersWithUsernamePassword("dave", []byte("samepass"))
	if err != nil {
		t.Fatal(err)
	}
	_, priv2, err := buildTestParametersWithUsernamePassword("eve", []byte("samepass"))
	if err != nil {
		t.Fatal(err)
	}

	// Derive the peer IDs and expect them to differ.
	pid1, _ := peer.IDFromPrivateKey(priv1)
	pid2, _ := peer.IDFromPrivateKey(priv2)
	if pid1 == pid2 {
		t.Fatal("different usernames should produce different keys")
	}
}

func TestAuthenticate(t *testing.T) {
	// Build the reference parameters and private key.
	params, priv, err := buildTestParametersWithUsernamePassword("frank", []byte("mypassword"))
	if err != nil {
		t.Fatal(err)
	}

	// Round-trip the parameters through the method's parameter codec.
	m := NewPasswordMethod()
	paramsBytes, err := params.MarshalBlock()
	if err != nil {
		t.Fatal(err)
	}
	unmarshaled, err := m.UnmarshalParameters(paramsBytes)
	if err != nil {
		t.Fatal(err)
	}

	// Authenticate with the unmarshaled parameters and compare peer IDs.
	authPriv, err := m.Authenticate(unmarshaled, []byte("mypassword"))
	if err != nil {
		t.Fatal(err)
	}

	// Derive the peer IDs and expect them to be equal.
	pid1, _ := peer.IDFromPrivateKey(priv)
	pid2, _ := peer.IDFromPrivateKey(authPriv)
	if pid1 != pid2 {
		t.Fatalf("authenticate should produce same key: %s != %s", pid1, pid2)
	}
}

// TestValidateScryptBounds tests that oversized scrypt parameters are rejected.
func TestValidateScryptBounds(t *testing.T) {
	// Collect parameter sets that should pass validation.
	salt := make([]byte, saltLen)
	valid := []*Parameters{
		{Salt: salt},
		{Salt: salt, ScryptN: DefaultScryptN, ScryptR: DefaultScryptR, ScryptP: DefaultScryptP},
	}

	// Validate each accepted parameter set.
	for i, params := range valid {
		if err := params.Validate(); err != nil {
			t.Fatalf("valid[%d]: %v", i, err)
		}
	}

	// Collect parameter sets that should fail validation, marshaling, and
	// authentication.
	invalid := []*Parameters{
		{Salt: salt, ScryptN: MaxScryptN + 1},
		{Salt: salt, ScryptN: 63},
		{Salt: salt, ScryptR: MaxScryptR + 1},
		{Salt: salt, ScryptP: MaxScryptP + 1},
	}
	method := NewPasswordMethod()

	// Reject each oversized parameter set at every entry point.
	for i, params := range invalid {
		if err := params.Validate(); err == nil {
			t.Fatalf("invalid[%d]: expected validate error", i)
		}
		data, err := params.MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := method.UnmarshalParameters(data); err == nil {
			t.Fatalf("invalid[%d]: expected unmarshal error", i)
		}
		if _, err := method.Authenticate(params, []byte("pw")); err == nil {
			t.Fatalf("invalid[%d]: expected authenticate error", i)
		}
	}
}
