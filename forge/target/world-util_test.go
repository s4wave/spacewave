package forge_target

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// TestLookupTargetReleasesObjectState proves LookupTarget releases its hidden
// object-state handle exactly once per call on both the success and decode-error
// paths, and releases nothing when the object is absent. The count is measured
// on a real testbed WorldState whose GetObject result is wrapped, so the wrapper
// forwards each release to the real owner rather than standing in for it.
func TestLookupTargetReleasesObjectState(t *testing.T) {
	ctx := t.Context()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	const key = "forge/target/lookup-release"
	created, _, err := CreateTarget(ctx, wtb.WorldState, key, &Target{})
	world.ReleaseObjectState(created)
	if err != nil {
		t.Fatal(err)
	}

	wrapped := &lookupReleaseWorldState{WorldState: wtb.WorldState}
	for range 3 {
		tgt, err := LookupTarget(ctx, wrapped, key)
		if err != nil {
			t.Fatal(err)
		}
		if tgt == nil {
			t.Fatal("expected target")
		}
	}
	if wrapped.releases != 3 {
		t.Fatalf("expected repeated lookups to release once each, got %d", wrapped.releases)
	}

	const badKey = "forge/target/lookup-release-bad"
	badObj, _, err := world.CreateWorldObject(ctx, wtb.WorldState, badKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&invalidTargetBlock{}, true)
		return nil
	})
	world.ReleaseObjectState(badObj)
	if err != nil {
		t.Fatal(err)
	}
	wrapped.releases = 0
	if _, err := LookupTarget(ctx, wrapped, badKey); err == nil {
		t.Fatal("expected target decode error")
	}
	if wrapped.releases != 1 {
		t.Fatalf("expected decode error to release once, got %d", wrapped.releases)
	}

	wrapped.releases = 0
	if _, err := LookupTarget(ctx, wrapped, "forge/target/lookup-release-missing"); err == nil {
		t.Fatal("expected not-found error")
	}
	if wrapped.releases != 0 {
		t.Fatalf("expected not-found lookup to release no state, got %d", wrapped.releases)
	}
}

type lookupReleaseWorldState struct {
	world.WorldState
	releases int
}

func (ws *lookupReleaseWorldState) GetObject(
	ctx context.Context,
	key string,
) (world.ObjectState, bool, error) {
	obj, found, err := ws.WorldState.GetObject(ctx, key)
	if err != nil || !found {
		return obj, found, err
	}
	return &lookupReleaseObjectState{ObjectState: obj, releases: &ws.releases}, true, nil
}

type lookupReleaseObjectState struct {
	world.ObjectState
	releases *int
}

// Release counts the release and forwards it to the real object-state owner so
// the count reflects the genuine release path rather than a fake counter.
func (obj *lookupReleaseObjectState) Release() {
	(*obj.releases)++
	world.ReleaseObjectState(obj.ObjectState)
}

type invalidTargetBlock struct{}

func (b *invalidTargetBlock) MarshalBlock() ([]byte, error) {
	return []byte{0x80}, nil
}

func (b *invalidTargetBlock) UnmarshalBlock([]byte) error {
	return nil
}
