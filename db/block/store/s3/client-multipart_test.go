package block_store_s3_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
)

// TestPutObjectStream uploads an object in concurrent parts, a short one as
// one PUT, and leaves nothing behind a failed upload.
func TestPutObjectStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	bucket, client := s3test.New(t)
	opts := block_store_s3.StreamOptions{ContentType: "application/octet-stream", PartBytes: block_store_s3.MinPartBytes, Workers: 3}

	// A stream of several parts, the last one short, arrives whole.
	data := make([]byte, 3*block_store_s3.MinPartBytes+100)
	for i := range data {
		data[i] = byte(i * 7 / 3)
	}
	size, err := client.PutObjectStream(ctx, s3test.BucketName, "big", bytes.NewReader(data), opts)
	if err != nil || size != int64(len(data)) {
		t.Fatalf("stream: %d bytes, %v", size, err)
	}
	body, err := client.GetObjectRange(ctx, s3test.BucketName, "big", 0, len(data))
	if err != nil || !bytes.Equal(body, data) {
		t.Fatalf("read back %d bytes, %v", len(body), err)
	}

	// A short stream and an empty one are plain objects.
	puts := bucket.Puts()
	if _, err := client.PutObjectStream(ctx, s3test.BucketName, "short", strings.NewReader("x"), opts); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PutObjectStream(ctx, s3test.BucketName, "empty", strings.NewReader(""), opts); err != nil {
		t.Fatal(err)
	}
	if bucket.Puts() != puts+2 {
		t.Fatalf("short streams took %d PUTs", bucket.Puts()-puts)
	}

	// A failed part aborts the upload, and no object appears.
	bucket.FailPut(func(key string) bool { return key == "failed" })
	if _, err := client.PutObjectStream(ctx, s3test.BucketName, "failed", bytes.NewReader(data), opts); err == nil {
		t.Fatal("a failed part must fail the upload")
	}
	if len(bucket.Keys("failed")) != 0 || bucket.OpenUploads() != 0 {
		t.Fatalf("failed upload left %v and %d open", bucket.Keys("failed"), bucket.OpenUploads())
	}
}
