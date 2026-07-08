package manifest_fetch_world_test

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	cdc "github.com/aperturerobotics/controllerbus/directive/controller"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_fetch_world "github.com/s4wave/spacewave/bldr/manifest/fetch/world"
	spacewave_release "github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

func TestFetchManifestReadsReleaseMetadataManifestRefsWithoutGraphLinks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	le := logrus.NewEntry(logrus.New())
	eng, ws := buildReleaseMetadataFetchTestWorld(t, ctx, le)

	wantRef := testReleaseManifestRef("spacewave-notes", bldr_manifest.BuildType_RELEASE, "js", 7)
	wrongPlatform := testReleaseManifestRef("spacewave-notes", bldr_manifest.BuildType_RELEASE, "desktop/darwin/arm64", 8)
	wrongManifest := testReleaseManifestRef("spacewave-other", bldr_manifest.BuildType_RELEASE, "js", 9)
	metadata := testReleaseMetadata("stable", []*bldr_manifest.ManifestRef{wantRef, wrongPlatform, wrongManifest})
	metadataRef := writeReleaseMetadataFetchTestBlock(t, ctx, ws, "release/metadata/stable", metadata)
	writeReleaseMetadataFetchTestBlock(t, ctx, ws, "release/metadata", &spacewave_release.ChannelDirectory{
		Channels: []*spacewave_release.ChannelEntry{{
			ChannelKey:         "stable",
			ReleaseMetadataRef: metadataRef,
		}},
	})

	dc := cdc.NewController(ctx, le)
	b := inmem.NewBus(dc)
	worldRel, err := b.AddController(ctx, &releaseMetadataFetchWorldController{engine: eng}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer worldRel()

	fetchRel, err := b.AddController(ctx, manifest_fetch_world.NewController(le, b, &manifest_fetch_world.Config{
		EngineId:                  "release-world-test",
		DisableWatch:              true,
		ReleaseMetadataChannelKey: "stable",
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fetchRel()

	val, _, ref, err := bus.ExecWaitValue[*bldr_manifest.FetchManifestValue](
		ctx,
		b,
		bldr_manifest.NewFetchManifest(
			"spacewave-notes",
			[]bldr_manifest.BuildType{bldr_manifest.BuildType_RELEASE},
			[]string{"js"},
			0,
		),
		bus.ReturnWhenIdle(),
		nil,
		func(val *bldr_manifest.FetchManifestValue) (bool, error) {
			return len(val.GetManifestRefs()) != 0, nil
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ref.Release()

	refs := val.GetManifestRefs()
	if len(refs) != 1 {
		t.Fatalf("manifest refs = %d, want exactly the release metadata spacewave-notes/js ref", len(refs))
	}
	got := refs[0]
	if !got.GetMeta().EqualVT(wantRef.GetMeta()) {
		t.Fatalf("manifest ref meta = %+v, want %+v", got.GetMeta(), wantRef.GetMeta())
	}
	if !got.GetManifestRef().EqualVT(wantRef.GetManifestRef()) {
		t.Fatalf("manifest object ref = %+v, want %+v", got.GetManifestRef(), wantRef.GetManifestRef())
	}
	if got.GetMeta().GetPlatformId() == wrongPlatform.GetMeta().GetPlatformId() {
		t.Fatalf("wrong-platform release metadata ref was returned: %+v", got.GetMeta())
	}
	if got.GetMeta().GetManifestId() == wrongManifest.GetMeta().GetManifestId() {
		t.Fatalf("wrong-manifest release metadata ref was returned: %+v", got.GetMeta())
	}
}

func buildReleaseMetadataFetchTestWorld(
	t *testing.T,
	ctx context.Context,
	le *logrus.Entry,
) (world.Engine, world.WorldState) {
	t.Helper()
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(ocs.Release)

	eng, err := world_block.NewEngine(ctx, le, ocs, nil, nil, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() {
		if err := eng.Close(); err != nil {
			t.Fatal(err.Error())
		}
	})

	return eng, world.NewEngineWorldState(eng, true)
}

type releaseMetadataFetchWorldController struct {
	engine world.Engine
}

func (c *releaseMetadataFetchWorldController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("release-metadata-fetch-world-test", controller.MustParseVersion("0.0.1"), "release metadata fetch world test")
}

func (c *releaseMetadataFetchWorldController) Execute(context.Context) error { return nil }

func (c *releaseMetadataFetchWorldController) Close() error { return nil }

func (c *releaseMetadataFetchWorldController) HandleDirective(
	_ context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(world.LookupWorldEngine)
	if !ok || dir.LookupWorldEngineID() != "release-world-test" {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver[world.LookupWorldEngineValue]([]world.LookupWorldEngineValue{c.engine}), nil)
}

func writeReleaseMetadataFetchTestBlock(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	blk block.Block,
) *block.BlockRef {
	t.Helper()
	objRef, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		bcs.ClearAllRefs()
		bcs.SetBlock(blk, true)
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	return objRef.GetRootRef()
}

func testReleaseMetadata(channelKey string, manifestRefs []*bldr_manifest.ManifestRef) *spacewave_release.ReleaseMetadata {
	return &spacewave_release.ReleaseMetadata{
		ProjectId:    "spacewave",
		Rev:          1,
		Version:      "0.1.0",
		ChannelKey:   channelKey,
		ManifestRefs: manifestRefs,
		BrowserShell: &spacewave_release.BrowserShellMetadata{
			Version:           "0.1.0",
			GenerationId:      "gen-1",
			EntrypointPath:    "/b/entrypoint/boot.mjs",
			ServiceWorkerPath: "/b/entrypoint/sw.js",
			SharedWorkerPath:  "/b/entrypoint/shared-worker.js",
			Assets: []*spacewave_release.BrowserAsset{{
				Path:        "/b/entrypoint/boot.mjs",
				Size:        1,
				Sha256:      testReleaseSHA256(1),
				ContentType: "text/javascript",
			}},
		},
		MinimumLauncherVersion: "0.1.0",
	}
}

func testReleaseManifestRef(
	manifestID string,
	buildType bldr_manifest.BuildType,
	platformID string,
	rev uint64,
) *bldr_manifest.ManifestRef {
	return &bldr_manifest.ManifestRef{
		Meta: &bldr_manifest.ManifestMeta{
			ManifestId: manifestID,
			BuildType:  buildType.String(),
			PlatformId: platformID,
			Rev:        rev,
		},
		ManifestRef: &bucket.ObjectRef{RootRef: &block.BlockRef{Hash: &hash.Hash{
			HashType: hash.HashType_HashType_SHA256,
			Hash:     testReleaseSHA256(byte(rev)),
		}}},
	}
}

func testReleaseSHA256(first byte) []byte {
	out := make([]byte, 32)
	out[0] = first
	return out
}
