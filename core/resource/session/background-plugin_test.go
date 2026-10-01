package resource_session_test

import (
	"context"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6/memfs"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/core/session"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
)

// TestSetBackgroundPlugin checks that a Session confirms only a plugin whose
// manifest in the Space declares background, and saves the choice in its
// metadata until withdrawn.
func TestSetBackgroundPlugin(t *testing.T) {
	// Bound the test.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// Create a Session with a Space.
	env := setupTestEnv(ctx, t)
	sessRef, sessIdx := env.createSession(ctx, t)
	acc := env.accessAccount(ctx, t, sessRef)
	env.createSpaceOnAccount(ctx, t, acc, "Background")
	const spaceID = "background-id"
	sessResource := env.buildSessionResource(ctx, t, sessRef)

	// Publish one plugin declaring background and one that does not.
	spaceResource, release, err := sessResource.MountSpace(ctx, spaceID)
	if err != nil {
		t.Fatalf("MountSpace: %v", err)
	}
	tx, err := spaceResource.GetWorldEngine().NewTransaction(ctx, true)
	if err != nil {
		release()
		t.Fatalf("NewTransaction: %v", err)
	}
	for _, plugin := range []struct {
		id         string
		background bool
	}{{"server", true}, {"viewer", false}} {
		meta := bldr_manifest.NewManifestMeta(plugin.id, bldr_manifest.BuildType_DEV, "web/js", 1)
		meta.Background = plugin.background
		if _, _, err := bldr_manifest_world.CommitManifest(
			ctx,
			logrus.NewEntry(logrus.StandardLogger()),
			tx,
			tx.AccessWorldState,
			bldr_manifest.NewManifest(meta, "entrypoint.js"),
			memfs.New(),
			memfs.New(),
			"manifest/"+plugin.id,
			nil,
			env.tb.Volume.GetPeerID(),
			timestamp.Now(),
		); err != nil {
			tx.Discard()
			release()
			t.Fatalf("CommitManifest(%s): %v", plugin.id, err)
		}
	}
	err = tx.Commit(ctx)
	release()
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Set choices through the resource and read them from the metadata.
	set := func(pluginID string, enabled bool) error {
		_, err := sessResource.SetBackgroundPlugin(ctx, &s4wave_session.SetBackgroundPluginRequest{
			SpaceId:  spaceID,
			PluginId: pluginID,
			Enabled:  enabled,
		})
		return err
	}
	backgroundPlugins := func() []*session.BackgroundPlugin {
		meta, err := env.sessCtrl.GetSessionMetadata(ctx, sessIdx)
		if err != nil {
			t.Fatalf("GetSessionMetadata: %v", err)
		}
		return meta.GetBackgroundPlugins()
	}

	// Refuse plugins that do not declare background or are absent.
	for _, pluginID := range []string{"viewer", "absent"} {
		if err := set(pluginID, true); err == nil {
			t.Fatalf("confirmed %s, want error", pluginID)
		}
	}
	if got := backgroundPlugins(); len(got) != 0 {
		t.Fatalf("background plugins after refusals = %v, want none", got)
	}

	// Confirm the background plugin, then withdraw it.
	if err := set("server", true); err != nil {
		t.Fatalf("confirm server: %v", err)
	}
	want := &session.BackgroundPlugin{SpaceId: spaceID, PluginId: "server"}
	if got := backgroundPlugins(); len(got) != 1 || !got[0].EqualVT(want) {
		t.Fatalf("background plugins = %v, want [%v]", got, want)
	}
	if err := set("server", false); err != nil {
		t.Fatalf("withdraw server: %v", err)
	}
	if got := backgroundPlugins(); len(got) != 0 {
		t.Fatalf("background plugins after withdraw = %v, want none", got)
	}
}
