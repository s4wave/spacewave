package spacewave_launcher_controller

import (
	crypto_rand "crypto/rand"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

const productionDistSigner = "12D3KooWL2DEcvqSXXrrCmUxMdPbqFcqzhHBvqseZWHwjAt7aXfW"

func TestResolveEndpointsFallsBackToProductionDefault(t *testing.T) {
	got, err := ResolveEndpoints(&Config{ProjectId: "spacewave"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("endpoints = %d, want 1", len(got))
	}
	if got[0].GetUrl() != "https://spacewave.app/api/release/config" {
		t.Fatalf("endpoint = %q, want production default", got[0].GetUrl())
	}
}

func TestResolveEndpointsConfigReplacesProductionDefault(t *testing.T) {
	const overlayEndpoint = "https://release-overlay.example/api/release/config"
	got, err := ResolveEndpoints(&Config{
		ProjectId: "spacewave",
		Endpoints: []*HttpEndpoint{
			{Url: overlayEndpoint},
			{Url: overlayEndpoint},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("endpoints = %d, want 1: %#v", len(got), got)
	}
	if got[0].GetUrl() != overlayEndpoint {
		t.Fatalf("endpoint = %q, want overlay endpoint", got[0].GetUrl())
	}
}

func TestResolveDistPeerIDsFallsBackToProductionDefault(t *testing.T) {
	got, err := ResolveDistPeerIDs(&Config{ProjectId: "spacewave"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("dist peer ids = %d, want 1", len(got))
	}
	if got[0].String() != productionDistSigner {
		t.Fatalf("dist peer id = %q, want production default", got[0].String())
	}
}

func TestResolveDistPeerIDsConfigReplacesProductionDefault(t *testing.T) {
	overlaySigner := newTestPeerID(t)
	got, err := ResolveDistPeerIDs(&Config{
		ProjectId:   "spacewave",
		DistPeerIds: []string{overlaySigner, overlaySigner},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("dist peer ids = %d, want 1: %#v", len(got), got)
	}
	if got[0].String() != overlaySigner {
		t.Fatalf("dist peer id = %q, want overlay signer", got[0].String())
	}
}

func newTestPeerID(t *testing.T) string {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(crypto_rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return pid.String()
}
