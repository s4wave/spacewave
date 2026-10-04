package blocktype_controller

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/blocktype"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestController tests the blocktype controller.
func TestController(t *testing.T) {
	// Prepare the controller test context and logger.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage and bus testbed for blocktype lookup.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Read the testbed bus and retain its static resolver for the fixture.
	b, sr := tb.Bus, tb.StaticResolver
	_ = sr

	// Capture the identifier the controller must resolve.
	spaceSettingsTypeID := space_world.SpaceSettingsBlockType.GetBlockTypeID()

	// Resolve the registered SpaceSettings type from the controller callback.
	lookupFunc := func(ctx context.Context, typeID string) (blocktype.BlockType, error) {
		if typeID == spaceSettingsTypeID {
			return space_world.SpaceSettingsBlockType, nil
		}
		return nil, nil
	}

	// Attach the controller that serves the SpaceSettings type lookup.
	c := NewController(lookupFunc)
	releaseCtrl, err := b.AddController(ctx, c, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer releaseCtrl()

	// Resolve SpaceSettings through the public blocktype directive.
	result, ref, err := blocktype.ExLookupBlockType(ctx, b, spaceSettingsTypeID)
	if err != nil {
		t.Fatalf("failed to lookup block type: %v", err)
	}
	if ref != nil {
		defer ref.Release()
	}
	if result == nil {
		t.Fatal("expected block type, got nil")
	}

	// Verify the resolved record carries the requested type identifier.
	if result.GetBlockTypeID() != spaceSettingsTypeID {
		t.Fatalf("expected type ID %q, got %q", spaceSettingsTypeID, result.GetBlockTypeID())
	}

	// Construct the block value represented by the resolved type.
	constructed := result.Constructor()
	if constructed == nil {
		t.Fatal("expected constructed block, got nil")
	}

	// Require the constructor to return a SpaceSettings block.
	spaceSettings, ok := constructed.(*space_world.SpaceSettings)
	if !ok {
		t.Fatalf("expected *SpaceSettings, got %T", constructed)
	}

	// Confirm the constructed block matches its registered type.
	if !result.MatchesBlockType(spaceSettings) {
		t.Fatal("constructed block should match its type")
	}

	// Serialize a populated SpaceSettings value through the block interface.
	spaceSettings.IndexPath = "/test"
	data, err := spaceSettings.MarshalBlock()
	if err != nil {
		t.Fatalf("failed to marshal block: %v", err)
	}

	// Decode the serialized block into a fresh SpaceSettings value.
	spaceSettings2 := &space_world.SpaceSettings{}
	if err := spaceSettings2.UnmarshalBlock(data); err != nil {
		t.Fatalf("failed to unmarshal block: %v", err)
	}

	// Verify that block serialization preserved the configured path.
	if spaceSettings2.IndexPath != "/test" {
		t.Fatalf("expected IndexPath %q, got %q", "/test", spaceSettings2.IndexPath)
	}

	// Verify that an unknown type resolves to no value and no error.
	dir := blocktype.NewLookupBlockType("nonexistent.type")
	val, _, ref2, err := bus.ExecOneOffTyped[blocktype.BlockType](
		ctx,
		b,
		dir,
		bus.ReturnWhenIdle(),
		nil,
	)
	if err != nil {
		t.Fatalf("expected nil error for nonexistent type, got: %v", err)
	}
	if val != nil && ref2 != nil {
		ref2.Release()
	}
	if val != nil {
		t.Fatal("expected nil value for nonexistent type")
	}
}
