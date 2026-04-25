package resource_provider

import (
	"context"
	"strings"
	"testing"

	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

func TestLoginWithEntityKeyRejectsNonPEMPrivateKey(t *testing.T) {
	r := &SpacewaveProviderResource{}
	_, err := r.LoginWithEntityKey(
		context.Background(),
		&s4wave_provider_spacewave.LoginWithEntityKeyRequest{
			PemPrivateKey: []byte("not a pem private key"),
		},
	)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "PEM private key") {
		t.Fatalf("expected PEM private key error, got %v", err)
	}
}
