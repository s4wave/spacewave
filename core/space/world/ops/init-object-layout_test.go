package space_world_ops

import (
	"context"
	"testing"
	"time"

	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	"github.com/sirupsen/logrus"
)

func TestInitObjectLayoutCreatesFilesTab(t *testing.T) {
	// Prepare the context and logger for object layout initialization.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for the object layout fixture.
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer btb.Release()

	// Start the World engine over the storage testbed.
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Initialize an object layout in the World.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	objKey := "object-layout/main"
	if _, _, err := InitObjectLayout(ctx, ws, wtb.Volume.GetPeerID(), objKey, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Verify the created object has the layout type.
	objectType, err := world_types.GetObjectType(ctx, ws, objKey)
	if err != nil {
		t.Fatal(err)
	}
	if objectType != s4wave_layout_world.ObjectLayoutTypeID {
		t.Fatalf("object type = %q, want %q", objectType, s4wave_layout_world.ObjectLayoutTypeID)
	}

	// Load the created object layout from the World.
	layout, objectState, err := s4wave_layout_world.LookupObjectLayout(ctx, ws, objKey)
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the layout contains one root row child.
	row := layout.GetLayoutModel().GetLayout()
	if row.GetId() != "root" {
		t.Fatalf("layout root id = %q, want root", row.GetId())
	}
	if got := len(row.GetChildren()); got != 1 {
		t.Fatalf("layout root children = %d, want 1", got)
	}

	// Verify the root child is the main tab set with one tab.
	tabSet := row.GetChildren()[0].GetTabSet()
	if tabSet == nil {
		t.Fatalf("layout first child is %T, want tabset", row.GetChildren()[0].GetNode())
	}
	if tabSet.GetId() != "main-tabset" {
		t.Fatalf("tabset id = %q, want main-tabset", tabSet.GetId())
	}
	if got := len(tabSet.GetChildren()); got != 1 {
		t.Fatalf("tabset children = %d, want 1", got)
	}

	// Verify the initial tab is the Files tab.
	tab := tabSet.GetChildren()[0]
	if tab.GetId() != "files" {
		t.Fatalf("tab id = %q, want files", tab.GetId())
	}
	if tab.GetName() != "Files" {
		t.Fatalf("tab name = %q, want Files", tab.GetName())
	}

	// Decode the Files tab data and verify it references the files object.
	var tabData s4wave_layout_world.ObjectLayoutTab
	if err := tabData.UnmarshalVT(tab.GetData()); err != nil {
		t.Fatal(err)
	}
	worldInfo := tabData.GetObjectInfo().GetWorldObjectInfo()
	if worldInfo == nil {
		t.Fatalf("tab data object info is %T, want WorldObjectInfo", tabData.GetObjectInfo().GetInfo())
	}
	if worldInfo.GetObjectKey() != "files" {
		t.Fatalf("tab object key = %q, want files", worldInfo.GetObjectKey())
	}
}
