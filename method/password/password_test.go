package auth_method_password

import (
	"testing"

	"github.com/aperturerobotics/bifrost/peer"
)

func TestBuildParametersWithUsernamePassword(t *testing.T) {
	params, priv, err := BuildParametersWithUsernamePassword("alice", []byte("hunter2"))
	if err != nil {
		t.Fatal(err)
	}
	if err := params.Validate(); err != nil {
		t.Fatal(err)
	}
	if priv == nil {
		t.Fatal("expected private key")
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	if pid.String() == "" {
		t.Fatal("expected non-empty peer ID")
	}
}

func TestDeterministic(t *testing.T) {
	_, priv1, err := BuildParametersWithUsernamePassword("bob", []byte("password123"))
	if err != nil {
		t.Fatal(err)
	}
	_, priv2, err := BuildParametersWithUsernamePassword("bob", []byte("password123"))
	if err != nil {
		t.Fatal(err)
	}

	pid1, _ := peer.IDFromPrivateKey(priv1)
	pid2, _ := peer.IDFromPrivateKey(priv2)
	if pid1 != pid2 {
		t.Fatalf("same username+password should produce same key: %s != %s", pid1, pid2)
	}
}

func TestDifferentPasswords(t *testing.T) {
	_, priv1, err := BuildParametersWithUsernamePassword("carol", []byte("pass1"))
	if err != nil {
		t.Fatal(err)
	}
	_, priv2, err := BuildParametersWithUsernamePassword("carol", []byte("pass2"))
	if err != nil {
		t.Fatal(err)
	}

	pid1, _ := peer.IDFromPrivateKey(priv1)
	pid2, _ := peer.IDFromPrivateKey(priv2)
	if pid1 == pid2 {
		t.Fatal("different passwords should produce different keys")
	}
}

func TestDifferentUsernames(t *testing.T) {
	_, priv1, err := BuildParametersWithUsernamePassword("dave", []byte("samepass"))
	if err != nil {
		t.Fatal(err)
	}
	_, priv2, err := BuildParametersWithUsernamePassword("eve", []byte("samepass"))
	if err != nil {
		t.Fatal(err)
	}

	pid1, _ := peer.IDFromPrivateKey(priv1)
	pid2, _ := peer.IDFromPrivateKey(priv2)
	if pid1 == pid2 {
		t.Fatal("different usernames should produce different keys")
	}
}

func TestAuthenticate(t *testing.T) {
	params, priv, err := BuildParametersWithUsernamePassword("frank", []byte("mypassword"))
	if err != nil {
		t.Fatal(err)
	}

	m := NewPasswordMethod()
	paramsBytes, err := params.MarshalBlock()
	if err != nil {
		t.Fatal(err)
	}
	unmarshaled, err := m.UnmarshalParameters(paramsBytes)
	if err != nil {
		t.Fatal(err)
	}

	authPriv, err := m.Authenticate(unmarshaled, []byte("mypassword"))
	if err != nil {
		t.Fatal(err)
	}

	pid1, _ := peer.IDFromPrivateKey(priv)
	pid2, _ := peer.IDFromPrivateKey(authPriv)
	if pid1 != pid2 {
		t.Fatalf("authenticate should produce same key: %s != %s", pid1, pid2)
	}
}
