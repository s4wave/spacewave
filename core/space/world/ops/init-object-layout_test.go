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
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer btb.Release()

	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	objKey := "object-layout/main"
	if _, _, err := InitObjectLayout(ctx, ws, wtb.Volume.GetPeerID(), objKey, time.Now()); err != nil {
		t.Fatal(err)
	}

	objectType, err := world_types.GetObjectType(ctx, ws, objKey)
	if err != nil {
		t.Fatal(err)
	}
	if objectType != s4wave_layout_world.ObjectLayoutTypeID {
		t.Fatalf("object type = %q, want %q", objectType, s4wave_layout_world.ObjectLayoutTypeID)
	}

	layout, _, err := s4wave_layout_world.LookupObjectLayout(ctx, ws, objKey)
	if err != nil {
		t.Fatal(err)
	}
	row := layout.GetLayoutModel().GetLayout()
	if row.GetId() != "root" {
		t.Fatalf("layout root id = %q, want root", row.GetId())
	}
	if got := len(row.GetChildren()); got != 1 {
		t.Fatalf("layout root children = %d, want 1", got)
	}
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
	tab := tabSet.GetChildren()[0]
	if tab.GetId() != "files" {
		t.Fatalf("tab id = %q, want files", tab.GetId())
	}
	if tab.GetName() != "Files" {
		t.Fatalf("tab name = %q, want Files", tab.GetName())
	}
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
