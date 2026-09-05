//go:build !js

package bldr_manifest_builder

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/bldr/testbed"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/sirupsen/logrus"
)

func TestCommitManifestUsesContentAddressing(t *testing.T) {
	ctx := withManifestCommitTimestamp(
		context.Background(),
		timestamp.New(time.Unix(1700000000, 0)),
	)
	firstWorld, err := testbed.BuildTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(firstWorld.Release)
	secondWorld, err := testbed.BuildTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(secondWorld.Release)

	distPath := filepath.Join(t.TempDir(), "dist")
	assetsPath := filepath.Join(t.TempDir(), "assets")
	writeCommitTestFile(t, distPath, "plugin-HASH.mjs", "export const value = 1;\n")
	writeCommitTestFile(t, assetsPath, "asset.txt", "asset\n")

	firstManifest, firstRef := commitManifestForTest(t, ctx, firstWorld, distPath, assetsPath)
	secondManifest, secondRef := commitManifestForTest(t, ctx, secondWorld, distPath, assetsPath)
	if !secondRef.EqualVT(firstRef) {
		t.Fatalf("identical build ref = %v, want %v", secondRef, firstRef)
	}
	if !secondManifest.GetDistFsRef().EqualVT(firstManifest.GetDistFsRef()) {
		t.Fatal("identical build produced a different dist filesystem ref")
	}
	if !secondManifest.GetAssetsFsRef().EqualVT(firstManifest.GetAssetsFsRef()) {
		t.Fatal("identical build produced a different assets filesystem ref")
	}

	// Dist copying uses this same timestamp policy and preserves content identity.
	var copiedRoot *bucket.ObjectRef
	for _, target := range []*testbed.Testbed{firstWorld, secondWorld} {
		_, copied, err := bldr_manifest_world.DeepCopyManifest(
			ctx, target.GetLogger(), firstWorld.GetWorldState().AccessWorldState,
			firstRef, nil, target.GetWorldState(), target.GetWorldState().AccessWorldState,
			"copied-manifest", nil, target.GetVolume().GetPeerID(), ManifestCommitTimestamp(ctx),
		)
		if err != nil {
			t.Fatal(err)
		}
		if copiedRoot != nil && !copied.GetRootRef().EqualVT(copiedRoot.GetRootRef()) {
			t.Fatal("fixed-time dist copying changed manifest content identity")
		}
		copiedRoot = copied
	}

	writeCommitTestFile(t, distPath, "plugin-HASH.mjs", "export const value = 2;\n")
	_, changedRef := commitManifestForTest(t, ctx, secondWorld, distPath, assetsPath)
	if changedRef.EqualVT(firstRef) {
		t.Fatal("changed content produced the first manifest ref")
	}
}

func commitManifestForTest(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	distPath string,
	assetsPath string,
) (*bldr_manifest.Manifest, *bucket.ObjectRef) {
	t.Helper()

	meta := bldr_manifest.NewManifestMeta(
		"spacewave-js-plugin",
		bldr_manifest.BuildType_DEV,
		"js",
		1,
	)
	builderConfig := &BuilderConfig{
		ManifestMeta:   meta,
		SourcePath:     t.TempDir(),
		DistSourcePath: t.TempDir(),
		WorkingPath:    t.TempDir(),
		EngineId:       tb.GetWorldEngineID(),
		ObjectKey:      bldr_manifest.NewManifestKey(tb.GetPluginHostObjKey(), meta),
		LinkObjectKeys: []string{tb.GetPluginHostObjKey()},
		PeerId:         tb.GetVolume().GetPeerID().String(),
	}
	manifestValue, manifestRef, err := builderConfig.CommitManifestWithPaths(
		ctx,
		tb.GetLogger(),
		tb.GetWorldState(),
		meta,
		"plugin-HASH.mjs",
		distPath,
		assetsPath,
	)
	if err != nil {
		t.Fatal(err)
	}
	return manifestValue, manifestRef
}

func writeCommitTestFile(t *testing.T, rootPath, relPath, contents string) {
	t.Helper()

	filePath := filepath.Join(rootPath, relPath)
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filePath, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
