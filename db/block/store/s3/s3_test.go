//go:build test_s3

package block_store_s3

import (
	"context"
	"testing"

	block_store_test "github.com/s4wave/spacewave/db/block/store/test"
)

var (
	bucketName   = "hydratest"
	objectPrefix = "test/"
)

var testConf = &ClientConfig{
	Endpoint:   "127.0.0.1:9000",
	DisableSsl: true,
	Credentials: &Credentials{
		AccessKeyId:     "hydra",
		SecretAccessKey: "hydratest",
	},
}

// TestBlockStoreS3 tests the s3 block store with a locally running endpoint.
func TestBlockStoreS3(t *testing.T) {
	ctx := context.Background()

	// Create the client
	minioClient, err := BuildClient(testConf)
	if err != nil {
		t.Fatal(err.Error())
	}

	client := NewS3Block(true, minioClient, bucketName, objectPrefix, 0)
	if err := block_store_test.TestAll(ctx, client, 0); err != nil {
		t.Fatal(err.Error())
	}
}
