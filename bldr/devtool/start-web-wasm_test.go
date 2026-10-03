//go:build !js

package devtool

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

func TestCachedManifestFetchControllerResolvesImportedExternalManifest(t *testing.T) {
	// Bound the cached manifest request lifetime.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build the Devtool bus in an isolated repository.
	repoRoot := t.TempDir()
	d, err := BuildDevtoolBus(ctx, logrus.NewEntry(logrus.New()), repoRoot, filepath.Join(repoRoot, ".bldr"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Release()

	// Start the cached manifest resolver on the Devtool bus.
	rel, err := d.startCachedManifestFetchController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	// Create the imported manifest root in the World.
	meta := bldr_manifest.NewManifestMeta("gizmo-core", bldr_manifest.BuildType_DEV, "web/js/wasm", 7)
	manifestRoot, _, err := world.AccessWorldObject(
		ctx,
		d.GetWorldState(),
		"test/gizmo-core-manifest-root",
		true,
		func(bcs *block.Cursor) error {
			bcs.SetBlock(bldr_manifest.NewManifest(meta, "entrypoint"), true)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Store the imported manifest under the plugin host.
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

	// Wait for the cached resolver to return a manifest reference.
	val, _, ref, err := bus.ExecWaitValue[*bldr_manifest.FetchManifestValue](
		ctx,
		d.GetBus(),
		bldr_manifest.NewFetchManifest("gizmo-core", nil, []string{"web/js/wasm"}, 0),
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

	// Verify the imported manifest identity and browser platform.
	refs := val.GetManifestRefs()
	if len(refs) != 1 {
		t.Fatalf("manifest refs = %d, want 1", len(refs))
	}
	got := refs[0].GetMeta()
	if got.GetManifestId() != "gizmo-core" {
		t.Fatalf("manifest id = %q, want gizmo-core", got.GetManifestId())
	}
	if got.GetPlatformId() != "web/js/wasm" {
		t.Fatalf("platform id = %q, want web/js/wasm", got.GetPlatformId())
	}
}

func TestCachedManifestFetchControllerDoesNotEmitEmptyManifestValue(t *testing.T) {
	// Bound the missing manifest request lifetime.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build the Devtool bus in an isolated repository.
	repoRoot := t.TempDir()
	d, err := BuildDevtoolBus(ctx, logrus.NewEntry(logrus.New()), repoRoot, filepath.Join(repoRoot, ".bldr"), false)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Release()

	// Start the cached manifest resolver on the Devtool bus.
	rel, err := d.startCachedManifestFetchController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()

	// Subscribe to cached values for an absent manifest.
	valueCh := make(chan *bldr_manifest.FetchManifestValue, 1)
	handler := directive.NewTypedCallbackHandler(
		func(v directive.TypedAttachedValue[*bldr_manifest.FetchManifestValue]) {
			select {
			case valueCh <- v.GetValue():
			default:
			}
		},
		nil,
		nil,
		nil,
	)
	_, ref, err := d.GetBus().AddDirective(
		bldr_manifest.NewFetchManifest("missing-core", nil, []string{"web/js/wasm"}, 0),
		handler,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()

	// Verify the cache miss emits no manifest value.
	select {
	case val := <-valueCh:
		t.Fatalf("unexpected cache-miss FetchManifest value with %d refs", len(val.GetManifestRefs()))
	case <-time.After(250 * time.Millisecond):
	}
}
