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
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	b, sr := tb.Bus, tb.StaticResolver
	_ = sr

	spaceSettingsTypeID := space_world.SpaceSettingsBlockType.GetBlockTypeID()

	lookupFunc := func(ctx context.Context, typeID string) (blocktype.BlockType, error) {
		if typeID == spaceSettingsTypeID {
			return space_world.SpaceSettingsBlockType, nil
		}
		return nil, nil
	}

	c := NewController(lookupFunc)
	releaseCtrl, err := b.AddController(ctx, c, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer releaseCtrl()

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

	if result.GetBlockTypeID() != spaceSettingsTypeID {
		t.Fatalf("expected type ID %q, got %q", spaceSettingsTypeID, result.GetBlockTypeID())
	}

	constructed := result.Constructor()
	if constructed == nil {
		t.Fatal("expected constructed block, got nil")
	}

	spaceSettings, ok := constructed.(*space_world.SpaceSettings)
	if !ok {
		t.Fatalf("expected *SpaceSettings, got %T", constructed)
	}

	if !result.MatchesBlockType(spaceSettings) {
		t.Fatal("constructed block should match its type")
	}

	spaceSettings.IndexPath = "/test"
	data, err := spaceSettings.MarshalBlock()
	if err != nil {
		t.Fatalf("failed to marshal block: %v", err)
	}

	spaceSettings2 := &space_world.SpaceSettings{}
	if err := spaceSettings2.UnmarshalBlock(data); err != nil {
		t.Fatalf("failed to unmarshal block: %v", err)
	}

	if spaceSettings2.IndexPath != "/test" {
		t.Fatalf("expected IndexPath %q, got %q", "/test", spaceSettings2.IndexPath)
	}

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
