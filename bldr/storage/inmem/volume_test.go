package storage_inmem

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/bldr/core"
	"github.com/s4wave/spacewave/bldr/storage"
	storage_controller "github.com/s4wave/spacewave/bldr/storage/controller"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/sirupsen/logrus"
)

// TestVolumeRestart checks that a restarted volume controller reopens the
// volume's identity and data, and that DeleteVolume drops them.
func TestVolumeRestart(t *testing.T) {
	// Bound the test and build the core bus with the in-memory storage attached.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	le := logrus.NewEntry(logrus.New())
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}

	// Attach the in-memory storage and its storage controller to the bus.
	st := NewInmemStorage("test")
	st.AddFactories(b, sr)
	info := controller.NewInfo(ControllerID, Version, "")
	release, err := b.AddController(ctx, storage_controller.BuildStorageController("test", []storage.Storage{st}, info), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	// Start the volume, record its peer id, and write one synced block.
	vol, stop := startTestVolume(ctx, t, b, st)
	peerID := vol.GetPeerID()
	ref, _, err := vol.PutBlock(ctx, []byte("kept"), &block.PutOpts{Sync: true})
	if err != nil {
		t.Fatal(err)
	}
	stop()

	// Restart the volume and assert it keeps its peer id and block.
	vol, stop = startTestVolume(ctx, t, b, st)
	if got := vol.GetPeerID(); got != peerID {
		t.Fatalf("restarted peer id = %s, want %s", got, peerID)
	}
	found, err := vol.GetBlockExists(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("restarted volume lost its block")
	}
	stop()

	// Delete the volume and assert a fresh volume gets a new peer id.
	if err := st.DeleteVolume("vol"); err != nil {
		t.Fatal(err)
	}
	vol, stop = startTestVolume(ctx, t, b, st)
	defer stop()
	if vol.GetPeerID() == peerID {
		t.Fatal("deleted volume kept its peer id")
	}
}

// startTestVolume runs a volume controller for the volume "vol" and returns
// its volume with a function that stops the controller.
func startTestVolume(ctx context.Context, t *testing.T, b bus.Bus, st *InmemStorage) (volume.Volume, func()) {
	// Mark the helper and build the volume config and its controller.
	t.Helper()

	// Build the volume config and construct its controller.
	conf, err := st.BuildVolumeConfig("vol", &volume_controller.Config{GcIntervalDur: "0"})
	if err != nil {
		t.Fatal(err)
	}
	ctrl, err := NewVolumeFactory(b).Construct(ctx, conf, controller.ConstructOpts{})
	if err != nil {
		t.Fatal(err)
	}

	// Add the controller to the bus and wait for its volume.
	release, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	vol, err := ctrl.(volume.Controller).GetVolume(ctx)
	if err != nil {
		release()
		t.Fatal(err)
	}
	return vol, release
}
