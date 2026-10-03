package bldr_manifest_pack

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/sirupsen/logrus"
)

// TestCommitDistDirManifestPacksAppBundle checks that a packaged app bundle
// keeps the built manifest identity and its executable permissions.
func TestCommitDistDirManifestPacksAppBundle(t *testing.T) {
	// Build the test World state.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	ws := newTestWorld(t, ctx, le)

	// Write an app bundle with an executable into the dist directory.
	distDir := t.TempDir()
	macosDir := filepath.Join(distDir, "Spacewave.app", "Contents", "MacOS")
	if err := os.MkdirAll(macosDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(macosDir, "Spacewave"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Commit the dist directory as a manifest and check its meta.
	meta := &bldr_manifest.ManifestMeta{
		ManifestId: "spacewave-dist",
		BuildType:  "release",
		PlatformId: "desktop/darwin/arm64",
		Rev:        5,
	}
	conf := &ProducerConfig{WorldState: ws, DistDir: distDir, Entrypoint: "Spacewave.app"}
	ref, err := commitDistDirManifest(ctx, ws, conf, meta)
	if err != nil {
		t.Fatal(err)
	}
	if !ref.GetMeta().EqualVT(meta) {
		t.Fatalf("meta = %v, want %v", ref.GetMeta(), meta)
	}

	// Open the packed manifest and verify entrypoint and permissions.
	err = bldr_manifest_world.AccessManifest(ctx, le, ws.AccessWorldState, ref.GetManifestRef(), func(
		ctx context.Context,
		bls *bucket_lookup.Cursor,
		bcs *block.Cursor,
		manifest *bldr_manifest.Manifest,
		distFS *unixfs.FSHandle,
		assetsFS *unixfs.FSHandle,
	) error {
		// Check the entrypoint and the executable's stored permissions.
		if manifest.GetEntrypoint() != "Spacewave.app" {
			t.Fatalf("entrypoint = %q", manifest.GetEntrypoint())
		}
		exe, _, err := distFS.LookupPath(ctx, "Spacewave.app/Contents/MacOS/Spacewave")
		if err != nil {
			return err
		}
		defer exe.Release()
		perm, err := exe.GetPermissions(ctx)
		if err != nil {
			return err
		}
		if perm.Perm() != 0o755 {
			t.Fatalf("executable permissions = %v, want 0755", perm.Perm())
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
