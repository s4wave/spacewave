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
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	ref := createSpacePluginManifest(t, ctx, tb, settingsPluginID, "test/platform", 1)
	key := manifest.NewManifestArtifactKey(ref.GetManifestRef())
	if _, _, err := manifest_world.SetManifest(ctx, tb.WorldState, "", key, ref.GetManifestRef()); err != nil {
		t.Fatal(err)
	}
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
	settings, err = space_world.LookupSpaceSettingsBody(ctx, tb.WorldState)
	if err != nil || !slices.Equal(settings.GetPluginInstallations()[settingsPluginID].GetManifestKeys(), expected) {
		t.Fatalf("installation lost its recovery artifacts: %v", err)
	}
	if _, err := reopenedClient.AddSpacePlugin(ctx, request); err != nil {
		t.Fatal(err)
	}
	settings, err = space_world.LookupSpaceSettingsBody(ctx, tb.WorldState)
	expected = append([]string{key}, expected[:2]...)
	if err != nil || !slices.Equal(settings.GetPluginInstallations()[settingsPluginID].GetManifestKeys(), expected) {
		t.Fatalf("explicit earlier install duplicated or lost an artifact: %v", err)
	}
	request.PluginId = "different-plugin"
	if _, err := client.AddSpacePlugin(ctx, request); err == nil {
		t.Fatal("installed another plugin's artifact")
	}
	request.PluginId, request.ManifestKey = settingsPluginID, "missing-artifact"
	if _, err := client.AddSpacePlugin(ctx, request); err == nil {
		t.Fatal("installed a missing artifact")
	}
	if _, err := reopenedClient.RemoveSpacePlugin(ctx, &s4wave_space.RemoveSpacePluginRequest{PluginId: settingsPluginID}); err != nil {
		t.Fatal(err)
	}
	settings, err = space_world.LookupSpaceSettingsBody(ctx, tb.WorldState)
	if err != nil || len(settings.GetPluginInstallations()) != 0 || len(settings.GetPluginIds()) != 0 {
		t.Fatalf("uninstall retained its selection: %v", err)
	}
}

// TestInstallSpacePluginPerPlatform checks that installing a build replaces only
// its platform's current artifact: the predecessor and the other platforms'
// artifacts stay pinned behind it.
func TestInstallSpacePluginPerPlatform(t *testing.T) {
	// Register one artifact per platform revision in the World.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	builds := []struct {
		platform string
		rev      uint64
	}{{"desktop/linux/amd64", 1}, {"desktop/darwin/arm64", 1}, {"desktop/linux/amd64", 2}}
	var keys []string
	for _, build := range builds {
		ref := createSpacePluginManifest(t, ctx, tb, settingsPluginID, build.platform, build.rev)
		key := manifest.NewManifestArtifactKey(ref.GetManifestRef())
		if _, _, err := manifest_world.SetManifest(ctx, tb.WorldState, "", key, ref.GetManifestRef()); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key)
	}

	// Install them in order as the submitting Session.
	sender := tb.Volume.GetPeerID()
	for _, key := range keys {
		changed, err := space_world_ops.InstallSpacePlugin(ctx, tb.WorldState, sender, settingsPluginID, key, time.Now())
		if err != nil || !changed {
			t.Fatalf("install %s: changed=%v err=%v", key, changed, err)
		}
	}

	// The newest linux artifact leads and keeps its predecessor and the darwin artifact.
	settings, err := space_world.LookupSpaceSettingsBody(ctx, tb.WorldState)
	if err != nil {
		t.Fatal(err)
	}
	expected := []string{keys[2], keys[1], keys[0]}
	if got := settings.GetPluginInstallations()[settingsPluginID].GetManifestKeys(); !slices.Equal(got, expected) {
		t.Fatalf("installed artifacts %v, expected %v", got, expected)
	}

	// Installing the leading artifact again changes nothing.
	changed, err := space_world_ops.InstallSpacePlugin(ctx, tb.WorldState, sender, settingsPluginID, keys[2], time.Now())
	if err != nil || changed {
		t.Fatalf("repeat install: changed=%v err=%v", changed, err)
	}
}
