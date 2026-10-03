package blob

import (
	"context"
	"testing"
)

// TestBlob_Raw tests building and validating a raw blob.
func TestBlob_Raw(t *testing.T) {
	// Verify a complete raw blob fixture passes full validation.
	b1 := buildMockRawBlob()
	if err := b1.ValidateFull(context.Background(), nil); err != nil {
		t.Fatal(err.Error())
	}

	// Verify raw blob validation rejects a size that disagrees with its data.
	b2 := buildMockRawBlob()
	b2.TotalSize -= 2
	if err := b2.ValidateFull(context.Background(), nil); err == nil {
		t.Fatal("expected error")
	}
}
