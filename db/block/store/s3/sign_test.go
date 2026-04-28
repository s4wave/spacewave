//go:build !tinygo

package block_store_s3

import (
	"net/http"
	"testing"
	"time"
)

// TestSignV4GetObject verifies the signer against the AWS S3 SigV4 example
// "GET Object" from the AWS Signature Version 4 documentation.
//
// Reference: https://docs.aws.amazon.com/AmazonS3/latest/API/sig-v4-header-based-auth.html
func TestSignV4GetObject(t *testing.T) {
	c := &Client{
		endpoint:  "examplebucket.s3.amazonaws.com",
		region:    "us-east-1",
		accessKey: "AKIAIOSFODNN7EXAMPLE",
		secretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		useSSL:    true,
	}
	req, err := http.NewRequest(http.MethodGet, "https://examplebucket.s3.amazonaws.com/test.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Range", "bytes=0-9")

	now := time.Date(2013, 5, 24, 0, 0, 0, 0, time.UTC)
	c.signV4(req, emptyPayloadHash, now)

	want := "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request, SignedHeaders=host;range;x-amz-content-sha256;x-amz-date, Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41"
	got := req.Header.Get("Authorization")
	if got != want {
		t.Fatalf("Authorization mismatch:\n want: %s\n  got: %s", want, got)
	}
}
