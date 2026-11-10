package s3test

import (
	"io"
	"testing"
)

// TestBucketHidesOnDelete checks a delete through the client hides the object
// behind a delete marker and keeps its version readable by id.
func TestBucketHidesOnDelete(t *testing.T) {
	ctx := t.Context()
	b, client := New(t)
	if err := client.PutObject(ctx, BucketName, "p/a", []byte("a"), ""); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteObject(ctx, BucketName, "p/a"); err != nil {
		t.Fatal(err)
	}
	if keys := b.Keys("p/"); len(keys) != 0 {
		t.Fatalf("keys %v remain listed", keys)
	}
	if n := b.Hidden("p/"); n != 1 {
		t.Fatalf("%d hidden versions, want 1", n)
	}

	// The hidden version still reads by its id.
	versions, err := client.ObjectVersions(ctx, BucketName, "p/a")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("versions %v, want a marker and the data", versions)
	}
	var read []string
	for _, version := range versions {
		rc, err := client.GetObjectVersion(ctx, BucketName, "p/a", version)
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(rc)
		_ = rc.Close()
		read = append(read, string(data))
	}
	if len(read) != 1 || read[0] != "a" {
		t.Fatalf("versions read %q, want only %q", read, "a")
	}
}
