package bldr_manifest_pack

import (
	"bytes"
	"context"
	"strings"
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

// TestProduceReleasePackOrdersTuplesLikeTheBundle checks that the tuples and
// encoded references come out in bundle order whatever order the sources
// arrive in. The bundle sorts its entries by key, and the consumer pairs each
// tuple with the bundle entry at the same index.
func TestProduceReleasePackOrdersTuplesLikeTheBundle(t *testing.T) {
	// Store two manifests in one source world.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	src := newTestWorld(t, ctx, le)
	var sources []ReleaseSource
	for _, id := range []string{"plugin", "plugin-b"} {
		tuple := testManifestPackTuple()
		tuple.ManifestId = id
		ref := storeTestManifest(t, ctx, src, tuple, false, true)
		sources = append(sources, ReleaseSource{Access: src.AccessWorldState, Ref: ref.GetManifestRef()})
	}

	// Pack the sources in both orders.
	var ids [][]string
	for _, ordered := range [][]ReleaseSource{sources, {sources[1], sources[0]}} {
		meta, refs, err := ProduceReleasePack(ctx, le, &ReleasePackConfig{
			WorldState:     newTestWorld(t, ctx, le),
			Sender:         peer.ID("sender"),
			BundleKey:      "plugin-handoff",
			Sources:        ordered,
			GitSHA:         "0123456789abcdef0123456789abcdef01234567",
			ProducerTarget: "plugin-release",
			CacheSchema:    "manifest-pack-v1",
			Writer:         &bytes.Buffer{},
		})
		if err != nil {
			t.Fatal(err)
		}

		// Require each tuple to describe the reference at its index.
		var got []string
		for i, tuple := range meta.GetManifests() {
			if tuple.GetManifestId() != refs[i].GetMeta().GetManifestId() {
				t.Fatalf("tuple %d is %q, reference is %q", i, tuple.GetManifestId(), refs[i].GetMeta().GetManifestId())
			}
			got = append(got, tuple.GetManifestId())
		}
		ids = append(ids, got)
	}

	// Require the same tuple order for both source orders.
	if strings.Join(ids[0], ",") != "plugin-b,plugin" || strings.Join(ids[1], ",") != "plugin-b,plugin" {
		t.Fatalf("tuple order = %v, want plugin-b,plugin for both source orders", ids)
	}
}
