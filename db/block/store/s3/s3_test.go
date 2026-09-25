//go:build test_s3

package block_store_s3

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
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

// TestBlockStoreS3Refs checks that a block read back from a locally running
// endpoint carries the refs it was written with.
func TestBlockStoreS3Refs(t *testing.T) {
	ctx := t.Context()
	minioClient, err := BuildClient(testConf)
	if err != nil {
		t.Fatal(err)
	}
	store := NewS3Block(true, minioClient, bucketName, objectPrefix, 0)

	child, _, err := store.PutBlock(ctx, []byte("refs child"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Data: []byte("refs root"), Refs: []*block.BlockRef{child}}}); err != nil {
		t.Fatal(err)
	}
	root, err := block.BuildBlockRef([]byte("refs root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := store.GetStoredBlock(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || string(stored.Data) != "refs root" || !stored.RefsKnown ||
		len(stored.Refs) != 1 || !stored.Refs[0].EqualsRef(child) {
		t.Fatalf("stored root = %v, want its data and the child ref", stored)
	}
}
