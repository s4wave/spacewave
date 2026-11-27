package space_world_ops

import (
	"slices"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	space_world "github.com/s4wave/spacewave/core/space/world"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

// TestAddSpacePluginsKeepsEarlierAddsAndIndexPath applies two AddSpacePluginsOps
// one after the other and checks that the second keeps the plugin ids the first
// added and the stored index path, and that a stored invalid id is dropped.
func TestAddSpacePluginsKeepsEarlierAddsAndIndexPath(t *testing.T) {
	// Start the World engine over the storage testbed.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	wtb, err := world_testbed.NewTestbed(tb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()
	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()

	// Two writers each read settings listing only "colors" and build an add.
	// The stored list also holds an ID a corrupted block decoded.
	if _, _, err := SetSpaceSettings(ctx, ws, sender, "", &space_world.SpaceSettings{
		IndexPath: "files",
		PluginIds: []string{"colors", "\b\x02Bad/ID"},
	}, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	addNotes := &AddSpacePluginsOp{PluginIds: []string{"notes"}, Timestamp: timestamp.New(time.Now())}
	addSQL := &AddSpacePluginsOp{PluginIds: []string{"sql", "colors"}, Timestamp: timestamp.New(time.Now())}

	// Both apply. Neither drops the other's plugin or clears the index
	// path, and the invalid ID is not written back.
	for _, op := range []*AddSpacePluginsOp{addNotes, addSQL} {
		if err := op.Validate(); err != nil {
			t.Fatal(err)
		}
		if _, _, err := ws.ApplyWorldOp(ctx, op, sender); err != nil {
			t.Fatal(err)
		}
	}
	actual, err := space_world.LookupSpaceSettingsBody(ctx, ws)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"colors", "notes", "sql"}; !slices.Equal(actual.GetPluginIds(), want) {
		t.Fatalf("plugin ids: got %v, want %v", actual.GetPluginIds(), want)
	}
	if actual.GetIndexPath() != "files" {
		t.Fatalf("index path: got %q, want files", actual.GetIndexPath())
	}
}
