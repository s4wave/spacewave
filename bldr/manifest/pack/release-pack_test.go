package bldr_manifest_pack

import (
	"bytes"
	"context"
	"testing"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestProduceReleasePackIsDeterministic checks that packing the same content
// twice, from sources committed at different times into different worlds,
// yields identical blocks: the pack a producer writes depends on the files
// alone.
func TestProduceReleasePackIsDeterministic(t *testing.T) {
	// Pack one manifest from a fresh source and scratch world.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	pack := func() ([]byte, *ManifestPackMetadata) {
		// Commit the manifest into a new source world.
		src := newTestWorld(t, ctx, le)
		scratch := newTestWorld(t, ctx, le)
		tuple := testManifestPackTuple()
		ref := storeTestManifest(t, ctx, src, tuple, false, true)

		// Produce the release pack from the source manifest.
		var buf bytes.Buffer
		meta, refs, err := ProduceReleasePack(ctx, le, &ReleasePackConfig{
			WorldState:     scratch,
			Sender:         peer.ID("sender"),
			BundleKey:      "plugin-handoff",
			Sources:        []ReleaseSource{{Access: src.AccessWorldState, Ref: ref.GetManifestRef()}},
			GitSHA:         "0123456789abcdef0123456789abcdef01234567",
			ProducerTarget: "plugin-release",
			CacheSchema:    "manifest-pack-v1",
			Writer:         &buf,
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(refs) != 1 {
			t.Fatalf("encoded manifests = %d, want 1", len(refs))
		}
		return buf.Bytes(), meta
	}

	// Require both packs to carry the same bytes and identity.
	a, metaA := pack()
	b, metaB := pack()
	if !bytes.Equal(a, b) {
		t.Fatal("pack bytes differ between identical builds")
	}
	if !bytes.Equal(metaA.GetPackSha256(), metaB.GetPackSha256()) || !metaA.GetManifestBundleRef().EqualVT(metaB.GetManifestBundleRef()) {
		t.Fatal("pack identity differs between identical builds")
	}
}
