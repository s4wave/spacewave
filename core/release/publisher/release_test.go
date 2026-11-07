package publisher_test

import (
	"bytes"
	"context"
	"path"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_fetch_world "github.com/s4wave/spacewave/bldr/manifest/fetch/world"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
	"github.com/s4wave/spacewave/core/cdn/writer/writertest"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	"github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/core/release/publisher"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// app is one platform's executable in the test release.
type app struct {
	meta *bldr_manifest.ManifestMeta
	exe  []byte
}

// apps are the test release's platforms.
var apps = []app{
	{&bldr_manifest.ManifestMeta{ManifestId: "demo-desktop", BuildType: "release", PlatformId: "desktop/linux/amd64", Rev: 1}, bytes.Repeat([]byte("linux executable "), 4096)},
	{&bldr_manifest.ManifestMeta{ManifestId: "demo-desktop", BuildType: "release", PlatformId: "desktop/windows/amd64", Rev: 1}, bytes.Repeat([]byte("windows executable "), 4096)},
}

// deployment returns the test Deployment writing r's Space.
func deployment(r *writertest.Rig) publisher.Deployment {
	return publisher.Deployment{
		ProjectID:          "demo",
		ChannelKey:         "stable",
		SpaceID:            writertest.Space,
		CdnBaseURL:         r.PackBase(),
		RootPointerBaseURL: r.RootsURL,
	}
}

// releaseWorld opens a local release World for d holding apps' manifests.
func releaseWorld(t *testing.T, d publisher.Deployment) (*publisher.LocalWorld, []*bucket.ObjectRef) {
	// Open the World in the bucket the launcher reads.
	ctx := t.Context()
	w, err := publisher.OpenLocalWorld(ctx, logrus.NewEntry(logrus.New()), filepath.Join(t.TempDir(), "release.s4wave"), d.ProjectID+"-release-world", d.ProjectID+"-release")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Release)
	tx, err := w.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Store each platform's manifest in the manifest store.
	manifestKey := d.ProjectID + "/release/manifests"
	if _, err := manifest_world.CreateManifestStore(ctx, tx, manifestKey); err != nil {
		t.Fatal(err)
	}
	var refs []*bucket.ObjectRef
	for _, a := range apps {
		ref, err := world.AccessObject(ctx, tx.AccessWorldState, nil, func(cursor *block.Cursor) error {
			return bldr_manifest.CreateManifestWithIoFS(ctx, cursor, bldr_manifest.NewManifest(a.meta, "app"), fstest.MapFS{"app": {Data: a.exe, Mode: 0o755}}, nil, nil)
		})
		if err != nil {
			t.Fatal(err)
		}
		setManifest(t, tx, manifestKey, a.meta, ref)
		refs = append(refs, ref)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return w, refs
}

// setManifest links the manifest at ref into the store at manifestKey.
func setManifest(t *testing.T, tx world.Tx, manifestKey string, meta *bldr_manifest.ManifestMeta, ref *bucket.ObjectRef) {
	ctx := t.Context()
	key := bldr_manifest.NewManifestKey(manifestKey, meta)
	if _, _, err := manifest_world.SetManifest(ctx, tx, "", key, ref); err != nil {
		t.Fatal(err)
	}
	if err := tx.SetGraphQuad(ctx, manifest_world.NewManifestQuad(manifestKey, key, meta.GetManifestId())); err != nil {
		t.Fatal(err)
	}
}

// TestReleaseLauncherFetchesEveryManifest releases a local World to a CDN
// Space and reads it as a launcher does: the launcher verifies the signed
// selector, mounts the Space through the Deployment's host config, and reads
// the selected channel and every selected manifest from the Space.
func TestReleaseLauncherFetchesEveryManifest(t *testing.T) {
	// Release the World.
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	r := writertest.NewRig(t)
	d := deployment(r)
	w, _ := releaseWorld(t, d)
	releaseKey, distKey := writertest.NewKey(t), writertest.NewKey(t)
	released, err := publisher.Release(ctx, w.Engine, publisher.ReleaseOptions{
		Deployment: d,
		Metadata:   &release.ReleaseMetadata{Version: "1.0.0", Rev: 1},
		ReleaseKey: releaseKey,
		DistKey:    distKey,
		Rev:        1,
		Publish:    cdn_publish.Options{Client: r.Client(r.Owner), OwnerKeyPem: r.OwnerPem},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(released.Metadata.GetManifestRefs()); n != len(apps) {
		t.Fatalf("released %d manifests, want %d", n, len(apps))
	}

	// Verify the selector against the pinned DistConfig peer.
	conf, _, signer, err := spacewave_launcher.ParseDistConfigPackedMsg(le, []byte(released.Signed), []peer.ID{writertest.PeerOf(t, distKey)}, d.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	if signer != writertest.PeerOf(t, distKey) || !conf.EqualVT(released.DistConfig) || conf.GetChannelKey() != d.ChannelKey {
		t.Fatal("the launcher verified a different selector than the release signed")
	}

	// A launcher mounting the Space reads the channel and every executable.
	ws := mountLauncher(t, ctx, le, d)
	checkLauncherReads(t, ctx, le, ws, conf.GetChannelKey(), released.Metadata, func(meta *bldr_manifest.ManifestMeta) []byte {
		return exeOf(t, meta)
	})
}

// mountLauncher mounts d's Space on a launcher bus through the Deployment's
// host config and returns the mounted release World.
func mountLauncher(t *testing.T, ctx context.Context, le *logrus.Entry, d publisher.Deployment) world.WorldState {
	// Run the host config's controllers on a launcher bus.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	b := tb.Bus
	tb.StaticResolver.AddFactory(cdn_world_controller.NewFactory(b))
	tb.StaticResolver.AddFactory(block_store_bucket.NewFactory(b))
	tb.StaticResolver.AddFactory(manifest_fetch_world.NewFactory(b))
	host, err := d.HostConfigSet()
	if err != nil {
		t.Fatal(err)
	}
	cs, err := configset_proto.ConfigSetMap(host).Resolve(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	_, applyRef, err := b.AddDirective(ctx, configset.NewApplyConfigSet(cs), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(applyRef.Release)

	// Look up the mounted release World.
	eng, _, engRef, err := world.ExLookupWorldEngine(ctx, b, false, d.ProjectID+"-release-world", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engRef.Release)
	return world.NewEngineWorldState(eng, false)
}

// checkLauncherReads reads channelKey's metadata from the mounted World ws,
// requires it to be staged, and reads each selected manifest's executable,
// requiring the one want returns for its meta.
func checkLauncherReads(t *testing.T, ctx context.Context, le *logrus.Entry, ws world.WorldState, channelKey string, staged *release.ReleaseMetadata, want func(*bldr_manifest.ManifestMeta) []byte) {
	// The mounted World holds the staged channel.
	_, _, err := world.AccessWorldObject(ctx, ws, path.Join(publisher.ChannelDirectoryKey, channelKey), false, func(cursor *block.Cursor) error {
		got, err := block.UnmarshalBlock[release.ReleaseMetadata](ctx, cursor)
		if err == nil && !got.EqualVT(staged) {
			t.Fatal("the launcher read different channel metadata than the release staged")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Each selected manifest checks out its executable.
	for _, ref := range staged.GetManifestRefs() {
		exe := want(ref.GetMeta())
		err := manifest_world.AccessManifest(ctx, le, ws.AccessWorldState, ref.GetManifestRef(), func(ctx context.Context, _ *bucket_lookup.Cursor, _ *block.Cursor, _ *bldr_manifest.Manifest, distFS *unixfs.FSHandle, _ *unixfs.FSHandle) error {
			if !bytes.Equal(readFile(t, ctx, distFS, "app"), exe) {
				t.Fatalf("%s: the launcher read a different executable", ref.GetMeta().GetPlatformId())
			}
			return nil
		})
		if err != nil {
			t.Fatalf("%s: %v", ref.GetMeta().GetPlatformId(), err)
		}
	}
}

// exeOf returns the executable of the app with meta's platform.
func exeOf(t *testing.T, meta *bldr_manifest.ManifestMeta) []byte {
	for _, a := range apps {
		if a.meta.GetPlatformId() == meta.GetPlatformId() {
			return a.exe
		}
	}
	t.Fatalf("no app for %s", meta.GetPlatformId())
	return nil
}

// readFile reads the file at name under fsh.
func readFile(t *testing.T, ctx context.Context, fsh *unixfs.FSHandle, name string) []byte {
	// Look up the file and size its buffer.
	file, _, err := fsh.LookupPath(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Release()
	size, err := file.GetSize(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Read the whole file.
	data := make([]byte, size)
	if _, err := file.ReadAt(ctx, 0, data); err != nil {
		t.Fatal(err)
	}
	return data
}

// failingUpload refuses every pack upload.
type failingUpload struct {
	cdn_publish.SessionClient
	uploads int
}

// SyncPull reports an empty Space.
func (c *failingUpload) SyncPull(context.Context, string, uint64) (*packfile.PullResponse, error) {
	return &packfile.PullResponse{}, nil
}

// SyncPushData refuses the upload.
func (c *failingUpload) SyncPushData(context.Context, string, string, int, []byte, []byte, []byte, uint32) error {
	c.uploads++
	return context.DeadlineExceeded
}

// PostCheckpoint must not be reached.
func (c *failingUpload) PostCheckpoint(context.Context, string, *sobject.SOCheckpoint) error {
	panic("checkpoint posted after a failed upload")
}

// TestReleaseSignsNothingBeforeDurable fails a release at the upload and at a
// manifest the World cannot check out; neither returns a signed selector.
func TestReleaseSignsNothingBeforeDurable(t *testing.T) {
	// A failed upload returns before signing.
	ctx := t.Context()
	d := publisher.Deployment{ProjectID: "demo", ChannelKey: "stable", SpaceID: "demo-space", CdnBaseURL: "https://unused.example"}
	w, refs := releaseWorld(t, d)
	client := &failingUpload{}
	opts := publisher.ReleaseOptions{
		Deployment: d,
		Metadata:   &release.ReleaseMetadata{Version: "1.0.0", Rev: 1},
		ReleaseKey: writertest.NewKey(t),
		DistKey:    writertest.NewKey(t),
		Rev:        1,
		Publish:    cdn_publish.Options{Client: client, OwnerKeyPem: "must-not-read.pem"},
	}
	released, err := publisher.Release(ctx, w.Engine, opts)
	if err == nil || released != nil || client.uploads == 0 {
		t.Fatalf("a failed upload released %v after %d uploads: %v", released, client.uploads, err)
	}

	// Add a platform whose manifest the World lacks; staging stops the
	// release before any upload.
	tx, err := w.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	absent := refs[0].CloneVT()
	absent.RootRef.Hash.Hash[0] ^= 0xff
	setManifest(t, tx, d.ProjectID+"/release/manifests", &bldr_manifest.ManifestMeta{ManifestId: "demo-desktop", BuildType: "release", PlatformId: "desktop/darwin/arm64", Rev: 1}, absent)
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	client = &failingUpload{}
	opts.Publish.Client = client
	released, err = publisher.Release(ctx, w.Engine, opts)
	if err == nil || released != nil || client.uploads != 0 {
		t.Fatalf("a release missing content uploaded %d packs and released %v: %v", client.uploads, released, err)
	}
}
