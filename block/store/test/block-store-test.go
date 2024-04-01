package block_store_test

import (
	"bytes"
	"context"
	"time"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
)

// TestAll tests all tests for a block store.
func TestAll(ctx context.Context, client block.StoreOps, putDelay time.Duration) error {
	waitAfterPut := func() {
		if putDelay != 0 {
			select {
			case <-time.After(putDelay):
			case <-ctx.Done():
			}
		}
	}

	sampleBlockBody := []byte("How hard are these tests? What exactly was in that phonebook of a contract I signed?")
	samplePutOpts := &block.PutOpts{HashType: hash.HashType_HashType_BLAKE3}
	sampleBlockRef, err := block.BuildBlockRef(sampleBlockBody, samplePutOpts)
	if err != nil {
		return err
	}

	// Check if the block exists
	retBlockExists, err := client.GetBlockExists(ctx, sampleBlockRef)
	if err != nil {
		return err
	}
	if retBlockExists {
		return errors.New("block existed before being put")
	}

	// Put the block
	wroteRef, existed, err := client.PutBlock(ctx, sampleBlockBody, samplePutOpts)
	if err != nil {
		return err
	}
	if existed || !wroteRef.EqualsRef(sampleBlockRef) {
		return errors.Errorf("wrote %s but expected %s", wroteRef.MarshalString(), sampleBlockRef.MarshalString())
	}

	waitAfterPut()

	// Get a not-found block
	// Returns a 404 error, but this is processed to exists=false.
	sampleBlockBody2 := []byte("I seem to be getting a distress signal from that aerial faith plate...")
	sampleBlockRef2, err := block.BuildBlockRef(sampleBlockBody2, samplePutOpts)
	if err != nil {
		return err
	}
	retBlockData, retBlockExists, err := client.GetBlock(ctx, sampleBlockRef2)
	if err != nil {
		return err
	}
	if retBlockExists || len(retBlockData) != 0 {
		return errors.New("expected block to not exist")
	}

	// Check if the block exists (not expected to)
	retBlockExists, err = client.GetBlockExists(ctx, sampleBlockRef2)
	if err != nil {
		return err
	}
	if retBlockExists {
		return errors.New("expected block to not exist")
	}

	// Put the block
	samplePutOpts2 := samplePutOpts.CloneVT()
	samplePutOpts2.ForceBlockRef = sampleBlockRef2.Clone()
	ref, existed, err := client.PutBlock(ctx, sampleBlockBody2, samplePutOpts2)
	if err != nil {
		return err
	}
	if err := ref.Validate(false); err != nil {
		return err
	}
	if !ref.EqualsRef(sampleBlockRef2) {
		return errors.Errorf("expected ref %s but got %s", sampleBlockRef2.MarshalString(), ref.MarshalString())
	}
	if existed {
		return errors.New("expected block to not have already existed")
	}

	waitAfterPut()

	// Get the block back again
	retBlockData, retBlockExists, err = client.GetBlock(ctx, sampleBlockRef2)
	if err != nil {
		return err
	}
	if !retBlockExists || !bytes.Equal(retBlockData, sampleBlockBody2) {
		return errors.New("expected block to exist")
	}

	// Check if the block exists
	retBlockExists, err = client.GetBlockExists(ctx, sampleBlockRef2)
	if err != nil {
		return err
	}
	if !retBlockExists {
		return errors.New("expected block to exist")
	}

	// Delete the block(s)
	err = client.RmBlock(ctx, sampleBlockRef2)
	if err != nil {
		return err
	}
	err = client.RmBlock(ctx, sampleBlockRef)
	if err != nil {
		return err
	}

	waitAfterPut()

	// Check if the block exists
	retBlockExists, err = client.GetBlockExists(ctx, sampleBlockRef2)
	if err != nil {
		return err
	}
	if retBlockExists {
		return errors.New("expected block to not exist")
	}
	return nil
}
