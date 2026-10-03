package volume_world_test

import (
	"context"
	"encoding/binary"
	"testing"

	"golang.org/x/sync/errgroup"

	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	volume_world "github.com/s4wave/spacewave/db/volume/world"
	"github.com/s4wave/spacewave/db/world/testbed"
)

// openDirectVolume bypasses controller deduplication to exercise independent
// adapters to the same authoritative World engine.
func openDirectVolume(tb *testbed.Testbed, key string) (*volume_world.Volume, error) {
	conf := volume_world.NewConfig(tb.Volume.GetID(), tb.EngineBucketID, tb.EngineID, key, nil)
	return volume_world.NewVolume(tb.Context, tb.Logger, tb.Bus, transform_all.BuildFactorySet(), conf)
}

// TestIndependentVolumes preserves accepted disjoint writes and serializes
// read-modify-write operations across adapters, including a fresh reopen.
func TestIndependentVolumes(t *testing.T) {
	// Start a shared World testbed for independent volume adapters.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Open independent adapters to the same World volume concurrently.
	volumes := make([]*volume_world.Volume, 2)
	var group errgroup.Group
	for i := range volumes {
		group.Go(func() error {
			vol, err := openDirectVolume(tb, "concurrent-volume")
			volumes[i] = vol
			return err
		})
	}
	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}

	// Check that concurrent adapters use the same persisted volume identity.
	for _, vol := range volumes {
		defer vol.Close()
		if vol.GetPeerID() != volumes[0].GetPeerID() {
			t.Fatal("concurrent initialization created different identities")
		}
	}

	// Apply concurrent counter updates and disjoint writes through both adapters.
	const updates = 16
	for i, vol := range volumes {
		group.Go(func() error {
			for range updates {
				if err := incrementVolumeCounter(ctx, vol, byte(i)); err != nil {
					return err
				}
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}

	// Sync the shared volume and reopen it through a fresh adapter.
	if _, err := volumes[0].Sync(ctx); err != nil {
		t.Fatal(err)
	}
	reopened, err := openDirectVolume(tb, "concurrent-volume")
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()

	// Check that reopening preserves the volume identity.
	if reopened.GetPeerID() != volumes[0].GetPeerID() {
		t.Fatal("reopen changed the persisted identity")
	}

	// Read the reopened volume through a retained KV transaction.
	tx, err := reopened.GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Check that every counter update survived concurrent writes and reopening.
	data, found, err := tx.Get(ctx, []byte("counter"))
	if err != nil || !found || len(data) != 8 {
		t.Fatalf("counter read: data=%x found=%v err=%v", data, found, err)
	}
	if got := binary.LittleEndian.Uint64(data); got != updates*uint64(len(volumes)) {
		t.Fatalf("counter = %d, want %d", got, updates*len(volumes))
	}

	// Check that each adapter retained its accepted disjoint write.
	for i := range volumes {
		if _, found, err := tx.Get(ctx, []byte{byte(i)}); err != nil || !found {
			t.Fatalf("writer %d lost its accepted write: found=%v err=%v", i, found, err)
		}
	}
}

// incrementVolumeCounter reads and updates a shared value in one KV transaction.
func incrementVolumeCounter(ctx context.Context, vol *volume_world.Volume, writer byte) error {
	// Acquire a writable volume transaction for one counter update.
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Read the shared counter within the retained write transaction.
	data, found, err := tx.Get(ctx, []byte("counter"))
	if err != nil {
		return err
	}
	var count uint64
	if found {
		count = binary.LittleEndian.Uint64(data)
	}

	// Write the incremented counter and record the accepted writer.
	if err := tx.Set(ctx, []byte("counter"), binary.LittleEndian.AppendUint64(nil, count+1)); err != nil {
		return err
	}
	if err := tx.Set(ctx, []byte{writer}, []byte("accepted")); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
