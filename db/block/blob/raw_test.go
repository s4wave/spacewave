package blob

import (
	"context"
	"testing"
)

// TestBlob_Raw tests building and validating a raw blob.
func TestBlob_Raw(t *testing.T) {
	b1 := buildMockRawBlob()
	if err := b1.ValidateFull(context.Background(), nil); err != nil {
		t.Fatal(err.Error())
	}

	b2 := buildMockRawBlob()
	b2.TotalSize -= 2
	if err := b2.ValidateFull(context.Background(), nil); err == nil {
		t.Fatal("expected error")
	}
}
