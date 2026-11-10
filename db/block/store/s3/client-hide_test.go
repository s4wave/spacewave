//go:build !tinygo

package block_store_s3

import (
	"net/http"
	"slices"
	"testing"
)

// TestDeleteObjectHidesOnly deletes and moves objects in a versioned bucket
// and finds every old version kept hidden: no request names a version, so the
// bucket's lifecycle rule, not the client, decides when a hidden version goes.
func TestDeleteObjectHidesOnly(t *testing.T) {
	// Serve a versioned bucket and record every delete request.
	ctx := t.Context()
	bucket, client := newFakeBucket(t, map[string]string{
		"p/packs/a": "a",
		"p/packs/b": "b",
	})
	bucket.versioned = true
	var deletes []string
	bucket.after = func(r *http.Request) {
		if r.Method == http.MethodDelete {
			deletes = append(deletes, r.URL.RawQuery)
		}
	}

	// Delete one object, and move the other to the trash.
	if err := client.DeleteObject(ctx, "bucket", "p/packs/a"); err != nil {
		t.Fatal(err)
	}
	if err := client.MoveObject(ctx, "bucket", "p/packs/b", "p/trash/b"); err != nil {
		t.Fatal(err)
	}

	// Each delete was a hide: the listing lost the keys, and both old
	// versions are kept.
	for _, query := range deletes {
		if query != "" {
			t.Errorf("delete sent query %q; want a plain DELETE", query)
		}
	}
	if len(deletes) != 2 {
		t.Errorf("sent %d deletes; want 2", len(deletes))
	}
	if keys := bucket.keys(); !slices.Equal(keys, []string{"p/trash/b"}) {
		t.Errorf("objects = %v; want only p/trash/b", keys)
	}
	if hidden := bucket.hiddenVersions(); hidden != 2 {
		t.Errorf("bucket keeps %d hidden versions; want 2", hidden)
	}

	// Deleting a missing object is already done, and hides nothing more.
	if err := client.DeleteObject(ctx, "bucket", "p/packs/a"); err != nil {
		t.Fatal(err)
	}
	if hidden := bucket.hiddenVersions(); hidden != 2 {
		t.Errorf("bucket keeps %d hidden versions after a repeat; want 2", hidden)
	}
}
