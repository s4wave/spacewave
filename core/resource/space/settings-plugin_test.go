package resource_space

import (
	"slices"
	"testing"
	"time"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/s4wave/spacewave/testbed"
)

// settingsPluginID is the plugin the settings tests install.
const settingsPluginID = "test-settings-plugin"

// TestInstallSpacePluginArtifact exercises persistence and identity validation
// through the public RPC, including reopening the Resource and uninstalling.
func TestInstallSpacePluginArtifact(t *testing.T) {
	// Start the World and register an immutable plugin artifact.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ref := createSpacePluginManifest(t, ctx, tb, settingsPluginID, "test/platform", 1)
	key := manifest.NewManifestArtifactKey(ref.GetManifestRef())
	if _, _, err := manifest_world.SetManifest(ctx, tb.WorldState, "", key, ref.GetManifestRef()); err != nil {
		t.Fatal(err)
	}

	// Install the artifact through the mounted Space Resource.
	body := &spaceResourceChatBody{engine: tb.BusEngine, engineID: tb.EngineID, bucketID: tb.EngineBucketID}
	resource := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, tb.Volume.GetPeerID().String())
	client := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, resource.GetMux()))
	request := &s4wave_space.AddSpacePluginRequest{PluginId: settingsPluginID, ManifestKey: key}
	if _, err := client.AddSpacePlugin(ctx, request); err != nil {
		t.Fatal(err)
	}

	// Reopening and a repeated name-only install preserve the selected artifact.
	reopened := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, tb.Volume.GetPeerID().String())
	reopenedClient := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, reopened.GetMux()))
	if _, err := reopenedClient.AddSpacePlugin(ctx, &s4wave_space.AddSpacePluginRequest{PluginId: settingsPluginID}); err != nil {
		t.Fatal(err)
	}
	settings, err := space_world.LookupSpaceSettingsBody(ctx, tb.WorldState)
	if err != nil || !slices.Equal(settings.GetPluginInstallations()[settingsPluginID].GetManifestKeys(), []string{key}) {
		t.Fatalf("installed artifact did not survive Resource reopen: %v", err)
	}
	selected, err := space_world.LookupSpacePluginManifest(ctx, tb.WorldState, settingsPluginID, key)
	if err != nil || !selected.GetManifestRef().GetRootRef().EqualVT(ref.GetManifestRef().GetRootRef()) {
		t.Fatalf("installed artifact changed identity: %v", err)
	}

	// Repeated attempts retain earlier exact artifacts, including across Resource
	// reopen. Explicitly selecting an earlier version moves it to the front once.
	expected := []string{key}
	for _, revision := range []uint64{8, 9} {
		// Register and install the next immutable JavaScript artifact.
		ref := createSpacePluginManifest(t, ctx, tb, settingsPluginID, "js", revision)
		nextKey := manifest.NewManifestArtifactKey(ref.GetManifestRef())
		if _, _, err := manifest_world.SetManifest(ctx, tb.WorldState, "", nextKey, ref.GetManifestRef()); err != nil {
			t.Fatal(err)
		}
		if _, err := reopenedClient.AddSpacePlugin(ctx, &s4wave_space.AddSpacePluginRequest{PluginId: settingsPluginID, ManifestKey: nextKey}); err != nil {
			t.Fatal(err)
		}
		expected = append([]string{nextKey}, expected...)
	}

	// Verify that the JavaScript predecessor and foreign artifact remain pinned.
	settings, err = space_world.LookupSpaceSettingsBody(ctx, tb.WorldState)
	if err != nil || !slices.Equal(settings.GetPluginInstallations()[settingsPluginID].GetManifestKeys(), expected) {
		t.Fatalf("installation lost its recovery artifacts: %v", err)
	}

	// Move the earlier foreign artifact to the front without duplicating it.
	if _, err := reopenedClient.AddSpacePlugin(ctx, request); err != nil {
		t.Fatal(err)
	}
	settings, err = space_world.LookupSpaceSettingsBody(ctx, tb.WorldState)
	expected = append([]string{key}, expected[:2]...)
	if err != nil || !slices.Equal(settings.GetPluginInstallations()[settingsPluginID].GetManifestKeys(), expected) {
		t.Fatalf("explicit earlier install duplicated or lost an artifact: %v", err)
	}

	// Reject an immutable artifact belonging to another plugin.
	request.PluginId = "different-plugin"
	if _, err := client.AddSpacePlugin(ctx, request); err == nil {
		t.Fatal("installed another plugin's artifact")
	}

	// Reject an artifact that the World does not contain.
	request.PluginId, request.ManifestKey = settingsPluginID, "missing-artifact"
	if _, err := client.AddSpacePlugin(ctx, request); err == nil {
		t.Fatal("installed a missing artifact")
	}

	// Remove the plugin and its installation references through the RPC.
	if _, err := reopenedClient.RemoveSpacePlugin(ctx, &s4wave_space.RemoveSpacePluginRequest{PluginId: settingsPluginID}); err != nil {
		t.Fatal(err)
	}
	settings, err = space_world.LookupSpaceSettingsBody(ctx, tb.WorldState)
	if err != nil || len(settings.GetPluginInstallations()) != 0 || len(settings.GetPluginIds()) != 0 {
		t.Fatalf("uninstall retained its selection: %v", err)
	}
}

// TestInstallSpacePluginPerPlatform checks that three Linux builds retain two
// Linux pins while preserving Darwin pins, rollback, and immutable manifests.
func TestInstallSpacePluginPerPlatform(t *testing.T) {
	// Register one artifact per platform revision in the World.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	builds := []struct {
		platform string
		rev      uint64
	}{
		{"desktop/linux/amd64", 1},
		{"desktop/darwin/arm64", 1},
		{"desktop/darwin/arm64", 2},
		{"desktop/linux/amd64", 2},
		{"desktop/linux/amd64", 3},
	}
	var keys []string
	for _, build := range builds {
		// Store each platform revision under its immutable artifact key.
		ref := createSpacePluginManifest(t, ctx, tb, settingsPluginID, build.platform, build.rev)
		key := manifest.NewManifestArtifactKey(ref.GetManifestRef())
		if _, _, err := manifest_world.SetManifest(ctx, tb.WorldState, "", key, ref.GetManifestRef()); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key)
	}

	// Commit each build's installation in the submitting Session's transaction.
	sender := tb.Volume.GetPeerID()
	for _, key := range keys {
		// Stage the installation references in the World transaction.
		tx, err := tb.Engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tx.Discard)
		changed, err := space_world_ops.InstallSpacePlugin(ctx, tx, sender, settingsPluginID, key, time.Now())
		if err != nil || !changed {
			t.Fatalf("install %s: changed=%v err=%v", key, changed, err)
		}

		// Publish the new installation and its retention changes together.
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		tx.Discard()
	}

	// Drop the oldest Linux pin and preserve both Darwin pins in their order.
	settings, err := space_world.LookupSpaceSettingsBody(ctx, tb.WorldState)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{keys[4], keys[3], keys[2], keys[1]}
	if got := settings.GetPluginInstallations()[settingsPluginID].GetManifestKeys(); !slices.Equal(got, expected) {
		t.Fatalf("installed artifacts %v, expected %v", got, expected)
	}
	t.Logf("after three Linux builds: expected=%v observed=%v removed=%s", expected, settings.GetPluginInstallations()[settingsPluginID].GetManifestKeys(), keys[0])

	// Retention removes installation pins without deleting immutable artifacts.
	for _, key := range keys {
		// Resolve even the artifact whose installation pin was removed.
		if _, err := space_world.LookupSpacePluginManifest(ctx, tb.WorldState, settingsPluginID, key); err != nil {
			t.Fatalf("retention removed immutable artifact %s: %v", key, err)
		}
	}

	// Installing the leading artifact again changes nothing.
	changed, err := space_world_ops.InstallSpacePlugin(ctx, tb.WorldState, sender, settingsPluginID, keys[4], time.Now())
	if err != nil || changed {
		t.Fatalf("repeat install: changed=%v err=%v", changed, err)
	}

	// Reinstall the rollback artifact without duplicating either platform's pins.
	changed, err = space_world_ops.InstallSpacePlugin(ctx, tb.WorldState, sender, settingsPluginID, keys[3], time.Now())
	if err != nil || !changed {
		t.Fatalf("rollback install: changed=%v err=%v", changed, err)
	}
	settings, err = space_world.LookupSpaceSettingsBody(ctx, tb.WorldState)
	if err != nil {
		t.Fatal(err)
	}
	expected = []string{keys[3], keys[4], keys[2], keys[1]}
	if got := settings.GetPluginInstallations()[settingsPluginID].GetManifestKeys(); !slices.Equal(got, expected) {
		t.Fatalf("rollback artifacts %v, expected %v", got, expected)
	}
}
