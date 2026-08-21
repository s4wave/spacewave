package bucket

import "testing"

// TestObjectRefCopyFromNilReceiver tests that copying into a nil receiver
// leaves the source object untouched.
func TestObjectRefCopyFromNilReceiver(t *testing.T) {
	var o *ObjectRef
	src := &ObjectRef{BucketId: "test-bucket"}
	o.CopyFrom(src)

	if src.BucketId != "test-bucket" {
		t.Fatalf("source mutated by nil-receiver copy: %v", src.BucketId)
	}
}
