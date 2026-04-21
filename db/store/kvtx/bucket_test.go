package store_kvtx

import (
	"testing"

	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
)

// TestBucketReconcilerMqueueId tests marshal/unmarshal consistency.
func TestBucketReconcilerMqueueId(t *testing.T) {
	p := bucket_store.BucketReconcilerPair{
		BucketID:     "bucket-id",
		ReconcilerID: "reconciler-id",
	}
	d := MarshalBucketReconcilerMqueueId(p)
	t.Log(string(d))
	expected := "JpsGBN3hb4s8RpwRBMsHuPaw7ML5KjEs1mD"
	if string(d) != expected {
		t.Fatalf("expected %s got %s", expected, string(d))
	}
}
