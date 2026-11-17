package s3test

import "testing"

// TestMover checks the S3 client moves by a copy and a delete, putting no
// body.
func TestMover(t *testing.T) {
	b, client := New(t)
	CheckMover(t, client, BucketName)
	if b.Puts() != 1 || b.Copies() != 2 {
		t.Fatalf("puts %d and copies %d, want 1 and 2", b.Puts(), b.Copies())
	}
}
