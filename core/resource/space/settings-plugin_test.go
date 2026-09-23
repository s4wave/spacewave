package resource_space

import (
	"slices"
	"testing"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	space_world "github.com/s4wave/spacewave/core/space/world"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/s4wave/spacewave/testbed"
)

// TestInstallSpacePluginArtifact exercises persistence and identity validation
// through the public RPC, including reopening the Resource and uninstalling.
func TestInstallSpacePluginArtifact(t *testing.T) {
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	ref := createPluginReadinessManifest(t, ctx, tb)
	key := manifest.NewManifestArtifactKey(ref.GetManifestRef())
	if _, _, err := manifest_world.SetManifest(ctx, tb.WorldState, "", key, ref.GetManifestRef()); err != nil {
		t.Fatal(err)
	}
	body := &spaceResourceChatBody{engine: tb.BusEngine, engineID: tb.EngineID, bucketID: tb.EngineBucketID}
	resource := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, tb.Volume.GetPeerID().String())
	client := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, resource.GetMux()))
	request := &s4wave_space.AddSpacePluginRequest{PluginId: pluginReadinessPluginID, ManifestKey: key}
	if _, err := client.AddSpacePlugin(ctx, request); err != nil {
		t.Fatal(err)
	}

	// Reopening and a repeated name-only install preserve the selected artifact.
	reopened := NewSpaceResourceWithSessionPeerID(tb.Logger, tb.Bus, body, tb.Volume.GetPeerID().String())
	reopenedClient := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceResourceClient(t, reopened.GetMux()))
	if _, err := reopenedClient.AddSpacePlugin(ctx, &s4wave_space.AddSpacePluginRequest{PluginId: pluginReadinessPluginID}); err != nil {
		t.Fatal(err)
	}
	settings, err := space_world.LookupSpaceSettingsBody(ctx, tb.WorldState)
	if err != nil || !slices.Equal(settings.GetPluginInstallations()[pluginReadinessPluginID].GetManifestKeys(), []string{key}) {
		t.Fatalf("installed artifact did not survive Resource reopen: %v", err)
	}
	selected, err := space_world.LookupSpacePluginManifest(ctx, tb.WorldState, pluginReadinessPluginID, key)
	if err != nil || !selected.GetManifestRef().GetRootRef().EqualVT(ref.GetManifestRef().GetRootRef()) {
		t.Fatalf("installed artifact changed identity: %v", err)
	}
	// Repeated attempts retain earlier exact artifacts, including across Resource
	// reopen. Explicitly selecting an earlier version moves it to the front once.
	expected := []string{key}
	for _, revision := range []uint64{8, 9} {
		ref := createSpacePluginManifest(t, ctx, tb, pluginReadinessPluginID, "js", revision)
		nextKey := manifest.NewManifestArtifactKey(ref.GetManifestRef())
		if _, _, err := manifest_world.SetManifest(ctx, tb.WorldState, "", nextKey, ref.GetManifestRef()); err != nil {
			t.Fatal(err)
		}
		if _, err := reopenedClient.AddSpacePlugin(ctx, &s4wave_space.AddSpacePluginRequest{PluginId: pluginReadinessPluginID, ManifestKey: nextKey}); err != nil {
			t.Fatal(err)
		}
		expected = append([]string{nextKey}, expected...)
	}
	settings, err = space_world.LookupSpaceSettingsBody(ctx, tb.WorldState)
	if err != nil || !slices.Equal(settings.GetPluginInstallations()[pluginReadinessPluginID].GetManifestKeys(), expected) {
		t.Fatalf("installation lost its recovery artifacts: %v", err)
	}
	if _, err := reopenedClient.AddSpacePlugin(ctx, request); err != nil {
		t.Fatal(err)
	}
	settings, err = space_world.LookupSpaceSettingsBody(ctx, tb.WorldState)
	expected = append([]string{key}, expected[:2]...)
	if err != nil || !slices.Equal(settings.GetPluginInstallations()[pluginReadinessPluginID].GetManifestKeys(), expected) {
		t.Fatalf("explicit earlier install duplicated or lost an artifact: %v", err)
	}
	request.PluginId = "different-plugin"
	if _, err := client.AddSpacePlugin(ctx, request); err == nil {
		t.Fatal("installed another plugin's artifact")
	}
	request.PluginId, request.ManifestKey = pluginReadinessPluginID, "missing-artifact"
	if _, err := client.AddSpacePlugin(ctx, request); err == nil {
		t.Fatal("installed a missing artifact")
	}
	if _, err := reopenedClient.RemoveSpacePlugin(ctx, &s4wave_space.RemoveSpacePluginRequest{PluginId: pluginReadinessPluginID}); err != nil {
		t.Fatal(err)
	}
	settings, err = space_world.LookupSpaceSettingsBody(ctx, tb.WorldState)
	if err != nil || len(settings.GetPluginInstallations()) != 0 || len(settings.GetPluginIds()) != 0 {
		t.Fatalf("uninstall retained its selection: %v", err)
	}
}
