package plugin_space

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
)

// valueRecorder is a resolver handler that records the values it holds.
type valueRecorder struct {
	directive.ResolverHandler
	// values is the values added since the last clear.
	values []directive.Value
}

// AddValue records the value.
func (r *valueRecorder) AddValue(val directive.Value) (uint32, bool) {
	r.values = append(r.values, val)
	return uint32(len(r.values)), true
}

// ClearValues drops the recorded values.
func (r *valueRecorder) ClearValues() []uint32 {
	r.values = nil
	return nil
}

// MarkIdle ignores the idle state.
func (r *valueRecorder) MarkIdle(bool) {}

// TestProcessResolversOnlyOnChange checks that a World change outside a
// resolver's inputs does not search for manifests again, while a new resolver,
// a stored manifest, and a change to the listed plugins do.
func TestProcessResolversOnlyOnChange(t *testing.T) {
	// Bound the test and build the testbed.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Store one manifest in a manifest store.
	const storeKey = "test/plugin-manifests"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, tb.WorldState, storeKey); err != nil {
		t.Fatal(err)
	}
	manifestRef := createReadinessManifest(t, ctx, tb)
	storeManifest := func(objKey string) {
		t.Helper()
		if err := bldr_manifest_world.ExStoreManifestOp(
			ctx, tb.WorldState, tb.Volume.GetPeerID(), objKey, []string{storeKey}, manifestRef,
		); err != nil {
			t.Fatal(err)
		}
	}
	storeManifest("test/manifests/first")

	// Construct the Space controller without its watch loop, so the test calls
	// processResolvers once per World change, and count its searches.
	logger, hook := logtest.NewNullLogger()
	logger.SetLevel(logrus.DebugLevel)
	searches := func() int {
		var n int
		for _, entry := range hook.AllEntries() {
			if entry.Message == "searching for manifests" {
				n++
			}
		}
		return n
	}
	ctrl, err := NewFactory(tb.Bus).Construct(ctx, &Config{
		SpaceId:  "space-test",
		EngineId: tb.EngineID,
	}, controller.ConstructOpts{Logger: logrus.NewEntry(logger)})
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Close()
	space := ctrl.(*Controller)
	setSourcePluginIDs(space, []string{pluginReadinessPluginID})
	addResolver := func() *valueRecorder {
		recorder := &valueRecorder{}
		entry := &resolverEntry{
			ctx:     ctx,
			dir:     bldr_manifest.NewFetchManifest(pluginReadinessPluginID, nil, []string{pluginReadinessPlatformID}, 0),
			handler: recorder,
		}
		space.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			space.resolvers[entry] = struct{}{}
		})
		return recorder
	}

	// The first pass resolves the manifest.
	first := addResolver()
	space.processResolvers(ctx, tb.WorldState)
	if got := searches(); got != 1 {
		t.Fatalf("searches after the first pass = %d, want 1", got)
	}
	if len(first.values) != 1 {
		t.Fatalf("values after the first pass = %d, want 1", len(first.values))
	}

	// A World change outside the manifests does not search again.
	unrelated, err := tb.WorldState.CreateObject(ctx, "test/unrelated", nil)
	if err != nil {
		t.Fatal(err)
	}
	world.ReleaseObjectState(unrelated)
	space.processResolvers(ctx, tb.WorldState)
	if got := searches(); got != 1 {
		t.Fatalf("searches after an unrelated change = %d, want 1", got)
	}

	// A new resolver searches once, and the first still does not.
	second := addResolver()
	space.processResolvers(ctx, tb.WorldState)
	if got := searches(); got != 2 {
		t.Fatalf("searches after a new resolver = %d, want 2", got)
	}
	if len(second.values) != 1 {
		t.Fatalf("values of the new resolver = %d, want 1", len(second.values))
	}

	// A stored manifest searches for both resolvers.
	storeManifest("test/manifests/second")
	space.processResolvers(ctx, tb.WorldState)
	if got := searches(); got != 4 {
		t.Fatalf("searches after storing a manifest = %d, want 4", got)
	}

	// Unlisting the plugin clears both resolvers without searching.
	setSourcePluginIDs(space, nil)
	space.processResolvers(ctx, tb.WorldState)
	if got := searches(); got != 4 {
		t.Fatalf("searches after unlisting the plugin = %d, want 4", got)
	}
	if len(first.values) != 0 || len(second.values) != 0 {
		t.Fatalf("values after unlisting the plugin = %d and %d, want none", len(first.values), len(second.values))
	}

	// Listing the plugin again searches for both resolvers.
	setSourcePluginIDs(space, []string{pluginReadinessPluginID})
	space.processResolvers(ctx, tb.WorldState)
	if got := searches(); got != 6 {
		t.Fatalf("searches after listing the plugin = %d, want 6", got)
	}
}
