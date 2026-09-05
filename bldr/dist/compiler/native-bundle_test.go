//go:build !js

package bldr_dist_compiler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/aperturerobotics/util/enabled"
	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestNativeBundleResources compiles both native layouts in the same scratch
// directory and verifies that switching layouts removes stale volume files.
func TestNativeBundleResources(t *testing.T) {
	// Keep the expensive compiler contract out of short test runs.
	if testing.Short() {
		t.Skip("builds a complete native distribution")
	}

	// Build beneath the module root so generated entrypoints resolve dependencies.
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(root, ".tmp")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	work, err := os.MkdirTemp(scratch, "native-bundle-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(work) })
	out := filepath.Join(work, "dist")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}

	// Use a real manifest World with the current machine's native compiler.
	platformID := "desktop/" + runtime.GOOS + "/" + runtime.GOARCH
	platform, err := bldr_platform.ParsePlatform(platformID)
	if err != nil {
		t.Fatal(err)
	}
	initWorld := func(ctx context.Context, engine world.Engine, _ peer.ID) error {
		_, err := bldr_manifest_world.CreateManifestStoreInEngine(ctx, engine, "manifests")
		return err
	}

	// Default, embedded, then external again exercise both layout transitions.
	var previous []byte
	for _, option := range []enabled.Enabled{enabled.Enabled_DEFAULT, enabled.Enabled_ENABLE, enabled.Enabled_DISABLE} {
		meta := bldr_dist.NewDistMeta("resource-fixture", platformID, nil, nil, "dist")
		err := BuildDistBundle(t.Context(), logrus.NewEntry(logrus.New()), root, root, "", work, out, "app", meta, bldr_manifest.BuildType_DEV, nil, platform, nil, initWorld, nil, 0, 0, 0, option, nil, "")
		if err != nil {
			t.Fatal(err)
		}

		// Only the selected layout may remain in the build output.
		volumePath := filepath.Join(out, "assets.kvfile")
		absentPath := filepath.Join(work, "entrypoint", "assets.kvfile")
		if option.IsEnabled(false) {
			volumePath, absentPath = absentPath, volumePath
		}
		if _, err := os.Stat(absentPath); !os.IsNotExist(err) {
			t.Fatalf("stale volume at %s: %v", absentPath, err)
		}
		volume, err := os.ReadFile(volumePath)
		if err != nil {
			t.Fatal(err)
		}
		if len(volume) == 0 {
			t.Fatal("empty packaged volume")
		}
		if previous != nil && !bytes.Equal(previous, volume) {
			t.Fatalf("repeated bundle changed: %d -> %d bytes", len(previous), len(volume))
		}
		previous = volume

		// The self-contained executable carries the actual packed World bytes.
		executable, err := os.ReadFile(filepath.Join(out, "app"))
		if err != nil {
			t.Fatal(err)
		}
		if option.IsEnabled(false) && !bytes.Contains(executable, volume) {
			t.Fatal("embedded executable does not contain its volume")
		}
	}
}
