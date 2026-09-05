//go:build !js

package bldr_dist_compiler

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	bldr_dist "github.com/s4wave/spacewave/bldr/dist"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestNativeBundleResources(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a complete native distribution")
	}
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
	platformID := "desktop/" + runtime.GOOS + "/" + runtime.GOARCH
	platform, err := bldr_platform.ParsePlatform(platformID)
	if err != nil {
		t.Fatal(err)
	}
	initWorld := func(ctx context.Context, engine world.Engine, _ peer.ID) error {
		_, err := bldr_manifest_world.CreateManifestStoreInEngine(ctx, engine, "manifests")
		return err
	}
	var previous []byte
	for range 2 {
		meta := bldr_dist.NewDistMeta("resource-fixture", platformID, nil, nil, "dist")
		err := BuildDistBundle(t.Context(), logrus.NewEntry(logrus.New()), root, root, "", work, out, "app", meta, bldr_manifest.BuildType_DEV, nil, platform, nil, initWorld, nil, 0, 0, 0, nil, "")
		if err != nil {
			t.Fatal(err)
		}
		volume, err := os.ReadFile(filepath.Join(out, "assets.kvfile"))
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
		if _, err := os.Stat(filepath.Join(out, "app")); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(work, "entrypoint", "assets.kvfile")); !os.IsNotExist(err) {
			t.Fatalf("volume entered Go compilation directory: %v", err)
		}
	}
	t.Logf("complete native bundle repeats with %d volume bytes", len(previous))
}
