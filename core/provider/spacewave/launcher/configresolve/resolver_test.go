package configresolve

import "testing"

const productionDistSigner = "12D3KooWL2DEcvqSXXrrCmUxMdPbqFcqzhHBvqseZWHwjAt7aXfW"
const overlayDistSigner = "12D3KooWNyn6cNNxHnLc5Nw8b7XkVaAWKB9vbfe921LuysEoY1Cz"

func TestResolveEndpointsFallsBackToDefault(t *testing.T) {
	got, err := ResolveEndpoints(false, nil, []string{"https://spacewave.app/api/release/config"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("endpoints = %d, want 1", len(got))
	}
	if got[0] != "https://spacewave.app/api/release/config" {
		t.Fatalf("endpoint = %q, want production default", got[0])
	}
}

func TestResolveEndpointsConfigReplacesDefault(t *testing.T) {
	const overlayEndpoint = "https://release-overlay.example/api/release/config"
	got, err := ResolveEndpoints(
		false,
		[]string{overlayEndpoint, overlayEndpoint},
		[]string{"https://spacewave.app/api/release/config"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("endpoints = %d, want 1: %#v", len(got), got)
	}
	if got[0] != overlayEndpoint {
		t.Fatalf("endpoint = %q, want overlay endpoint", got[0])
	}
}

func TestResolveEndpointsCanDisableEndpointFetch(t *testing.T) {
	got, err := ResolveEndpoints(true, nil, []string{"https://spacewave.app/api/release/config"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("endpoints = %d, want none: %#v", len(got), got)
	}
}

func TestResolveEndpointsRejectsDisableWithEndpoints(t *testing.T) {
	_, err := ResolveEndpoints(
		true,
		[]string{"https://release-overlay.example/api/release/config"},
		[]string{"https://spacewave.app/api/release/config"},
	)
	if err == nil {
		t.Fatal("expected disable_endpoint_fetch with endpoints to fail")
	}
}

func TestResolveDistPeerIDsFallsBackToDefault(t *testing.T) {
	got := ResolveDistPeerIDs(nil, []string{productionDistSigner})
	if len(got) != 1 {
		t.Fatalf("dist peer ids = %d, want 1", len(got))
	}
	if got[0] != productionDistSigner {
		t.Fatalf("dist peer id = %q, want production default", got[0])
	}
}

func TestResolveDistPeerIDsConfigReplacesDefault(t *testing.T) {
	got := ResolveDistPeerIDs(
		[]string{overlayDistSigner, overlayDistSigner},
		[]string{productionDistSigner},
	)
	if len(got) != 1 {
		t.Fatalf("dist peer ids = %d, want 1: %#v", len(got), got)
	}
	if got[0] != overlayDistSigner {
		t.Fatalf("dist peer id = %q, want overlay signer", got[0])
	}
}
