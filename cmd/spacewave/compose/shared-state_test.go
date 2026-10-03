//go:build !js

package spacewave_compose

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	bldr_dist_compiler "github.com/s4wave/spacewave/bldr/dist/compiler"
	dist_entrypoint "github.com/s4wave/spacewave/bldr/dist/entrypoint"
	entrypoint_state "github.com/s4wave/spacewave/bldr/entrypoint/state"
	bldr_project_starlark "github.com/s4wave/spacewave/bldr/project/starlark"
	default_storage "github.com/s4wave/spacewave/bldr/storage/default"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	"github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/sirupsen/logrus"
)

// TestSharedSessionCatalogSurvivesEntrypointChange checks native and distribution
// storage wiring against one real durable Session catalog, in both directions.
func TestSharedSessionCatalogSurvivesEntrypointChange(t *testing.T) {
	// Register a Session through the native CLI's mounted host volume.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	root := t.TempDir()
	cliBus, err := cli_entrypoint.BuildCliBus(ctx, le, "spacewave", root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cliBus.Release)
	cliBus.GetStaticResolver().AddFactory(session_controller.NewFactory(cliBus.GetBus()))

	// Register the CLI Session and release its mounted catalog.
	cliSessions, releaseCLI := openSharedStateSessions(t, ctx, cliBus.GetBus())
	ref := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
		ProviderId: "local", ProviderAccountId: "account", Id: "session",
	}}
	entry, err := cliSessions.RegisterSession(ctx, ref, &session.SessionMetadata{DisplayName: "CLI Session"})
	if err != nil {
		t.Fatal(err)
	}
	releaseCLI()
	cliBus.Release()

	// Mount the distribution's production volume configuration after the CLI exits.
	distBus, factories, err := dist_entrypoint.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = distBus.Close() })
	factories.AddFactory(session_controller.NewFactory(distBus))
	storage := default_storage.NewController(default_storage.StorageID, distBus, root)
	releaseStorage, err := distBus.AddController(ctx, storage, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseStorage)

	// Mount the distribution Session volume on the host storage.
	_, volumeRef, err := storage_volume.ExecVolumeController(ctx, distBus, entrypoint_state.NewVolumeConfig(default_storage.StorageID))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(volumeRef.Release)

	// Read the CLI Session through the distribution catalog.
	distSessions, releaseDist := openSharedStateSessions(t, ctx, distBus)
	entries, err := distSessions.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || !entries[0].EqualVT(entry) {
		t.Fatalf("desktop Sessions = %v, want CLI entry %v", entries, entry)
	}

	// Persist a desktop change, then reopen it through a fresh native CLI bus.
	if err := distSessions.UpdateSessionMetadata(ctx, ref, &session.SessionMetadata{DisplayName: "Desktop Session"}); err != nil {
		t.Fatal(err)
	}
	releaseDist()
	volumeRef.Release()
	releaseStorage()
	if err := distBus.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen the persisted catalog through a fresh CLI bus.
	reopened, err := cli_entrypoint.BuildCliBus(ctx, le, "spacewave", root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Release)
	reopened.GetStaticResolver().AddFactory(session_controller.NewFactory(reopened.GetBus()))
	reopenedSessions, releaseReopened := openSharedStateSessions(t, ctx, reopened.GetBus())
	t.Cleanup(releaseReopened)

	// Verify the CLI observes the desktop metadata change.
	metadata, err := reopenedSessions.GetSessionMetadata(ctx, entry.GetSessionIndex())
	if err != nil {
		t.Fatal(err)
	}
	if metadata.GetDisplayName() != "Desktop Session" {
		t.Fatalf("CLI Session metadata = %v, want desktop change", metadata)
	}

	// Both compositions create exactly one catalog file in this isolated root.
	files, err := filepath.Glob(filepath.Join(root, "*.s4wave"))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(files, []string{filepath.Join(root, entrypoint_state.Filename)}) {
		t.Fatalf("state volumes = %v, want only %s", files, entrypoint_state.Filename)
	}
}

// openSharedStateSessions retains a Session controller using the default host volume.
func openSharedStateSessions(t *testing.T, ctx context.Context, b bus.Bus) (*session_controller.Controller, func()) {
	t.Helper()
	ctrl, _, ref, err := loader.WaitExecControllerRunningTyped[*session_controller.Controller](
		ctx, b, resolver.NewLoadControllerWithConfig(&session_controller.Config{}), nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ctrl, ref.Release
}

// TestCLIReleaseLeavesWebDemandToDesktop verifies renderer-specific startup demand.
func TestCLIReleaseLeavesWebDemandToDesktop(t *testing.T) {
	// Evaluate the same build configuration used by release producers.
	result, err := bldr_project_starlark.Evaluate(filepath.Join("..", "..", "..", "bldr.star"))
	if err != nil {
		t.Fatal(err)
	}

	// CLI daemons expose their Session state without starting the desktop plugin.
	for _, test := range []struct {
		build    string
		manifest string
		web      bool
	}{
		{"release-cli-darwin-arm64", "spacewave-cli", false},
		{"release-desktop-darwin-arm64", "spacewave-dist", true},
	} {
		var conf bldr_dist_compiler.Config
		if err := conf.UnmarshalJSON(result.Config.GetBuild()[test.build].GetManifestOverrides()[test.manifest].GetConfig()); err != nil {
			t.Fatal(err)
		}
		if got := slices.Contains(conf.GetLoadPlugins(), "web"); got != test.web {
			t.Fatalf("%s loads web = %t, want %t", test.build, got, test.web)
		}
	}
}
