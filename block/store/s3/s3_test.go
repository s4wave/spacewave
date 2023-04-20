//go:build test_s3
// +build test_s3

package block_store_s3

import (
	"bytes"
	"context"
	"testing"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
)

var bucketName = "hydratest"
var objectPrefix = "test/"

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

	client := NewS3Block(ctx, true, minioClient, bucketName, objectPrefix, 0)

	sampleBlockBody := []byte("How hard are these tests? What exactly was in that phonebook of a contract I signed?")
	samplePutOpts := &block.PutOpts{HashType: hash.HashType_HashType_BLAKE3}
	sampleBlockRef, err := block.BuildBlockRef(sampleBlockBody, samplePutOpts)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check if the block exists
	retBlockExists, err := client.GetBlockExists(sampleBlockRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	if retBlockExists {
		t.Fail()
	}

	// Put the block
	wroteRef, existed, err := client.PutBlock(sampleBlockBody, samplePutOpts)
	if err != nil {
		t.Fatal(err.Error())
	}
	if existed || !wroteRef.EqualsRef(sampleBlockRef) {
		t.Fail()
	}

	// Get a not-found block
	// Returns a 404 error, but this is processed to exists=false.
	sampleBlockBody2 := []byte("I seem to be getting a distress signal from that aerial faith plate...")
	sampleBlockRef2, err := block.BuildBlockRef(sampleBlockBody2, samplePutOpts)
	if err != nil {
		t.Fatal(err.Error())
	}
	retBlockData, retBlockExists, err := client.GetBlock(sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if retBlockExists || len(retBlockData) != 0 {
		t.Fail()
	}

	// Check if the block exists (not expected to)
	retBlockExists, err = client.GetBlockExists(sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if retBlockExists {
		t.Fail()
	}

	// Put the block
	samplePutOpts2 := samplePutOpts.CloneVT()
	samplePutOpts2.ForceBlockRef = sampleBlockRef2.Clone()
	ref, existed, err := client.PutBlock(sampleBlockBody2, samplePutOpts2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := ref.Validate(); err != nil {
		t.Fatal(err.Error())
	}
	if !ref.EqualsRef(sampleBlockRef2) {
		t.Fail()
	}
	if existed {
		t.Fail()
	}

	// Get the block back again
	retBlockData, retBlockExists, err = client.GetBlock(sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !retBlockExists || !bytes.Equal(retBlockData, sampleBlockBody2) {
		t.Fail()
	}

	// Check if the block exists
	retBlockExists, err = client.GetBlockExists(sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !retBlockExists {
		t.Fail()
	}

	// Delete the block(s)
	err = client.RmBlock(sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = client.RmBlock(sampleBlockRef)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Check if the block exists
	retBlockExists, err = client.GetBlockExists(sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if retBlockExists {
		t.Fail()
	}
}
