package hashmap

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
)

func TestBTreePrefixBoundsAndCancellation(t *testing.T) {
	// Populate the B-tree with ordinary and maximum-byte prefixes.
	ctx := t.Context()
	m := NewBTreeMap[int]()
	for i, key := range []string{"a", "ba", "bb", "c", "\xff", "\xff\x00"} {
		if err := m.Set(ctx, []byte(key), i); err != nil {
			t.Fatal(err)
		}
	}

	// Verify each prefix scan returns only its matching keys.
	for prefix, want := range map[string][]string{"b": {"ba", "bb"}, "\xff": {"\xff", "\xff\x00"}, "missing": nil} {
		// Collect the keys yielded by the prefix scan.
		var keys []string
		err := m.IteratePrefix(ctx, []byte(prefix), func(_ context.Context, key []byte, _ int) error {
			keys = append(keys, string(key))
			return nil
		})

		// Compare the scanned keys with the prefix fixture.
		if err != nil || !slices.Equal(keys, want) {
			t.Fatalf("prefix %q: got %q, want %q: %v", prefix, keys, want, err)
		}
	}

	// Verify the B-tree scan preserves the callback error.
	stop := errors.New("stop scan")
	if err := m.Iterate(ctx, func(context.Context, []byte, int) error { return stop }); !errors.Is(err, stop) {
		t.Fatalf("callback error lost: %v", err)
	}

	// Cancel the prefix scan after its first matching key.
	canceled, cancel := context.WithCancel(ctx)
	visited := 0
	err := m.IteratePrefix(canceled, []byte("b"), func(context.Context, []byte, int) error {
		visited++
		cancel()
		return nil
	})

	// Require the canceled scan to stop after one callback.
	if !errors.Is(err, context.Canceled) || visited != 1 {
		t.Fatalf("canceled scan visited=%d err=%v", visited, err)
	}
}

// BenchmarkHashmapPrefix scans ten neighboring keys in a 100,000-key store.
// The transaction wrapper matches the prefix operation used by Cayley queries.
func BenchmarkHashmapPrefix(b *testing.B) {
	// Populate the benchmark B-tree with 100,000 ordered keys.
	ctx := b.Context()
	m := NewBTreeMap[[]byte]()
	for i := range 100000 {
		key := []byte(fmt.Sprintf("%06d", i))
		if err := m.Set(ctx, key, key); err != nil {
			b.Fatal(err)
		}
	}

	// Open the read transaction used by the prefix benchmark.
	tx, err := NewHashmapKvtx(m).NewTransaction(ctx, false)
	if err != nil {
		b.Fatal(err)
	}
	defer tx.Discard()

	// Measure repeated scans of the ten-key prefix.
	prefix := []byte("05000")
	b.ReportAllocs()
	for b.Loop() {
		// Count the keys returned by the transaction prefix scan.
		count := 0
		err := tx.ScanPrefix(ctx, prefix, func(_, _ []byte) error { count++; return nil })

		// Require each benchmark scan to return ten matching keys.
		if err != nil || count != 10 {
			b.Fatalf("matches=%d err=%v", count, err)
		}
	}
}
