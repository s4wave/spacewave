package bldr_manifest_world

import (
	"context"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6/memfs"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// TestEncodeReleaseManifestIgnoresSourceTimestamps checks that the encoding
// depends on the files alone: the same content committed at two times encodes
// to the same dist and assets references.
func TestEncodeReleaseManifestIgnoresSourceTimestamps(t *testing.T) {
	// Start a testbed.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Mount a mock World over the testbed's empty cursor.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer ocs.Release()
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err)
	}

	// Encode the same Manifest committed at two different times.
	meta := &bldr_manifest.ManifestMeta{ManifestId: "plugin", BuildType: "release", PlatformId: "js", Rev: 1}
	encode := func(committedAt time.Time) *bldr_manifest.Manifest {
		src := commitTestManifest(t, ctx, le, ws, meta, committedAt)
		out, _, err := EncodeReleaseManifest(ctx, le, ws.AccessWorldState, src, meta, ws)
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	a := encode(time.Unix(1_000, 0))
	b := encode(time.Unix(2_000, 0))

	// Require the same dist and assets references from both encodings.
	if a.GetDistFsRef().GetEmpty() {
		t.Fatal("encoded dist is empty")
	}
	if !a.GetDistFsRef().EqualVT(b.GetDistFsRef()) {
		t.Fatalf("dist refs differ across commit times: %v vs %v", a.GetDistFsRef(), b.GetDistFsRef())
	}
	if !a.GetAssetsFsRef().EqualVT(b.GetAssetsFsRef()) {
		t.Fatalf("assets refs differ across commit times: %v vs %v", a.GetAssetsFsRef(), b.GetAssetsFsRef())
	}
}

// commitTestManifest commits a one-file Manifest stamped with committedAt and
// returns its reference.
func commitTestManifest(
	t *testing.T,
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	meta *bldr_manifest.ManifestMeta,
	committedAt time.Time,
) *bucket.ObjectRef {
	// Write a one-file dist filesystem.
	t.Helper()
	dist := memfs.New()
	f, err := dist.Create("plugin.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("plugin bytes")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	// Commit the Manifest stamped with committedAt.
	ref, err := world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
		return bldr_manifest.CreateManifestWithBilly(
			ctx,
			bcs,
			&bldr_manifest.Manifest{Meta: meta.CloneVT(), Entrypoint: "plugin.bin"},
			dist,
			nil,
			timestamp.New(committedAt),
		)
	})
	if err != nil {
		t.Fatal(err)
	}
	return ref
}
