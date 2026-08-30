package dex_solicit

import (
	"bytes"
	"testing"
)

func TestSolicitationContextMatchesLogicalStoreAcrossBuckets(t *testing.T) {
	logicalStore := []byte("shared-space")
	left := solicitationContext(&Config{BucketId: "left-bucket", ProtocolContext: logicalStore})
	right := solicitationContext(&Config{BucketId: "right-bucket", ProtocolContext: logicalStore})
	if !bytes.Equal(left, right) {
		t.Fatalf("logical store contexts differ: %q != %q", left, right)
	}
	if got := string(solicitationContext(&Config{BucketId: "local-bucket"})); got != "local-bucket" {
		t.Fatalf("legacy bucket context = %q, want local-bucket", got)
	}
}
