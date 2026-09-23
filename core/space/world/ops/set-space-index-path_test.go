package space_world_ops

import (
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	space_world "github.com/s4wave/spacewave/core/space/world"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

func TestSpaceIndexRepairPreservesConcurrentPluginInstallation(t *testing.T) {
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

	// The repair was chosen before a collaborator installed a plugin.
	repair := &SetSpaceIndexPathOp{
		IndexPath:         "files-1",
		ExpectedIndexPath: new("files"),
		Timestamp:         timestamp.New(time.Now()),
	}
	current := &space_world.SpaceSettings{
		IndexPath: "files",
		PluginIds: []string{"colors"},
		PluginInstallations: map[string]*space_world.SpacePluginInstallation{
			"colors": {ManifestKeys: []string{"plugins/colors/v1"}},
		},
	}
	if _, _, err := SetSpaceSettings(ctx, ws, sender, "", current, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ws.ApplyWorldOp(ctx, repair, sender); err != nil {
		t.Fatal(err)
	}

	// Only the requested field changes; the later installation stays accepted.
	current.IndexPath = "files-1"
	check := func() {
		t.Helper()
		actual, err := space_world.LookupSpaceSettingsBody(ctx, ws)
		if err != nil {
			t.Fatal(err)
		}
		if !actual.EqualVT(current) {
			t.Fatalf("settings changed outside the index path: got %v, want %v", actual, current)
		}
	}
	check()

	// Replaying a stale automatic repair preserves a newer index selection.
	repair.IndexPath = "stale-fallback"
	if _, _, err := ws.ApplyWorldOp(ctx, repair, sender); err != nil {
		t.Fatal(err)
	}
	check()

	// An explicit selection can clear the index without clearing plugin state.
	repair.ExpectedIndexPath = nil
	repair.IndexPath = ""
	if _, _, err := ws.ApplyWorldOp(ctx, repair, sender); err != nil {
		t.Fatal(err)
	}
	current.IndexPath = ""
	check()
}
