//go:build !js

package devtool

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

func TestCachedManifestFetchControllerResolvesImportedExternalManifest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	repoRoot := t.TempDir()
	d, err := BuildDevtoolBus(ctx, logrus.NewEntry(logrus.New()), repoRoot, filepath.Join(repoRoot, ".bldr"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Release()

	rel, err := d.startCachedManifestFetchController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	meta := bldr_manifest.NewManifestMeta("glados-core", bldr_manifest.BuildType_DEV, "web/js/wasm", 7)
	manifestRoot, _, err := world.AccessWorldObject(
		ctx,
		d.GetWorldState(),
		"test/glados-core-manifest-root",
		true,
		func(bcs *block.Cursor) error {
			bcs.SetBlock(bldr_manifest.NewManifest(meta, "entrypoint"), true)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	manifestRef := bldr_manifest.NewManifestRef(meta, manifestRoot)
	manifestKey := bldr_manifest.NewManifestKey(d.GetPluginHostObjectKey(), meta)
	if err := bldr_manifest_world.ExStoreManifestOp(
		ctx,
		d.GetWorldState(),
		d.GetVolume().GetPeerID(),
		manifestKey,
		[]string{d.GetPluginHostObjectKey()},
		manifestRef,
	); err != nil {
		t.Fatal(err)
	}

	val, _, ref, err := bus.ExecWaitValue[*bldr_manifest.FetchManifestValue](
		ctx,
		d.GetBus(),
		bldr_manifest.NewFetchManifest("glados-core", nil, []string{"web/js/wasm"}, 0),
		bus.ReturnWhenIdle(),
		nil,
		func(val *bldr_manifest.FetchManifestValue) (bool, error) {
			return len(val.GetManifestRefs()) != 0, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()

	refs := val.GetManifestRefs()
	if len(refs) != 1 {
		t.Fatalf("manifest refs = %d, want 1", len(refs))
	}
	got := refs[0].GetMeta()
	if got.GetManifestId() != "glados-core" {
		t.Fatalf("manifest id = %q, want glados-core", got.GetManifestId())
	}
	if got.GetPlatformId() != "web/js/wasm" {
		t.Fatalf("platform id = %q, want web/js/wasm", got.GetPlatformId())
	}
}
