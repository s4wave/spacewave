package space_sobject

import (
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/testbed"
)

func TestNewSpaceWorldEngineConfigDisablesChangelog(t *testing.T) {
	// Build a shared object reference pointing at the test space.
	sharedObjectRef := &sobject.SharedObjectRef{
		ProviderResourceRef: &provider.ProviderResourceRef{
			Id:                "test-space",
			ProviderId:        "local",
			ProviderAccountId: "test-account",
		},
	}

	// Build the engine config and assert its engine id and changelog.
	conf := newSpaceWorldEngineConfig(sharedObjectRef, &Config{})
	if conf.GetEngineId() != space.SpaceEngineId(sharedObjectRef) {
		t.Fatalf("unexpected engine id: %q", conf.GetEngineId())
	}
	if !conf.GetInitWorldOp().GetLastChangeDisable() {
		t.Fatal("expected Space world init to disable changelog")
	}
}

func TestMountSpaceBodyProvidesSpaceWorldOps(t *testing.T) {
	// Write test space settings through the world state.
	ctx := t.Context()
	ws := world.NewEngineWorldState(mountTestSpace(t), true)
	settings := &space_world.SpaceSettings{
		IndexPath: "/test",
		PluginIds: []string{
			"spacewave-test",
		},
	}

	// Apply the settings operation.
	if _, _, err := space_world_ops.SetSpaceSettings(ctx, ws, "", "", settings, true, time.Now()); err != nil {
		t.Fatal(err.Error())
	}

	// Look up the settings and assert the index path persisted.
	gotSettings, objectState, err := space_world.LookupSpaceSettings(ctx, ws)
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatal(err.Error())
	}
	if gotSettings.GetIndexPath() != "/test" {
		t.Fatalf("unexpected index path: %q", gotSettings.GetIndexPath())
	}
}

// TestSpaceWorldChangelogFollowsSettings checks that writing a Space's
// settings switches its World changelog, which then records each change.
func TestSpaceWorldChangelogFollowsSettings(t *testing.T) {
	// Mount a new Space and open its World.
	ctx := t.Context()
	ws := world.NewEngineWorldState(mountTestSpace(t), true)

	// setChangelog writes the Space settings with the changelog setting.
	setChangelog := func(enabled bool) {
		settings := &space_world.SpaceSettings{ChangelogEnabled: enabled}
		if _, _, err := space_world_ops.SetSpaceSettings(ctx, ws, "", "", settings, true, time.Now()); err != nil {
			t.Fatal(err.Error())
		}
	}

	// createObject creates an empty object at key.
	createObject := func(key string) {
		obj, err := ws.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// changedKeys returns the keys of the recorded changes, oldest first.
	changedKeys := func() []string {
		// Read the changelog, newest first.
		entries, err := world_block.ReadChangeLogEntries(ctx, ws.AccessWorldState, world_block.ChangeLogReadOptions{})
		if err != nil {
			t.Fatal(err.Error())
		}

		// Collect the changed keys, oldest first.
		var keys []string
		for _, entry := range slices.Backward(entries) {
			for _, change := range entry.Changes {
				keys = append(keys, change.GetKey())
			}
		}
		return keys
	}

	// A new Space keeps no changelog.
	createObject("before")
	if keys := changedKeys(); len(keys) != 0 {
		t.Fatalf("changelog before enabling: %q", keys)
	}

	// Enabling starts history after the settings write.
	setChangelog(true)
	createObject("after")
	if keys := changedKeys(); !slices.Equal(keys, []string{"after"}) {
		t.Fatalf("changelog after enabling: %q", keys)
	}

	// Disabling drops the history.
	setChangelog(false)
	createObject("disabled")
	if keys := changedKeys(); len(keys) != 0 {
		t.Fatalf("changelog after disabling: %q", keys)
	}
}

// mountTestSpace mounts a new Space body on a testbed and returns its World
// engine. The testbed is released when the test ends.
func mountTestSpace(t *testing.T) world.Engine {
	// Start the default testbed.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Register the provider and space controllers. The space controller runs
	// its own engine, so no engine factory is registered.
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	// Load the local provider controller.
	providerID := "local"
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     tb.Volume.GetPeerID().String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(provCtrlRef.Release)

	// Load the space sobject controller.
	_, spaceSobjectCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&Config{}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(spaceSobjectCtrlRef.Release)

	// Access the test provider account.
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tb.Bus, providerID, "test-account", false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(provAccRef.Release)

	// Fetch the shared-object feature from the provider account.
	wsProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the test space shared object.
	spaceMeta, err := space.NewSharedObjectMeta("Test Space")
	if err != nil {
		t.Fatal(err.Error())
	}
	soRef, err := wsProv.CreateSharedObject(ctx, "test-space", spaceMeta, "", "")
	if err != nil {
		t.Fatal(err.Error())
	}

	// Mount the space body from the shared object.
	mounted, mountedRef, err := space.ExMountSpaceSoBody(ctx, tb.Bus, soRef, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(mountedRef.Release)
	return mounted.GetSharedObjectBody().GetWorldEngine()
}
