package publisher_test

import (
	"context"
	"crypto/rand"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
	"github.com/s4wave/spacewave/core/cdn/writer/writertest"
	"github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/core/release/publisher"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// exeSize is the size of each revision's executable. The bytes are random,
// so a revision's packs are about this size per platform.
const exeSize = 64 << 10

// revisions are the executables stored per platform and revision.
type revisions map[string]map[uint64][]byte

// of returns the executable stored for meta.
func (r revisions) of(meta *bldr_manifest.ManifestMeta) []byte {
	return r[meta.GetPlatformId()][meta.GetRev()]
}

// storeRevision stores a manifest of each app platform at rev with a fresh
// random executable, recording it in revs.
func storeRevision(t *testing.T, w *publisher.LocalWorld, d publisher.Deployment, rev uint64, revs revisions) {
	// Open a write transaction; the first revision creates the store.
	ctx := t.Context()
	tx, err := w.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	manifestKey := d.ProjectID + "/release/manifests"
	if rev == 1 {
		if _, err := manifest_world.CreateManifestStore(ctx, tx, manifestKey); err != nil {
			t.Fatal(err)
		}
	}

	// Store each platform's build of rev.
	for _, a := range apps {
		meta := a.meta.CloneVT()
		meta.Rev = rev
		exe := make([]byte, exeSize)
		_, _ = rand.Read(exe)
		ref, err := world.AccessObject(ctx, tx.AccessWorldState, nil, func(cursor *block.Cursor) error {
			return bldr_manifest.CreateManifestWithIoFS(ctx, cursor, bldr_manifest.NewManifest(meta, "app"), fstest.MapFS{"app": {Data: exe, Mode: 0o755}}, nil, nil)
		})
		if err != nil {
			t.Fatal(err)
		}
		setManifest(t, tx, manifestKey, meta, ref)
		if revs[meta.GetPlatformId()] == nil {
			revs[meta.GetPlatformId()] = make(map[uint64][]byte)
		}
		revs[meta.GetPlatformId()][rev] = exe
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// storedRevs returns the revisions of each platform in w's manifest store,
// newest first.
func storedRevs(t *testing.T, w *publisher.LocalWorld, d publisher.Deployment) map[string][]uint64 {
	// Read the manifest store.
	ctx := t.Context()
	tx, err := w.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	groups, _, err := manifest_world.CollectManifests(ctx, tx, nil, d.ProjectID+"/release/manifests")
	if err != nil {
		t.Fatal(err)
	}

	// Group the revisions by platform.
	out := make(map[string][]uint64)
	for _, manifests := range groups {
		for _, m := range manifests {
			platform := m.Manifest.GetMeta().GetPlatformId()
			out[platform] = append(out[platform], m.GetRev())
		}
	}
	return out
}

// spaceBytes returns the bytes of the Space's current packs.
func spaceBytes(t *testing.T, r *writertest.Rig) uint64 {
	pulled, err := r.Client(r.Owner).SyncPull(t.Context(), writertest.Space, 0)
	if err != nil {
		t.Fatal(err)
	}
	var total uint64
	for _, entry := range pulled.GetEntries() {
		if !entry.IsSuperseded() {
			total += entry.GetSizeBytes()
		}
	}
	return total
}

// TestReleaseRetainsAndReissues releases five revisions with the default
// retention, then rolls one platform back by reissuing its oldest retained
// revision. The release World keeps three revisions per platform, the Space
// supersedes the packs of the rest, and a launcher reads the reissued
// executable at a revision above the newest.
func TestReleaseRetainsAndReissues(t *testing.T) {
	// Release five revisions of every platform.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	r := writertest.NewRig(t)
	d := deployment(r)
	w, err := publisher.OpenLocalWorld(ctx, le, filepath.Join(t.TempDir(), "release.s4wave"), d.ProjectID+"-release-world", d.ProjectID+"-release")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Release)
	revs := make(revisions)
	opts := publisher.ReleaseOptions{
		Deployment: d,
		ReleaseKey: writertest.NewKey(t),
		DistKey:    writertest.NewKey(t),
		Publish:    cdn_publish.Options{Client: r.Client(r.Owner), OwnerKeyPem: r.OwnerPem, Logger: le},
	}
	for rev := uint64(1); rev <= 5; rev++ {
		storeRevision(t, w, d, rev, revs)
		opts.Rev, opts.Metadata = rev, &release.ReleaseMetadata{Version: "1.0.0", Rev: rev}
		if _, err := publisher.Release(ctx, w.Engine, opts); err != nil {
			t.Fatalf("release %d: %v", rev, err)
		}
	}

	// The World keeps the newest three revisions of each platform, and the
	// Space's current packs hold about their content, not the pruned
	// revisions'.
	for platform, got := range storedRevs(t, w, d) {
		if !slices.Equal(got, []uint64{5, 4, 3}) {
			t.Fatalf("%s kept revisions %v, want [5 4 3]", platform, got)
		}
	}
	retained := uint64(3 * len(apps) * exeSize)
	if got := spaceBytes(t, r); got < retained || got >= retained+uint64(len(apps)*exeSize) {
		t.Fatalf("the Space holds %d bytes for %d bytes of retained executables", got, retained)
	}

	// Reissue linux's oldest retained revision.
	linux := apps[0].meta.CloneVT()
	linux.Rev = 3
	opts.Rev, opts.Metadata = 6, &release.ReleaseMetadata{Version: "1.0.0-rollback", Rev: 6}
	released, err := publisher.Reissue(ctx, w.Engine, opts, linux)
	if err != nil {
		t.Fatal(err)
	}

	// The channel selects the copy above the newest, beside windows' newest.
	selected := make(map[string]uint64)
	for _, ref := range released.Metadata.GetManifestRefs() {
		selected[ref.GetMeta().GetPlatformId()] = ref.GetMeta().GetRev()
	}
	if selected[linux.GetPlatformId()] != 6 || selected[apps[1].meta.GetPlatformId()] != 5 {
		t.Fatalf("the reissue selected revisions %v", selected)
	}

	// A launcher reads revision 3's executable as revision 6, though
	// retention has since dropped revision 3's own manifest.
	if got := storedRevs(t, w, d)[linux.GetPlatformId()]; !slices.Equal(got, []uint64{6, 5, 4}) {
		t.Fatalf("linux kept revisions %v after the reissue, want [6 5 4]", got)
	}
	ws := mountLauncher(t, ctx, le, d)
	checkLauncherReads(t, ctx, le, ws, d.ChannelKey, released.Metadata, func(meta *bldr_manifest.ManifestMeta) []byte {
		if meta.GetPlatformId() == linux.GetPlatformId() && meta.GetRev() == 6 {
			return revs.of(linux)
		}
		return revs.of(meta)
	})
}
