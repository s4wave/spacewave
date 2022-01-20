package forge_target

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/bifrost/util/prng"
	hydra_all "github.com/aperturerobotics/hydra/core/all"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/aperturerobotics/timestamp"
	"github.com/sirupsen/logrus"
)

// buildTestbedHandle builds a testbed with a handle.
func buildTestbedHandle(t *testing.T) (*testbed.Testbed, world.WorldState, ExecControllerHandle) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	hydra_all.AddFactories(tb.Bus, tb.StaticResolver)

	// construct & mount world controller
	engineID := "forge-target-test"
	volumeID := tb.Volume.GetID()
	bucketID := testbed.BucketId
	objectStoreID := engineID
	worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(
		ctx,
		tb.Bus,
		world_block_engine.NewConfig(
			engineID,
			volumeID, bucketID,
			objectStoreID,
			nil,
			nil,
		),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	_ = worldCtrlRef
	// defer worldCtrlRef.Release()

	wh, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	// defer wh.Release()

	worldState := world.NewEngineWorldState(ctx, wh, true)
	ts := timestamp.Now()
	handle := ExecControllerHandleWithAccess(tb.Volume.GetPeerID(), wh, worldState.AccessWorldState, &ts)
	return tb, worldState, handle
}

// TestStoreBlobValue tests storing a byte slice as a blob value.
func TestStoreBlobValue(t *testing.T) {
	tb, _, handle := buildTestbedHandle(t)
	ctx := tb.Context

	// Test storing large value
	rnd := prng.BuildSeededRand([]byte("test-store-blob-value"))
	dat := make([]byte, 250000) // 250kb
	_, err := io.ReadFull(rnd, dat)
	if err != nil {
		t.Fatal(err.Error())
	}
	fv, err := StoreBlobValueFromBytes(ctx, handle, dat)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Test loading value
	outData, err := LoadBlobValueToBytes(ctx, handle, fv)
	if err != nil {
		t.Fatal(err.Error())
	}
	if bytes.Compare(outData, dat) != 0 {
		t.Fatalf("output value was different: len(%d) and expected len(%d)", len(outData), len(dat))
	}
}

// TestStoreMsgpackBlobValue tests storing a blob value.
func TestStoreMsgpackBlobValue(t *testing.T) {
	tb, _, handle := buildTestbedHandle(t)
	ctx := tb.Context

	// Test storing value
	testValue := map[string]int{"test": 2}
	fv, err := StoreMsgpackBlobValue(ctx, handle, testValue)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Test loading value
	var outValue map[string]int
	ov, err := LoadMsgpackBlobValue(ctx, handle, fv, &outValue)
	if err != nil {
		t.Fatal(err.Error())
	}
	loaded, _ := ov.(*map[string]int)
	if loaded == nil || (*loaded)["test"] != 2 {
		t.Fatalf("output value was different: %#v", ov)
	}
}

// TestStoreMsgpackBlockValue tests storing a msgpack block
func TestStoreMsgpackBlockValue(t *testing.T) {
	tb, _, handle := buildTestbedHandle(t)
	ctx := tb.Context

	// Test storing value directly in block
	testValue := map[string]int{"test": 2}
	fv, err := StoreMsgpackValue(ctx, handle, testValue)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Test loading value
	var outValue map[string]int
	ov, err := LoadMsgpackValue(ctx, handle, fv, &outValue)
	if err != nil {
		t.Fatal(err.Error())
	}
	loaded, _ := ov.(*map[string]int)
	if loaded == nil || (*loaded)["test"] != 2 {
		t.Fatalf("output value was different: %#v", ov)
	}
}
