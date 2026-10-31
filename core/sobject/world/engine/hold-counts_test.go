//go:build !js

package sobject_world_engine_test

import (
	"math/rand/v2"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/s4wave/spacewave/bldr/storage"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	"github.com/s4wave/spacewave/core/sobject"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
)

// TestHoldCountsOff runs the same random writes on a Volume that keeps shared
// counts and on one whose Config turns them off. The first stores shared
// counts and its engine names deadlist records; the second does neither.
func TestHoldCountsOff(t *testing.T) {
	for _, off := range []bool{false, true} {
		// Start a World on a Volume with the switch set.
		st := holdCountsStorage{Storage: storage_native.NewS4db(false, t.TempDir()), off: off}
		w, so := startGatedWorldOn(t, st, nil, nil)
		r := &deadlistRun{t: t, w: w, so: so, rng: rand.New(rand.NewPCG(1, 0)), checked: make(map[uint64]bool)}

		// Run the steps and enough writes for a checkpoint after them,
		// noting the records named and the most counts stored at once.
		var holds int
		note := func() {
			r.check()
			holds = max(holds, countHolds(t, w))
		}
		for range 120 {
			r.step()
			note()
		}
		for range 2 * sobject.MinCheckpointOperations {
			r.create(false)
		}
		waitSeqIdle(t, w)
		note()

		// Check the Volume's counts and the engine's records.
		t.Logf("off %v: records %d, holds %d, steps %v", off, len(r.checked), holds, r.ran)
		if off && (holds != 0 || len(r.checked) != 0) {
			t.Fatalf("with counts off the Volume stored %d shared counts and the engine named %d deadlist records", holds, len(r.checked))
		}
		if !off && (holds == 0 || len(r.checked) == 0) {
			t.Fatalf("with counts on the Volume stored %d shared counts and the engine named %d deadlist records", holds, len(r.checked))
		}
	}
}

// holdCountsStorage is s4db storage whose Volumes keep shared counts unless
// off is set.
type holdCountsStorage struct {
	storage.Storage
	off bool
}

// BuildVolumeConfig builds the s4db Volume config with the switch set.
func (s holdCountsStorage) BuildVolumeConfig(id string, base *volume_controller.Config) (config.Config, error) {
	conf, err := s.Storage.BuildVolumeConfig(id, base)
	if c, ok := conf.(*volume_s4db.Config); ok {
		c.StoreConfig = &store_kvtx.Config{DisableHoldCounts: s.off}
	}
	return conf, err
}

// countHolds counts the shared count records in the account Volume's graph.
// The graph's records sit under the key byte 2, and a shared count's kind byte
// is 'h'.
func countHolds(t *testing.T, w *localWorld) int {
	// Read the graph in one transaction.
	t.Helper()
	ctx := t.Context()
	tx, err := w.acc.GetVolume().(kvtx_volume.KvtxVolume).GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Count the records under the shared count prefix.
	var n int
	err = tx.ScanPrefixKeys(ctx, []byte{2, 'h'}, func([]byte) error {
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}
