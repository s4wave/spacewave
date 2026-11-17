package s3test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/pkg/errors"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// TestHider hides and unhides objects through the S3 client, and tries to
// make each call lose a version or serve the wrong one. A hide keeps every
// version and a repeat adds no marker; a read by id serves the hidden bytes
// even after a later write to the key; an unhide makes the exact version
// current, and refuses a key a later write covers or a version that is gone;
// a bucket that does not keep versions refuses the hide and keeps the object.
// No call deletes a content version.
func TestHider(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	b, client := New(t)
	var hider block_store_s3.ObjectHider = client
	put := func(key, data string) {
		t.Helper()
		if err := client.PutObject(ctx, BucketName, key, []byte(data), "application/octet-stream"); err != nil {
			t.Fatal(err)
		}
	}
	read := func(key string) (string, error) {
		data, err := client.GetObjectRange(ctx, BucketName, key, 0, 64)
		return string(data), err
	}
	versions := func(key string, want ...string) {
		t.Helper()
		if got := b.Versions(key); !slices.Equal(got, want) {
			t.Fatalf("versions of %s = %q, want %q", key, got, want)
		}
	}
	noneDeleted := func(step string) {
		t.Helper()
		if n := b.ContentDeleted(""); n != 0 {
			t.Fatalf("after the %s the bucket deleted %d content versions", step, n)
		}
	}

	// A hide covers the current version and keeps both.
	put("a", "one")
	put("a", "two")
	hidden, err := hider.HideObjectVersion(ctx, BucketName, "a")
	if err != nil || hidden.Version == "" || hidden.Marker == "" {
		t.Fatalf("hide = %+v, %v", hidden, err)
	}
	t.Logf("hid a: version %s under marker %s", hidden.Version, hidden.Marker)
	versions("a", "a hidden", "a two", "a one")
	if _, err := read("a"); !errors.Is(err, block_store_s3.ErrNotFound) {
		t.Fatalf("plain read of a hidden key = %v, want not found", err)
	}
	if data, err := hider.GetObjectVersionRange(ctx, BucketName, "a", hidden.Version, 1, 2); err != nil || string(data) != "wo" {
		t.Fatalf("read of the hidden version = %q, %v", data, err)
	}
	if v, err := hider.HiddenObjectVersion(ctx, BucketName, "a"); err != nil || v != hidden.Version {
		t.Fatalf("hidden version = %q, %v; want %q", v, err, hidden.Version)
	}

	// A repeated hide returns the same version and adds no marker.
	again, err := hider.HideObjectVersion(ctx, BucketName, "a")
	if err != nil || again != hidden {
		t.Fatalf("second hide = %+v, %v; want %+v", again, err, hidden)
	}
	versions("a", "a hidden", "a two", "a one")
	noneDeleted("hides")

	// An unhide naming a version that is gone refuses and deletes nothing.
	if err := hider.UnhideObjectVersion(ctx, BucketName, "a", "no-such-version"); err == nil {
		t.Fatal("unhid a version that is not there")
	}
	versions("a", "a hidden", "a two", "a one")

	// A second marker, as a lifecycle rule adds, goes too, and the unhide
	// makes the exact version current; a repeat succeeds.
	b.Hide("a")
	for range 2 {
		if err := hider.UnhideObjectVersion(ctx, BucketName, "a", hidden.Version); err != nil {
			t.Fatal(err)
		}
		versions("a", "a two", "a one")
		if data, err := read("a"); err != nil || data != "two" {
			t.Fatalf("read after the unhide = %q, %v", data, err)
		}
	}
	noneDeleted("unhide")

	// A key written after its hide: a read by id still serves the hidden
	// bytes, and the unhide refuses rather than delete the newer version.
	put("b", "old")
	hiddenB, err := hider.HideObjectVersion(ctx, BucketName, "b")
	if err != nil {
		t.Fatal(err)
	}
	put("b", "new")
	if data, err := hider.GetObjectVersionRange(ctx, BucketName, "b", hiddenB.Version, 0, 8); err != nil || string(data) != "old" {
		t.Fatalf("read of the hidden version after a later write = %q, %v", data, err)
	}
	if err := hider.UnhideObjectVersion(ctx, BucketName, "b", hiddenB.Version); err == nil {
		t.Fatal("unhid a version a later write covers")
	}
	versions("b", "b new", "b hidden", "b old")
	if v, err := hider.HiddenObjectVersion(ctx, BucketName, "b"); err != nil || v != "" {
		t.Fatalf("hidden version of a rewritten key = %q, %v; want none", v, err)
	}

	// A read by id refuses to name no version, and a hide of a key with no
	// content finds nothing.
	if _, err := hider.GetObjectVersionRange(ctx, BucketName, "a", "", 0, 1); err == nil {
		t.Fatal("read with no version named")
	}
	if _, err := hider.HideObjectVersion(ctx, BucketName, "missing"); !errors.Is(err, block_store_s3.ErrNotFound) {
		t.Fatalf("hide of a missing key = %v, want not found", err)
	}
	noneDeleted("later write")

	// A bucket that keeps no versions refuses the hide, keeping the object.
	b.SetVersioning(false)
	put("u", "kept")
	if _, err := hider.HideObjectVersion(ctx, BucketName, "u"); !errors.Is(err, block_store_s3.ErrNotVersioned) {
		t.Fatalf("hide without versioning = %v, want not versioned", err)
	}
	if data, err := read("u"); err != nil || data != "kept" {
		t.Fatalf("read after the refused hide = %q, %v", data, err)
	}
	if n := b.ContentDeleted("u"); n != 0 {
		t.Fatalf("the refused hide deleted %d versions", n)
	}
}
