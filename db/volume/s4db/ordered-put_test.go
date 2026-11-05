//go:build !js && !wasip1

package volume_s4db_test

import (
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/core"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// TestOrderedPut checks that block writes made with an ordered-commit context
// flush nothing to the drive, while plain block writes do.
func TestOrderedPut(t *testing.T) {
	// Register the s4db volume factory on a test bus.
	ctx := t.Context()
	b, sr, err := core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	sr.AddFactory(volume_s4db.NewFactory(b))

	// Load a volume and reach its database.
	conf := &volume_s4db.Config{
		Path:         filepath.Join(t.TempDir(), "volume.s4wave"),
		VolumeConfig: &volume_controller.Config{GcIntervalDur: "1h"},
	}
	ctrl, _, ref, err := loader.WaitExecControllerRunningTyped[volume.Controller](
		ctx, b, resolver.NewLoadControllerWithConfig(conf), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()
	vol, err := ctrl.GetVolume(ctx)
	if err != nil {
		t.Fatal(err)
	}
	db := volume_s4db.GetDB(vol)

	// Count the database's flushes to the drive.
	flushes := func() uint64 {
		st, err := db.Stats(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return st.Flushes
	}
	before := flushes()

	// Ordered writes of a block and a batch flush nothing.
	orderedCtx := kvtx.WithOrderedCommitContext(ctx)
	if _, _, err := block.PutBlock(orderedCtx, vol, block_mock.NewExample("ordered")); err != nil {
		t.Fatal(err)
	}
	batch := []*block.PutBatchEntry{{Data: []byte("batch"), RefsKnown: true}}
	batch[0].Ref, err = block.BuildBlockRef(batch[0].Data, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vol.PutBlockBatch(orderedCtx, batch); err != nil {
		t.Fatal(err)
	}
	if after := flushes(); after != before {
		t.Fatalf("ordered writes flushed %d times", after-before)
	}

	// A plain write flushes.
	if _, _, err := block.PutBlock(ctx, vol, block_mock.NewExample("durable")); err != nil {
		t.Fatal(err)
	}
	if after := flushes(); after == before {
		t.Fatal("plain write did not flush")
	}
}
