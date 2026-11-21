package forge_target

import (
	"bytes"
	"context"
	"io"
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/prng"
	hydra_all "github.com/s4wave/spacewave/db/core/all"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

// buildTestbedHandle builds a testbed with a handle.
func buildTestbedHandle(t *testing.T) (*testbed.Testbed, world.WorldState, ExecControllerHandle) {
	// Create the context and logger for the execution-handle testbed.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed and register its controller factories.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)
	hydra_all.AddFactories(tb.Bus, tb.StaticResolver)

	// Construct and mount the world controller used by the handle.
	engineID := "forge-target-test"
	volumeID := tb.Volume.GetID()
	bucketID := tb.BucketId
	objectStoreID := engineID
	wh, worldCtrlRef, err := world.StartEngine(
		ctx,
		tb.Bus,
		world_block_engine.NewConfig(
			engineID,
			volumeID, bucketID,
			objectStoreID,
			nil,
			nil,
			false,
		),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(worldCtrlRef.Release)

	// Construct the execution handle over a stage of the mounted World state
	// held until the test ends.
	worldState := world.NewEngineWorldState(wh, true)
	stage, err := worldState.StageWorldState(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(stage.Release)
	ts := timestamp.Now()
	uniqueID := "test-handle"
	handle := ExecControllerHandleWithAccess(uniqueID, tb.Volume.GetPeerID(), wh, stage.AccessWorldState, ts)
	return tb, worldState, handle
}

// TestStoreBlobValue tests storing a byte slice as a blob value.
func TestStoreBlobValue(t *testing.T) {
	// Create the storage testbed and execution handle for the blob round trip.
	tb, _, handle := buildTestbedHandle(t)
	ctx := tb.Context

	// Store a large blob value.
	rnd := prng.BuildSeededReader([]byte("test-store-blob-value"))

	// Fill a large byte slice from the seeded blob reader.
	dat := make([]byte, 250000) // 250kb
	_, err := io.ReadFull(rnd, dat)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Store the large byte slice as a blob value.
	fv, err := StoreBlobValueFromBytes(ctx, handle, dat)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Load the blob and compare its bytes with the stored value.
	outData, err := LoadBlobValueToBytes(ctx, handle, fv)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the loaded blob matches the stored bytes.
	if !bytes.Equal(outData, dat) {
		t.Fatalf("output value was different: len(%d) and expected len(%d)", len(outData), len(dat))
	}
}
