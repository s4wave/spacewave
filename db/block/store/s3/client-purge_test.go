//go:build !tinygo

package block_store_s3

import (
	"context"
	"slices"
	"testing"
)

// TestPurgeObject purges every version of a key on a bucket that keeps old
// versions, and refuses a bucket that cannot list them rather than leave a
// hidden version billed.
func TestPurgeObject(t *testing.T) {
	ctx := context.Background()

	t.Run("versioned", func(t *testing.T) {
		// Hide two versions of the key, keep a current one, and a neighbor.
		b, c := newFakeBucket(t, map[string]string{"p/a": "1", "p/ab": "keep"})
		b.versioned = true
		b.mtx.Lock()
		b.hidden["p/a"] = 2
		b.mtx.Unlock()

		// Purge, then find no version of the key and the neighbor untouched.
		var got []string
		if err := c.PurgeObject(ctx, "bucket", "p/a", func(v string) { got = append(got, v) }); err != nil {
			t.Fatal(err)
		}
		slices.Sort(got)
		if want := []string{"current", "hidden", "hidden"}; !slices.Equal(got, want) {
			t.Fatalf("deleted %v, want %v", got, want)
		}
		if n := b.hiddenVersions(); n != 0 {
			t.Fatalf("%d hidden versions left", n)
		}
		if keys := b.keys(); !slices.Equal(keys, []string{"p/ab"}) {
			t.Fatalf("keys %v, want only the neighbor", keys)
		}

		// A repeated purge finds nothing and succeeds.
		if err := c.PurgeObject(ctx, "bucket", "p/a", func(v string) { t.Fatalf("deleted %s again", v) }); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("unlisted", func(t *testing.T) {
		// A bucket that answers a version listing with 501 keeps the object.
		b, c := newFakeBucket(t, map[string]string{"p/a": "1"})
		if err := c.PurgeObject(ctx, "bucket", "p/a", func(v string) { t.Fatalf("deleted %s", v) }); err == nil {
			t.Fatal("purge fell back where versions cannot be listed")
		}
		if keys := b.keys(); !slices.Equal(keys, []string{"p/a"}) {
			t.Fatalf("keys %v, want the object kept", keys)
		}
	})
}
