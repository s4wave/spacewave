//go:build !js

package spacewave_cli

import (
	"testing"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/s4wave/spacewave/core/sobject"
)

// TestDecodeInvite verifies the invite forms the app hands out decode the same.
func TestDecodeInvite(t *testing.T) {
	// Encode an invite the way the app links it.
	want := &sobject.SOInviteMessage{SharedObjectId: "space"}
	data, err := want.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	encoded := b58.Encode(data)

	// Links and bearer tokens decode locally.
	for _, input := range []string{
		"https://spacewave.app/#/join/" + encoded,
		bearerInvitePrefix + encoded,
	} {
		got, err := decodeInvite(input)
		if err != nil {
			t.Fatalf("decode %q: %v", input, err)
		}
		if !got.EqualVT(want) {
			t.Fatalf("decode %q = %v, want %v", input, got, want)
		}
	}

	// A bare code is left for the cloud lookup.
	got, err := decodeInvite("ABCD2345")
	if err != nil || got != nil {
		t.Fatalf("short code decoded to %v, %v", got, err)
	}
}
