package resource_space

import (
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/world"
)

func TestSpaceContentsResource_GetPluginDescriptionsCache(t *testing.T) {
	ctx := t.Context()
	var calls int

	r := &SpaceContentsResource{
		buildDescriptions: func(_ context.Context, _ world.WorldState, pluginIDs []string) (map[string]string, error) {
			calls++
			return map[string]string{
				pluginIDs[0]: "desc-" + pluginIDs[0],
			}, nil
		},
	}

	descriptions, err := r.getPluginDescriptions(ctx, nil, []string{"alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 build, got %d", calls)
	}
	if descriptions["alpha"] != "desc-alpha" {
		t.Fatalf("unexpected description: %#v", descriptions)
	}

	descriptions["alpha"] = "mutated"
	cachedDescriptions, err := r.getPluginDescriptions(ctx, nil, []string{"alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected cache hit, got %d builds", calls)
	}
	if cachedDescriptions["alpha"] != "desc-alpha" {
		t.Fatalf("cache alias leaked mutation: %#v", cachedDescriptions)
	}

	_, err = r.getPluginDescriptions(ctx, nil, []string{"beta"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected rebuild after plugin set change, got %d builds", calls)
	}

	reorderedDescriptions, err := r.getPluginDescriptions(ctx, nil, []string{"beta", "alpha"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("expected rebuild for changed plugin set, got %d builds", calls)
	}
	if !slices.Equal(r.descriptionPluginIDs, []string{"beta", "alpha"}) {
		t.Fatalf("unexpected cached plugin ids: %v", r.descriptionPluginIDs)
	}
	if reorderedDescriptions["beta"] != "desc-beta" {
		t.Fatalf("unexpected rebuilt descriptions: %#v", reorderedDescriptions)
	}
}
