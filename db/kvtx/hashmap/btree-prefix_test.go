package hashmap

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
)

func TestBTreePrefixBoundsAndCancellation(t *testing.T) {
	ctx := t.Context()
	m := NewBTreeMap[int]()
	for i, key := range []string{"a", "ba", "bb", "c", "\xff", "\xff\x00"} {
		if err := m.Set(ctx, []byte(key), i); err != nil {
			t.Fatal(err)
		}
	}
	for prefix, want := range map[string][]string{"b": {"ba", "bb"}, "\xff": {"\xff", "\xff\x00"}, "missing": nil} {
		var keys []string
		err := m.IteratePrefix(ctx, []byte(prefix), func(_ context.Context, key []byte, _ int) error {
			keys = append(keys, string(key))
			return nil
		})
		if err != nil || !slices.Equal(keys, want) {
			t.Fatalf("prefix %q: got %q, want %q: %v", prefix, keys, want, err)
		}
	}
	stop := errors.New("stop scan")
	if err := m.Iterate(ctx, func(context.Context, []byte, int) error { return stop }); !errors.Is(err, stop) {
		t.Fatalf("callback error lost: %v", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	visited := 0
	err := m.IteratePrefix(canceled, []byte("b"), func(context.Context, []byte, int) error {
		visited++
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) || visited != 1 {
		t.Fatalf("canceled scan visited=%d err=%v", visited, err)
	}
}

// BenchmarkHashmapPrefix scans ten neighboring keys in a 100,000-key store.
// The transaction wrapper matches the prefix operation used by Cayley queries.
func BenchmarkHashmapPrefix(b *testing.B) {
	ctx := b.Context()
	m := NewBTreeMap[[]byte]()
	for i := range 100000 {
		key := []byte(fmt.Sprintf("%06d", i))
		if err := m.Set(ctx, key, key); err != nil {
			b.Fatal(err)
		}
	}
	tx, err := NewHashmapKvtx(m).NewTransaction(ctx, false)
	if err != nil {
		b.Fatal(err)
	}
	defer tx.Discard()
	prefix := []byte("05000")
	b.ReportAllocs()
	for b.Loop() {
		count := 0
		err := tx.ScanPrefix(ctx, prefix, func(_, _ []byte) error { count++; return nil })
		if err != nil || count != 10 {
			b.Fatalf("matches=%d err=%v", count, err)
		}
	}
}
