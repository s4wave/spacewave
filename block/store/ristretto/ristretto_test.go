package block_store_ristretto

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/sirupsen/logrus"
)

// TestBlockStoreRistretto tests the ristretto block store.
func TestBlockStoreRistretto(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	storeID := "test-store"
	ctrl := NewController(le, &Config{BlockStoreId: storeID})
	storeProm, storeRef := ctrl.AddBlockStoreRef()
	defer storeRef.Release()

	go ctrl.Execute(ctx)

	clientPtr, err := storeProm.Await(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	client := *clientPtr

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

	// NOTE: Ristretto is a concurrent cache.
	// It sometimes takes some time for an operation to be applied.
	// Delays are added here to compensate for that, although non-ideal.

	// Put the block
	wroteRef, existed, err := client.PutBlock(sampleBlockBody, samplePutOpts)
	if err != nil {
		t.Fatal(err.Error())
	}
	if existed || !wroteRef.EqualsRef(sampleBlockRef) {
		t.Fail()
	}

	<-time.After(time.Millisecond * 100)

	retBlockExists, err = client.GetBlockExists(sampleBlockRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !retBlockExists {
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

	<-time.After(time.Millisecond * 100)

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

	<-time.After(time.Millisecond * 100)

	// Check if the block exists
	retBlockExists, err = client.GetBlockExists(sampleBlockRef2)
	if err != nil {
		t.Fatal(err.Error())
	}
	if retBlockExists {
		t.Fail()
	}
}
