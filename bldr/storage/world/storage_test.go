package storage_world

import (
	"testing"

	"golang.org/x/sync/errgroup"

	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	volume_world "github.com/s4wave/spacewave/db/volume/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/db/world/testbed"
)

// TestNamedVolumes covers stable names, prefix isolation, identity, and deletion
// through the real World and Volume implementations.
func TestNamedVolumes(t *testing.T) {
	// Start a World testbed and two storages with distinct prefixes.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	first, err := NewStorage(ctx, tb.Bus, tb.EngineID, "apps/first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewStorage(ctx, tb.Bus, tb.EngineID, "apps/second")
	if err != nil {
		t.Fatal(err)
	}

	// Open four volumes concurrently over the binding matrix.
	bindings := []struct {
		storage *Storage
		name    string
	}{
		{first, "account/a"},
		{first, "account/b"},
		{second, "account/a"},
		{first, "account/a"},
	}
	volumes := make([]*volume_world.Volume, len(bindings))
	var group errgroup.Group
	for i, binding := range bindings {
		group.Go(func() error {
			conf, err := binding.storage.BuildVolumeConfig(binding.name, nil)
			if err != nil {
				return err
			}
			volumes[i], err = volume_world.NewVolume(ctx, tb.Logger, tb.Bus, transform_all.BuildFactorySet(), conf.(*volume_world.Config))
			return err
		})
	}
	if err := group.Wait(); err != nil {
		t.Fatal(err)
	}

	// Close every volume when the test finishes.
	for _, vol := range volumes {
		defer vol.Close()
	}

	// Assert equal names in one prefix reopen one identity and others differ.
	if volumes[0].GetPeerID() != volumes[3].GetPeerID() {
		t.Fatal("equal names in the same prefix did not reopen one identity")
	}
	if volumes[0].GetPeerID() == volumes[1].GetPeerID() || volumes[0].GetPeerID() == volumes[2].GetPeerID() {
		t.Fatal("different names or prefixes share an identity")
	}

	// Reject an empty volume name.
	if _, err := first.VolumeKey(""); err == nil {
		t.Fatal("empty name was accepted")
	}

	// Load the backing of the shared volume.
	key, _ := first.VolumeKey("account/a")
	backing, err := volume_world.LoadBacking(ctx, tb.WorldState, key)
	if err != nil {
		t.Fatal(err)
	}

	// A second object retaining this backing prevents its deletion.
	owner, err := tb.WorldState.CreateObject(ctx, "retained-reference", nil)
	if err != nil {
		t.Fatal(err)
	}
	world.ReleaseObjectState(owner)
	if err := tb.WorldState.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("retained-reference", "retains", backing.KvObjectKey, "")); err != nil {
		t.Fatal(err)
	}

	// Close the volumes holding the shared backing and delete the volume.
	for _, i := range []int{0, 3} {
		if err := volumes[i].Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := first.DeleteVolume("account/a"); err != nil {
		t.Fatal(err)
	}

	// Assert the descriptor is gone while the retained backing survives.
	if found, err := tb.WorldState.HasObject(ctx, key); err != nil || found {
		t.Fatalf("deleted descriptor exists=%v err=%v", found, err)
	}
	if found, err := tb.WorldState.HasObject(ctx, backing.KvObjectKey); err != nil || !found {
		t.Fatalf("shared backing exists=%v err=%v", found, err)
	}

	// Unshared owned backing is removed with its descriptor.
	otherKey, _ := first.VolumeKey("account/b")
	if err := volumes[1].Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.DeleteVolume("account/b"); err != nil {
		t.Fatal(err)
	}
	if found, err := tb.WorldState.HasObject(ctx, otherKey+"/kv"); err != nil || found {
		t.Fatalf("unshared backing exists=%v err=%v", found, err)
	}

	// Assert the deletion left the other prefix's volume untouched.
	secondKey, _ := second.VolumeKey("account/a")
	if _, err := volume_world.LoadBacking(ctx, tb.WorldState, secondKey); err != nil {
		t.Fatalf("deletion affected another prefix: %v", err)
	}
}
