package bldr_manifest_world

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aperturerobotics/cayley/quad"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestNewManifestQuadLabelsConcreteManifestID(t *testing.T) {
	gq := NewManifestQuad("plugin-host", "plugin-host/ref/spacewave-web", "spacewave-web")
	want := quad.IRI("spacewave-web").String()
	if gq.GetLabel() != want {
		t.Fatalf("manifest quad label = %q, want %q", gq.GetLabel(), want)
	}
}

func TestNewManifestQuadKeepsEmptyBundleLabel(t *testing.T) {
	gq := NewManifestQuad("plugin-host", "plugin-host/bundle", "")
	if gq.GetLabel() != "" {
		t.Fatalf("manifest quad label = %q, want empty bundle/store label", gq.GetLabel())
	}
}

func TestCollectReleaseWorldManifestsForManifestID(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const releaseManifestKey = "spacewave/release/manifests"
	if _, err := CreateManifestStore(ctx, ws, releaseManifestKey); err != nil {
		t.Fatal(err.Error())
	}

	ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 11)
	if err := ExStoreManifestOp(
		ctx,
		ws,
		peer.ID("test"),
		"release/manifests/spacewave-web/js",
		[]string{releaseManifestKey},
		ref,
	); err != nil {
		t.Fatal(err.Error())
	}
	quads, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(
			releaseManifestKey,
			PredManifest.String(),
			"release/manifests/spacewave-web/js",
			"",
		),
		1,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(quads) != 1 {
		t.Fatalf("manifest graph edge count = %d", len(quads))
	}
	wantLabel := quad.IRI("spacewave-web").String()
	if quads[0].GetLabel() != wantLabel {
		t.Fatalf("manifest graph edge label = %q, want %q", quads[0].GetLabel(), wantLabel)
	}

	got, errs, err := CollectManifestsForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		releaseManifestKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(errs) != 0 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if got[0].Manifest.GetMeta().GetManifestId() != "spacewave-web" {
		t.Fatalf("manifest id = %q", got[0].Manifest.GetMeta().GetManifestId())
	}
	if got[0].Manifest.GetMeta().GetPlatformId() != "js" {
		t.Fatalf("platform id = %q", got[0].Manifest.GetMeta().GetPlatformId())
	}
	if !got[0].ManifestRef.EqualVT(ref.GetManifestRef()) {
		t.Fatalf("manifest ref was not preserved")
	}
}

func TestCollectStartupManifestsSkipsUnreadableLinkedRef(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	goodRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 7)
	const goodRefKey = "plugin-host/ref/good"
	storeTestManifestRefObject(t, ctx, ws, goodRefKey, goodRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, goodRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	badRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 9)
	badRef.ManifestRef.RootRef.Hash.Hash[0] ^= 0xff
	const badRefKey = "plugin-host/ref/missing"
	storeTestManifestRefObject(t, ctx, ws, badRefKey, badRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, badRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	defaultGot, defaultErrs, err := CollectManifestsForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(defaultErrs) != 0 {
		t.Fatalf("default manifest errors = %v", defaultErrs)
	}
	if len(defaultGot) != 0 {
		t.Fatalf("default manifest count = %d", len(defaultGot))
	}

	got, errs, err := CollectStartupManifestsForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(errs) != 1 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if !strings.Contains(errs[0].Error(), badRefKey) {
		t.Fatalf("manifest error %q does not mention bad ref key %q", errs[0].Error(), badRefKey)
	}
	if !strings.Contains(errs[0].Error(), badRef.GetManifestRef().GetRootRef().MarshalString()) {
		t.Fatalf("manifest error %q does not mention bad root ref", errs[0].Error())
	}
	if !errors.Is(errs[0], block.ErrNotFound) {
		t.Fatalf("manifest error = %v, want block not found", errs[0])
	}
	var skipErr *StartupManifestSkipError
	if !errors.As(errs[0], &skipErr) {
		t.Fatalf("manifest error = %T, want StartupManifestSkipError", errs[0])
	}
	if skipErr.ObjectKey != badRefKey {
		t.Fatalf("skip object key = %q, want %q", skipErr.ObjectKey, badRefKey)
	}
	if !skipErr.ObjectRef.EqualVT(badRef.GetManifestRef()) {
		t.Fatalf("skip object ref was not preserved")
	}
	if len(got) != 1 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if got[0].GetRev() != 7 {
		t.Fatalf("manifest rev = %d", got[0].GetRev())
	}
	if !got[0].ManifestRef.EqualVT(goodRef.GetManifestRef()) {
		t.Fatalf("manifest ref was not preserved")
	}
}

func TestCollectStartupManifestsSkipsUnavailableBucketRef(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	goodRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 7)
	const goodRefKey = "plugin-host/ref/good"
	storeTestManifestRefObject(t, ctx, ws, goodRefKey, goodRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, goodRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	badRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 9)
	badRef.GetManifestRef().BucketId = "missing-bucket"
	const badRefKey = "plugin-host/ref/missing-bucket"
	storeTestManifestRefObject(t, ctx, ws, badRefKey, badRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, badRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	got, errs, err := CollectStartupManifestsForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(errs) != 1 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if !errors.Is(errs[0], bucket.ErrBucketNotFound) {
		t.Fatalf("manifest error = %v, want bucket not found", errs[0])
	}
	if !strings.Contains(errs[0].Error(), badRefKey) {
		t.Fatalf("manifest error %q does not mention bad ref key %q", errs[0].Error(), badRefKey)
	}
	if !strings.Contains(errs[0].Error(), "bucket=missing-bucket") {
		t.Fatalf("manifest error %q does not mention missing bucket", errs[0].Error())
	}
	if len(got) != 1 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if got[0].GetRev() != 7 {
		t.Fatalf("manifest rev = %d", got[0].GetRev())
	}
	if !got[0].ManifestRef.EqualVT(goodRef.GetManifestRef()) {
		t.Fatalf("manifest ref was not preserved")
	}
}

func TestDumpStartupManifestGraphForManifestIDIncludesRetainedRefDiagnostics(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	goodRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 7)
	const goodRefKey = "plugin-host/ref/good"
	storeTestManifestRefObject(t, ctx, ws, goodRefKey, goodRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, goodRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	badRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 9)
	badRef.GetManifestRef().BucketId = "missing-bucket"
	const badRefKey = "plugin-host/ref/missing-bucket"
	storeTestManifestRefObject(t, ctx, ws, badRefKey, badRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, badRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	legacyRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 11)
	legacyRef.GetManifestRef().BucketId = "legacy-missing-bucket"
	const legacyRefKey = "plugin-host/ref/legacy-missing-bucket"
	storeTestManifestRefObject(t, ctx, ws, legacyRefKey, legacyRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, legacyRefKey, "")); err != nil {
		t.Fatal(err.Error())
	}

	dump, err := DumpStartupManifestGraphForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	for _, want := range []string{
		"startup manifest graph manifest_id=spacewave-web platform_ids=js",
		"root plugin-host type=bldr/manifest-store",
		"edge plugin-host -> plugin-host/ref/good label=<spacewave-web>",
		"edge plugin-host -> plugin-host/ref/missing-bucket label=<spacewave-web>",
		"edge plugin-host -> plugin-host/ref/legacy-missing-bucket label=<empty>",
		"candidate plugin-host/ref/good type=<unknown>",
		"ref_meta=manifest_id=spacewave-web,build_type=production,platform_id=js,rev=7",
		"manifest_meta=manifest_id=spacewave-web,build_type=production,platform_id=js,rev=7",
		"candidate plugin-host/ref/missing-bucket type=<unknown>",
		"manifest_bucket=missing-bucket",
		"manifest_bucket=legacy-missing-bucket",
		"skip=bucket not found",
	} {
		if !strings.Contains(dump, want) {
			t.Fatalf("dump missing %q:\n%s", want, dump)
		}
	}
	if !strings.Contains(dump, "manifest_root="+badRef.GetManifestRef().GetRootRef().MarshalString()) {
		t.Fatalf("dump missing bad manifest root ref:\n%s", dump)
	}
	if !strings.Contains(dump, "object_root=") {
		t.Fatalf("dump missing object root refs:\n%s", dump)
	}
}

func TestCollectStartupManifestsForManifestIDNarrowsLabelsAndKeepsLegacyEmpty(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	exactRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 7)
	const exactRefKey = "plugin-host/ref/exact"
	storeTestManifestRefObject(t, ctx, ws, exactRefKey, exactRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, exactRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	legacyRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 5)
	const legacyRefKey = "plugin-host/ref/legacy-empty"
	storeTestManifestRefObject(t, ctx, ws, legacyRefKey, legacyRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, legacyRefKey, "")); err != nil {
		t.Fatal(err.Error())
	}

	unrelatedRef := createTestManifestRef(t, ctx, tb, "other-plugin", "js", 11)
	unrelatedRef.ManifestRef.RootRef.Hash.Hash[0] ^= 0xff
	const unrelatedRefKey = "plugin-host/ref/unrelated"
	storeTestManifestRefObject(t, ctx, ws, unrelatedRefKey, unrelatedRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, unrelatedRefKey, "other-plugin")); err != nil {
		t.Fatal(err.Error())
	}

	got, errs, err := CollectStartupManifestsForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(errs) != 0 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if len(got) != 2 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if got[0].GetRev() != 7 {
		t.Fatalf("exact manifest rev = %d", got[0].GetRev())
	}
	if got[1].GetRev() != 5 {
		t.Fatalf("legacy manifest rev = %d", got[1].GetRev())
	}
	if !got[0].ManifestRef.EqualVT(exactRef.GetManifestRef()) {
		t.Fatalf("exact manifest ref was not preserved")
	}
	if !got[1].ManifestRef.EqualVT(legacyRef.GetManifestRef()) {
		t.Fatalf("legacy manifest ref was not preserved")
	}
}

func TestCollectStartupManifestsForManifestIDCoverageMatrix(t *testing.T) {
	type setupFunc func(
		t *testing.T,
		ctx context.Context,
		tb *testbed.Testbed,
		ws world.WorldState,
		storeKey string,
	) (*manifest.ManifestRef, string)

	cases := []struct {
		name  string
		setup setupFunc
	}{
		{
			name: "direct manifest exact-label edge",
			setup: func(
				t *testing.T,
				ctx context.Context,
				tb *testbed.Testbed,
				ws world.WorldState,
				storeKey string,
			) (*manifest.ManifestRef, string) {
				ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 31)
				const manifestKey = "plugin-host/direct/exact"
				if _, _, err := SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
					t.Fatal(err.Error())
				}
				if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, manifestKey, "spacewave-web")); err != nil {
					t.Fatal(err.Error())
				}
				return ref, manifestKey
			},
		},
		{
			name: "direct manifest legacy empty-label edge",
			setup: func(
				t *testing.T,
				ctx context.Context,
				tb *testbed.Testbed,
				ws world.WorldState,
				storeKey string,
			) (*manifest.ManifestRef, string) {
				ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 32)
				const manifestKey = "plugin-host/direct/legacy-empty"
				if _, _, err := SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
					t.Fatal(err.Error())
				}
				if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, manifestKey, "")); err != nil {
					t.Fatal(err.Error())
				}
				return ref, manifestKey
			},
		},
		{
			name: "release-world manifest exact-label edge",
			setup: func(
				t *testing.T,
				ctx context.Context,
				tb *testbed.Testbed,
				ws world.WorldState,
				storeKey string,
			) (*manifest.ManifestRef, string) {
				ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 33)
				const manifestKey = "release/manifests/spacewave-web/js"
				if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{storeKey}, ref); err != nil {
					t.Fatal(err.Error())
				}
				return ref, manifestKey
			},
		},
		{
			name: "release-world manifest legacy empty-label edge",
			setup: func(
				t *testing.T,
				ctx context.Context,
				tb *testbed.Testbed,
				ws world.WorldState,
				storeKey string,
			) (*manifest.ManifestRef, string) {
				ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 34)
				const manifestKey = "release/manifests/spacewave-web/js/legacy-empty"
				if _, _, err := SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
					t.Fatal(err.Error())
				}
				if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, manifestKey, "")); err != nil {
					t.Fatal(err.Error())
				}
				return ref, manifestKey
			},
		},
		{
			name: "bundle manifest exact-label edge",
			setup: func(
				t *testing.T,
				ctx context.Context,
				tb *testbed.Testbed,
				ws world.WorldState,
				storeKey string,
			) (*manifest.ManifestRef, string) {
				ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 35)
				const manifestKey = "plugin-host/bundle/exact/manifest"
				if _, _, err := SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
					t.Fatal(err.Error())
				}
				const bundleKey = "plugin-host/bundle/exact"
				if _, _, err := CreateManifestBundle(ctx, ws, bundleKey, []string{manifestKey}, timestamp.Now()); err != nil {
					t.Fatal(err.Error())
				}
				if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, bundleKey, "spacewave-web")); err != nil {
					t.Fatal(err.Error())
				}
				return ref, manifestKey
			},
		},
		{
			name: "bundle manifest legacy empty-label edge",
			setup: func(
				t *testing.T,
				ctx context.Context,
				tb *testbed.Testbed,
				ws world.WorldState,
				storeKey string,
			) (*manifest.ManifestRef, string) {
				ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 36)
				const manifestKey = "plugin-host/bundle/legacy-empty/manifest"
				if _, _, err := SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
					t.Fatal(err.Error())
				}
				const bundleKey = "plugin-host/bundle/legacy-empty"
				if _, _, err := CreateManifestBundle(ctx, ws, bundleKey, []string{manifestKey}, timestamp.Now()); err != nil {
					t.Fatal(err.Error())
				}
				if err := ws.DeleteGraphQuad(ctx, NewManifestQuad(bundleKey, manifestKey, "spacewave-web")); err != nil {
					t.Fatal(err.Error())
				}
				if err := ws.SetGraphQuad(ctx, NewManifestQuad(bundleKey, manifestKey, "")); err != nil {
					t.Fatal(err.Error())
				}
				if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, bundleKey, "")); err != nil {
					t.Fatal(err.Error())
				}
				return ref, manifestKey
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			le := logrus.NewEntry(logrus.New())

			tb, err := testbed.NewTestbed(ctx, le)
			if err != nil {
				t.Fatal(err.Error())
			}
			defer tb.Release()

			ocs, err := tb.BuildEmptyCursor(ctx)
			if err != nil {
				t.Fatal(err.Error())
			}
			defer ocs.Release()

			ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
			if err != nil {
				t.Fatal(err.Error())
			}

			const storeKey = "plugin-host"
			if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
				t.Fatal(err.Error())
			}

			wantRef, wantManifestKey := tc.setup(t, ctx, tb, ws, storeKey)
			got, errs, err := CollectStartupManifestsForManifestID(
				ctx,
				ws,
				"spacewave-web",
				[]string{"js"},
				storeKey,
			)
			if err != nil {
				t.Fatal(err.Error())
			}
			if len(errs) != 0 {
				t.Fatalf("manifest errors = %v", errs)
			}
			if len(got) != 1 {
				t.Fatalf("manifest count = %d", len(got))
			}
			if got[0].ManifestKey != wantManifestKey {
				t.Fatalf("manifest key = %q, want %q", got[0].ManifestKey, wantManifestKey)
			}
			if !got[0].ManifestRef.EqualVT(wantRef.GetManifestRef()) {
				t.Fatalf("manifest ref was not preserved")
			}
		})
	}
}

func TestCollectStartupManifestsForManifestIDSkipsRefMetadataMismatchesBeforeOpen(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	goodRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 7)
	const goodRefKey = "plugin-host/ref/good"
	storeTestManifestRefObject(t, ctx, ws, goodRefKey, goodRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, goodRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	wrongIDRef := createTestManifestRef(t, ctx, tb, "other-plugin", "js", 11)
	wrongIDRef.ManifestRef.RootRef.Hash.Hash[0] ^= 0xff
	const wrongIDRefKey = "plugin-host/ref/wrong-id"
	storeTestManifestRefObject(t, ctx, ws, wrongIDRefKey, wrongIDRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, wrongIDRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	wrongPlatformRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "desktop/linux/amd64", 13)
	wrongPlatformRef.ManifestRef.RootRef.Hash.Hash[0] ^= 0xff
	const wrongPlatformRefKey = "plugin-host/ref/wrong-platform"
	storeTestManifestRefObject(t, ctx, ws, wrongPlatformRefKey, wrongPlatformRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, wrongPlatformRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	got, errs, err := CollectStartupManifestsForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(errs) != 0 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if !got[0].ManifestRef.EqualVT(goodRef.GetManifestRef()) {
		t.Fatalf("selected manifest ref = %v, want good ref", got[0].ManifestRef)
	}
}

func TestCollectStartupManifestsForManifestIDRejectsDecodedMetadataMismatch(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	decodedOtherRef := createTestManifestRef(t, ctx, tb, "other-plugin", "js", 11)
	refHint := manifest.NewManifestRef(
		&manifest.ManifestMeta{
			ManifestId: "spacewave-web",
			BuildType:  "production",
			PlatformId: "js",
			Rev:        11,
		},
		decodedOtherRef.GetManifestRef(),
	)
	const refHintKey = "plugin-host/ref/decoded-mismatch"
	storeTestManifestRefObject(t, ctx, ws, refHintKey, refHint)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, refHintKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	got, errs, err := CollectStartupManifestsForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(got) != 0 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if len(errs) != 1 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if !strings.Contains(errs[0].Error(), "manifest ref meta does not match manifest meta") {
		t.Fatalf("manifest error = %v, want decoded metadata mismatch", errs[0])
	}
}

func TestCollectStartupManifestsRejectsInvalidDecodedMetadata(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	invalidRef := createTestManifestRef(t, ctx, tb, "Spacewave-Web", "js", 11)
	const invalidManifestKey = "plugin-host/manifest/invalid"
	if _, _, err := SetManifest(ctx, ws, peer.ID("test"), invalidManifestKey, invalidRef.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, invalidManifestKey, "")); err != nil {
		t.Fatal(err.Error())
	}

	got, errs, err := CollectStartupManifests(ctx, ws, []string{"js"}, storeKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(got) != 0 {
		t.Fatalf("manifest set count = %d", len(got))
	}
	if len(errs) != 1 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if !strings.Contains(errs[0].Error(), "manifest_id") {
		t.Fatalf("manifest error = %v, want invalid manifest metadata", errs[0])
	}
}

func TestStartupManifestSkipErrorIncludesBucketDiagnostics(t *testing.T) {
	err := newStartupManifestSkipError(
		"plugin-host/ref/missing-bucket",
		&bucket.ObjectRef{BucketId: "missing-bucket"},
		bucket.ErrBucketNotFound,
	)
	if !errors.Is(err, bucket.ErrBucketNotFound) {
		t.Fatalf("skip error = %v, want bucket not found", err)
	}
	if !strings.Contains(err.Error(), "bucket=missing-bucket") {
		t.Fatalf("skip error %q does not mention missing bucket", err.Error())
	}
}

func TestStartupContextErrorClassifiesFatalContextErrors(t *testing.T) {
	if got := startupContextError(context.Canceled); !errors.Is(got, context.Canceled) {
		t.Fatalf("canceled error = %v, want context canceled", got)
	}
	if got := startupContextError(context.DeadlineExceeded); !errors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("deadline error = %v, want deadline exceeded", got)
	}
	if got := startupContextError(bucket.ErrBucketNotFound); got != nil {
		t.Fatalf("availability error = %v, want nil fatal context error", got)
	}
}

func TestCollectManifestsReportsUnreadableManifestObject(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	badRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 9).GetManifestRef().CloneVT()
	badRef.RootRef.Hash.Hash[0] ^= 0xff
	const badManifestKey = "plugin-host/manifest/bad"
	if _, _, err := SetManifest(ctx, ws, peer.ID("test"), badManifestKey, badRef); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, badManifestKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	got, errs, err := CollectManifestsForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(got) != 0 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if len(errs) != 1 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if !strings.Contains(errs[0].Error(), badManifestKey) {
		t.Fatalf("manifest error %q does not mention bad manifest key %q", errs[0].Error(), badManifestKey)
	}
}

func TestCollectDirectManifestForManifestID(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const manifestKey = "glados-core"
	ref := createTestManifestRef(t, ctx, tb, manifestKey, "js", 7)
	if _, _, err := SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(manifestKey, manifestKey, manifestKey)); err != nil {
		t.Fatal(err.Error())
	}

	got, errs, err := CollectManifestsForManifestID(
		ctx,
		ws,
		manifestKey,
		[]string{"js"},
		manifestKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(errs) != 0 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if got[0].Manifest.GetMeta().GetManifestId() != manifestKey {
		t.Fatalf("manifest id = %q", got[0].Manifest.GetMeta().GetManifestId())
	}
	if !got[0].ManifestRef.EqualVT(ref.GetManifestRef()) {
		t.Fatalf("manifest ref was not preserved")
	}
}

func createTestManifestRef(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	manifestID string,
	platformID string,
	rev uint64,
) *manifest.ManifestRef {
	t.Helper()

	meta := &manifest.ManifestMeta{
		ManifestId: manifestID,
		BuildType:  "production",
		PlatformId: platformID,
		Rev:        rev,
	}
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer oc.Release()

	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(manifest.NewManifest(meta, "entrypoint"), true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc.SetRootRef(rootRef)
	return manifest.NewManifestRef(meta, oc.GetRef())
}

func storeTestManifestRefObject(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	ref *manifest.ManifestRef,
) {
	t.Helper()

	if _, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(ref.CloneVT(), true)
		return nil
	}); err != nil {
		t.Fatal(err.Error())
	}
}
