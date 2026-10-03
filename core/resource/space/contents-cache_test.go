package resource_space

import (
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/world"
)

func TestSpaceContentsResource_GetPluginDescriptionsCache(t *testing.T) {
	// Prepare a description cache test under the test context.
	ctx := t.Context()
	var calls int

	// Count description builds with a deterministic plugin description source.
	r := &SpaceContentsResource{
		buildDescriptions: func(_ context.Context, _ world.WorldState, pluginIDs []string) (map[string]string, error) {
			calls++
			return map[string]string{
				pluginIDs[0]: "desc-" + pluginIDs[0],
			}, nil
		},
	}

	// Build and verify the initial plugin description.
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

	// Verify caller mutation cannot alter the cached plugin descriptions.
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

	// Verify changing the plugin identity rebuilds its descriptions.
	_, err = r.getPluginDescriptions(ctx, nil, []string{"beta"})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected rebuild after plugin set change, got %d builds", calls)
	}

	// Verify changing the plugin set retains its new ordered cache key.
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
