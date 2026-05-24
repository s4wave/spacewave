package entitykeylogin

import (
	"strings"
	"testing"
)

func TestParsePrivateKeyRejectsMissingPrivateKey(t *testing.T) {
	_, err := ParsePrivateKey([]byte("not a pem private key"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "PEM private key") {
		t.Fatalf("expected PEM private key error, got %v", err)
	}
}
