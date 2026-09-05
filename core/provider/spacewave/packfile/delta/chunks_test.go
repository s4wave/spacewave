package delta

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
	packfile "github.com/s4wave/spacewave/core/provider/spacewave/packfile"
)

// TestEmitDeltaChunksEncodedLimit exercises index-heavy packs at the complete
// file limit and verifies every block remains readable after splitting.
func TestEmitDeltaChunksEncodedLimit(t *testing.T) {
	// Small values make index overhead dominate the payload-only accounting.
	ctx := t.Context()
	specs, reader := buildTestKvfile(t, "encoded", 200, 32)
	iter, err := DiffBlockStores(ctx, reader, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Reopen each emitted file and collect the original values exactly once.
	const limit = 1024
	seen := make(map[string][]byte)
	_, err = EmitDeltaChunks(ctx, "encoded-limit", iter, limit, func(_ context.Context, _ int, entry *packfile.PackfileEntry, data []byte) error {
		if len(data) > limit || entry.GetSizeBytes() != uint64(len(data)) {
			t.Fatalf("encoded size = %d, metadata = %d, limit = %d", len(data), entry.GetSizeBytes(), limit)
		}
		packed, err := kvfile.BuildReader(bytes.NewReader(data), uint64(len(data)))
		if err != nil {
			return err
		}
		return packed.ScanPrefixEntries(nil, func(entry *kvfile.IndexEntry, _ int) error {
			key := string(entry.GetKey())
			if _, exists := seen[key]; exists {
				t.Fatalf("duplicate block %s", key)
			}
			value, found, err := packed.Get(entry.GetKey())
			if err != nil {
				return err
			}
			if !found {
				t.Fatalf("missing block %s", key)
			}
			seen[key] = bytes.Clone(value)
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range specs {
		if !bytes.Equal(seen[spec.key], spec.data) {
			t.Fatalf("block %s did not round trip", spec.key)
		}
	}
}

// TestEmitDeltaChunksRejectsOversizeBlock prevents a single payload from
// bypassing the encoded byte ceiling or reaching the publication callback.
func TestEmitDeltaChunksRejectsOversizeBlock(t *testing.T) {
	// The payload alone fits exactly, but its index cannot fit beside it.
	_, reader := buildTestKvfile(t, "oversize", 1, 1024)
	iter, err := DiffBlockStores(t.Context(), reader, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Reject before publishing the oversized file.
	_, err = EmitDeltaChunks(t.Context(), "oversize", iter, 1024, func(context.Context, int, *packfile.PackfileEntry, []byte) error {
		t.Fatal("oversized block reached emitter")
		return nil
	})
	if err == nil {
		t.Fatal("oversized block accepted")
	}
}
