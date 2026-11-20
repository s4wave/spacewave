package resource_space

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	unixfs_linkedfolder "github.com/s4wave/spacewave/sdk/unixfs/linkedfolder"
)

// testRunningLink is a linked folder running on the test daemon.
type testRunningLink struct {
	conf     *unixfs_linkedfolder.Config
	progress unixfs_linkedfolder.LinkProgress
}

func (l *testRunningLink) GetConfig() *unixfs_linkedfolder.Config { return l.conf }

func (l *testRunningLink) GetLinkProgress() (unixfs_linkedfolder.LinkProgress, <-chan struct{}) {
	return l.progress, make(chan struct{})
}

// TestDriveLinks reads two linked-folder nodes, one on each of two Devices,
// that link the same folder path into one Drive, joins the link running on
// this Device to its own row only, and wakes on a Device's transition.
func TestDriveLinks(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	engine := tb.Engine

	// Seed two Devices showing their nodes indexing and linked, and a
	// Flowgraph placing one node on each beside a node writing another Drive.
	const graphKey = "flowgraph/main"
	capID := func(nodeID string) string { return s4wave_flowgraph.NodeCapabilityID(graphKey, nodeID) }
	devices := map[string]*s4wave_device.Device{
		"devices/self":  {PeerId: "peer-self", Capabilities: []*s4wave_device.DeviceCapability{{Id: capID("link-self"), State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE, Detail: unixfs_linkedfolder.StatusIndexing}}},
		"devices/other": {PeerId: "peer-other", Capabilities: []*s4wave_device.DeviceCapability{{Id: capID("link-other"), State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE, Detail: "watching; 1 file changing, will link when quiet"}}},
	}
	commitDriveLinksTx(t, ctx, engine, func(tx world.Tx) error {
		for key, device := range devices {
			if err := putTestDevice(ctx, tx, key, device); err != nil {
				return err
			}
		}
		if _, _, err := tx.ApplyWorldOp(ctx, &s4wave_flowgraph.CreateFlowgraphOp{ObjectKey: graphKey, Name: "main"}, ""); err != nil {
			return err
		}
		params := map[string]string{unixfs_linkedfolder.ParamFolder: "/photos", unixfs_linkedfolder.ParamPath: "trips"}
		_, err := s4wave_flowgraph.UpdateFlowgraph(ctx, tx, graphKey, &s4wave_flowgraph.UpdateFlowgraphRequest{
			SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{
				"link-self":  {TypeId: unixfs_linkedfolder.NodeTypeID, ObjectKey: "drive", Parameters: params},
				"link-other": {TypeId: unixfs_linkedfolder.NodeTypeID, ObjectKey: "drive", Parameters: params},
				"elsewhere":  {TypeId: unixfs_linkedfolder.NodeTypeID, ObjectKey: "other-drive", Parameters: params},
			},
			SetPlacements: map[string]*s4wave_flowgraph.FlowgraphPlacements{
				"link-self":  {Copies: []*s4wave_flowgraph.FlowgraphPlacement{testDevicePlacement("devices/self")}},
				"link-other": {Copies: []*s4wave_flowgraph.FlowgraphPlacement{testDevicePlacement("devices/other")}},
				"elsewhere":  {Copies: []*s4wave_flowgraph.FlowgraphPlacement{testDevicePlacement("devices/self")}},
			},
		})
		return err
	})

	// Read the Drive's links: one per Device.
	links, reads, seqno, err := readDriveLinks(ctx, engine, "drive")
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 2 {
		t.Fatalf("links: got %d, want 2", len(links))
	}
	byKey := map[string]*s4wave_space.DriveLink{}
	for _, l := range links {
		byKey[l.link.GetDeviceKey()] = l.link
	}
	if got := byKey["devices/self"].GetState(); got != s4wave_space.DriveLinkState_DriveLinkState_INDEXING {
		t.Fatalf("self state: %v", got)
	}
	if got := byKey["devices/other"].GetState(); got != s4wave_space.DriveLinkState_DriveLinkState_LINKED {
		t.Fatalf("other state: %v", got)
	}

	// Join the link running here: only this Device's row carries its counts.
	running := []unixfs_linkedfolder.RunningLink{&testRunningLink{
		conf:     &unixfs_linkedfolder.Config{Folder: "/photos", Path: "trips", DevicePeerId: "peer-self"},
		progress: unixfs_linkedfolder.LinkProgress{Files: 12, Bytes: 3400},
	}}
	resp, waits := joinLiveLinks(links, running, nil, false)
	if len(waits) != 1 {
		t.Fatalf("waits: got %d, want 1", len(waits))
	}
	for _, l := range resp.GetLinks() {
		local := l.GetLocal()
		switch l.GetDeviceKey() {
		case "devices/self":
			if local.GetFiles() != 12 || local.GetBytes() != 3400 || local.GetFolder() != "/photos" {
				t.Fatalf("self local: %v", local)
			}
		default:
			if local != nil {
				t.Fatalf("other Device shows local counts: %v", local)
			}
		}
	}

	// A Device's transition ends the read set's wait.
	waited := make(chan error, 1)
	go func() {
		_, err := reads.WaitChange(ctx, engine, seqno)
		waited <- err
	}()
	devices["devices/other"].Capabilities[0].State = s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_REJECTED
	commitDriveLinksTx(t, ctx, engine, func(tx world.Tx) error {
		return putTestDevice(ctx, tx, "devices/other", devices["devices/other"])
	})
	if err := <-waited; err != nil {
		t.Fatal(err)
	}
	links, _, _, err = readDriveLinks(ctx, engine, "drive")
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range links {
		if l.link.GetDeviceKey() == "devices/other" && l.link.GetState() != s4wave_space.DriveLinkState_DriveLinkState_REJECTED {
			t.Fatalf("other state after rejection: %v", l.link.GetState())
		}
	}
}

// putTestDevice writes a Device object at key.
func putTestDevice(ctx context.Context, tx world.Tx, key string, device *s4wave_device.Device) error {
	if _, _, err := world.AccessWorldObject(ctx, tx, key, true, func(cursor *block.Cursor) error {
		cursor.SetBlock(device.CloneVT(), true)
		return nil
	}); err != nil {
		return err
	}
	return world_types.SetObjectType(ctx, tx, key, s4wave_device.DeviceTypeID)
}

// testDevicePlacement places a node on the Device at key.
func testDevicePlacement(key string) *s4wave_flowgraph.FlowgraphPlacement {
	return &s4wave_flowgraph.FlowgraphPlacement{Kind: s4wave_flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE, ObjectKey: key}
}

// commitDriveLinksTx runs fn in a write transaction and commits it.
func commitDriveLinksTx(t *testing.T, ctx context.Context, engine world.Engine, fn func(tx world.Tx) error) {
	t.Helper()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := fn(tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
