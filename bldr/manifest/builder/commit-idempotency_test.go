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

func TestCommitManifestReusesExistingRevisionForIdenticalOutput(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.BuildTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	fixedTimestamp := timestamp.New(time.Unix(1700000000, 0))
	ctx = withManifestCommitTimestamp(ctx, fixedTimestamp)

	distPath := filepath.Join(t.TempDir(), "dist")
	assetsPath := filepath.Join(t.TempDir(), "assets")
	writeCommitTestFile(t, distPath, "plugin-HASH.mjs", "export const value = 1;\n")
	writeCommitTestFile(t, assetsPath, "asset.txt", "asset\n")

	deps := []*InputManifest_ManifestDep{{
		ManifestId: "spacewave-web",
		ManifestRef: &bucket.ObjectRef{
			BucketId: "provider-bucket",
		},
	}}
	firstManifest, firstRef := commitManifestForTest(t, ctx, tb, 1, distPath, assetsPath)
	firstResult := NewBuilderResult(firstManifest, firstRef, NewInputManifest(nil, nil))
	firstResult.GetInputManifest().ManifestDeps = deps

	identicalCtx := WithManifestCommitIdentity(ctx, firstResult, deps)
	secondManifest, secondRef := commitManifestForTest(t, identicalCtx, tb, 2, distPath, assetsPath)
	if secondManifest.GetMeta().GetRev() != 1 {
		t.Fatalf("identical build rev = %d, want 1", secondManifest.GetMeta().GetRev())
	}
	if !secondRef.EqualVT(firstRef) {
		t.Fatalf("identical build ref = %v, want %v", secondRef, firstRef)
	}
	assertCommitTestRevisions(t, ctx, tb, []uint64{1})

	writeCommitTestFile(t, distPath, "plugin-HASH.mjs", "export const value = 2;\n")
	changedCtx := WithManifestCommitIdentity(ctx, firstResult, deps)
	thirdManifest, thirdRef := commitManifestForTest(t, changedCtx, tb, 2, distPath, assetsPath)
	if thirdManifest.GetMeta().GetRev() != 2 {
		t.Fatalf("changed build rev = %d, want 2", thirdManifest.GetMeta().GetRev())
	}
	if thirdRef.EqualVT(firstRef) {
		t.Fatal("changed build reused the first manifest ref")
	}
	assertCommitTestRevisions(t, ctx, tb, []uint64{2, 1})
}

func commitManifestForTest(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	rev uint64,
	distPath string,
	assetsPath string,
) (*bldr_manifest.Manifest, *bucket.ObjectRef) {
	t.Helper()

	meta := bldr_manifest.NewManifestMeta(
		"spacewave-js-plugin",
		bldr_manifest.BuildType_DEV,
		"js",
		rev,
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

func assertCommitTestRevisions(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	want []uint64,
) {
	t.Helper()

	got, errs, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		tb.GetWorldState(),
		"spacewave-js-plugin",
		[]string{"js"},
		tb.GetPluginHostObjKey(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 0 {
		t.Fatalf("collect manifest errors: %v", errs)
	}
	if len(got) != len(want) {
		t.Fatalf("collected revisions = %d, want %d", len(got), len(want))
	}
	for i, rev := range want {
		if got[i].GetRev() != rev {
			t.Fatalf("collected[%d] rev = %d, want %d", i, got[i].GetRev(), rev)
		}
	}
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
