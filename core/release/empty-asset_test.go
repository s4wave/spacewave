package release

import (
	"crypto/sha256"
	"testing"
)

// TestEmptyBrowserAsset verifies zero-byte modules survive metadata serialization.
func TestEmptyBrowserAsset(t *testing.T) {
	// An empty module has a real content digest despite containing no bytes.
	digest := sha256.Sum256(nil)
	asset := &BrowserAsset{
		Path:        "/entrypoint/empty.mjs",
		Sha256:      digest[:],
		ContentType: "application/javascript",
	}
	if err := asset.Validate(); err != nil {
		t.Fatal(err)
	}

	// Proto scalar defaults must preserve the valid zero size on readback.
	data, err := asset.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	var decoded BrowserAsset
	if err := decoded.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatal(err)
	}
	if decoded.GetSize() != 0 {
		t.Fatal("empty asset acquired a nonzero size")
	}

	// Empty content does not waive the required content identity.
	decoded.Sha256 = nil
	if err := decoded.Validate(); err == nil {
		t.Fatal("expected missing digest to fail")
	}
}
