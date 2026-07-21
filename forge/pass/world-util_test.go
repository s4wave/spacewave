package forge_pass

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

func TestLookupPassReleasesObjectState(t *testing.T) {
	ctx := t.Context()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	const key = "forge/pass/lookup-release"
	_, _, err = world.CreateWorldObject(ctx, wtb.WorldState, key, func(bcs *block.Cursor) error {
		bcs.SetBlock(&Pass{PassState: State_PassState_PENDING}, true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	wrapped := &lookupReleaseWorldState{WorldState: wtb.WorldState}
	for range 3 {
		pass, target, err := LookupPass(ctx, wrapped, key)
		if err != nil {
			t.Fatal(err)
		}
		if pass == nil {
			t.Fatal("expected pass")
		}
		if target != nil {
			t.Fatalf("expected no target, got %#v", target)
		}
	}
	if wrapped.releases != 3 {
		t.Fatalf("expected repeated lookups to release once each, got %d", wrapped.releases)
	}

	const badKey = "forge/pass/lookup-release-bad"
	_, _, err = world.CreateWorldObject(ctx, wtb.WorldState, badKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&invalidPassBlock{}, true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	wrapped.releases = 0
	if _, _, err := LookupPass(ctx, wrapped, badKey); err == nil {
		t.Fatal("expected pass decode error")
	}
	if wrapped.releases != 1 {
		t.Fatalf("expected decode error to release once, got %d", wrapped.releases)
	}

	wrapped.releases = 0
	if _, _, err := LookupPass(ctx, wrapped, "forge/pass/lookup-release-missing"); err == nil {
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
	return &lookupReleaseObjectState{
		ObjectState: obj,
		releases:    &ws.releases,
	}, true, nil
}

type lookupReleaseObjectState struct {
	world.ObjectState
	releases *int
}

func (obj *lookupReleaseObjectState) Release() {
	(*obj.releases)++
}

type invalidPassBlock struct{}

func (b *invalidPassBlock) MarshalBlock() ([]byte, error) {
	return []byte{0x80}, nil
}

func (b *invalidPassBlock) UnmarshalBlock([]byte) error {
	return nil
}
