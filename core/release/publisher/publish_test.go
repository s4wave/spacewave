package publisher

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
	"github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// failingUpload stops a publication at the cloud pack boundary.
type failingUpload struct {
	// SessionClient supplies methods that must never be reached after failure.
	cdn_publish.SessionClient
	// uploads counts attempted pack uploads.
	uploads int
	// roots counts attempted root replacement.
	roots int
	// existing is the cloud's committed pack inventory.
	existing []*packfile.PackfileEntry
	// data and entry retain the attempted immutable pack for the next check.
	data  []byte
	entry *packfile.PackfileEntry
}

// SyncPull returns the cloud pack inventory.
func (c *failingUpload) SyncPull(context.Context, string, string) ([]byte, error) {
	return (&packfile.PullResponse{Entries: c.existing}).MarshalVT()
}

// SyncPushData fails before any release root may advance.
func (c *failingUpload) SyncPushData(_ context.Context, _ string, id string, count int, data, _ []byte, bloom []byte, format uint32) error {
	c.uploads++
	c.data = bytes.Clone(data)
	c.entry = &packfile.PackfileEntry{Id: id, BlockCount: uint64(count), SizeBytes: uint64(len(data)), BloomFilter: bytes.Clone(bloom), BloomFormatVersion: format}
	return errors.New("upload failed")
}

// PostRoot records an unexpected publication after upload failure.
func (c *failingUpload) PostRoot(context.Context, string, *sobject.SORoot, []*sobject.SOOperationRejection) error {
	c.roots++
	return nil
}

// TestPublishPreservesRootAfterUploadFailure exercises a real local release
// World with an executable stored outside its manifest-reference object.
func TestPublishPreservesRootAfterUploadFailure(t *testing.T) {
	// Mount the same plaintext publication format used by Bldr remotes.
	ctx := t.Context()
	w, err := OpenLocalWorld(ctx, logrus.NewEntry(logrus.New()), filepath.Join(t.TempDir(), "release.bdb"), "test-release", "test-release")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Release)
	tx, err := w.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Store a real filesystem manifest and link it from an application key.
	const manifestKey = "test/releases"
	meta := &bldr_manifest.ManifestMeta{ManifestId: "test-desktop", BuildType: "release", PlatformId: "desktop/windows/amd64", Rev: 1}
	ref, err := world.AccessObject(ctx, tx.AccessWorldState, nil, func(cursor *block.Cursor) error {
		return bldr_manifest.CreateManifestWithIoFS(ctx, cursor, bldr_manifest.NewManifest(meta, "app.exe"), fstest.MapFS{"app.exe": {Data: []byte("installed executable"), Mode: 0o755}}, nil, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifest_world.CreateManifestStore(ctx, tx, manifestKey); err != nil {
		t.Fatal(err)
	}
	_, _, err = manifest_world.SetManifest(ctx, tx, "", "test/native", ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.SetGraphQuad(ctx, manifest_world.NewManifestQuad(manifestKey, "test/native", meta.GetManifestId())); err != nil {
		t.Fatal(err)
	}
	metadata, err := StageChannel(ctx, tx, manifestKey, &release.ReleaseMetadata{ProjectId: "test", Version: "0.1.0", ChannelKey: "alpha", Rev: 1})
	if err != nil {
		t.Fatal(err)
	}

	// Staging a second channel must preserve the first channel's metadata ref.
	if _, err := StageChannel(ctx, tx, manifestKey, &release.ReleaseMetadata{ProjectId: "test", Version: "0.2.0", ChannelKey: "beta", Rev: 2}); err != nil {
		t.Fatal(err)
	}
	_, _, err = world.AccessWorldObject(ctx, tx, ChannelDirectoryKey, false, func(cursor *block.Cursor) error {
		directory, err := block.UnmarshalBlock[*release.ChannelDirectory](ctx, cursor, func() block.Block { return &release.ChannelDirectory{} })
		if err == nil && len(directory.GetChannels()) != 2 {
			return errors.New("staging replaced another channel")
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// The selected manifest starts its pack closure, and a retry preserves
	// byte ordering so unchanged content retains its pack identities.
	_, first, err := collectBlocks(ctx, w.Engine, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 || !first[0].ref.EqualVT(metadata.ManifestRefs[0].ManifestRef.RootRef) {
		t.Fatal("release content does not begin with the selected manifest")
	}
	_, repeated, err := collectBlocks(ctx, w.Engine, metadata)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len(repeated) {
		t.Fatal("repeated export changed the block closure")
	}
	for index, entry := range first {
		if !entry.ref.EqualVT(repeated[index].ref) {
			t.Fatalf("repeated export reordered block %d", index)
		}
	}

	// A failed pack write must leave the previously advertised root untouched.
	client := &failingUpload{}
	_, err = Publish(ctx, w.Engine, metadata, cdn_publish.Options{Client: client, DstSpaceID: "test-space", ValidatorKeyPem: "must-not-read.pem", CdnBaseURL: "https://unused.example"})
	if err == nil || client.uploads != 1 || client.roots != 0 {
		t.Fatalf("publication: error=%v uploads=%d roots=%d", err, client.uploads, client.roots)
	}

	// After that pack becomes durable, changing channel metadata must upload
	// only new blocks, even though it changes the release pack boundaries.
	previousData := bytes.Clone(client.data)
	previousEntry := client.entry
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "release.kvf", time.Time{}, bytes.NewReader(previousData))
	}))
	t.Cleanup(server.Close)
	tx, err = w.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	metadata, err = StageChannel(ctx, tx, manifestKey, &release.ReleaseMetadata{ProjectId: "test", Version: "0.1.1", ChannelKey: "alpha", Rev: 3})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	client.existing = []*packfile.PackfileEntry{previousEntry}
	client.uploads = 0
	_, err = Publish(ctx, w.Engine, metadata, cdn_publish.Options{Client: client, DstSpaceID: "test-space", ValidatorKeyPem: "must-not-read.pem", CdnBaseURL: server.URL})
	if err == nil || client.uploads != 1 || client.roots != 0 {
		t.Fatalf("incremental publication: error=%v uploads=%d roots=%d", err, client.uploads, client.roots)
	}
	reader, err := kvfile.BuildReader(bytes.NewReader(client.data), uint64(len(client.data)))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range first {
		_, found, err := reader.Get([]byte(entry.ref.GetHash().MarshalString()))
		if err != nil || found {
			t.Fatalf("republished existing release block: found=%v error=%v", found, err)
		}
	}

	// Missing executable content must fail before even attempting a pack upload.
	client.uploads = 0
	metadata.ManifestRefs[0].ManifestRef.RootRef.Hash.Hash[0] ^= 0xff
	_, err = Publish(ctx, w.Engine, metadata, cdn_publish.Options{Client: client, DstSpaceID: "test-space", ValidatorKeyPem: "must-not-read.pem", CdnBaseURL: "https://unused.example"})
	if err == nil || client.uploads != 0 || client.roots != 0 {
		t.Fatalf("missing content: error=%v uploads=%d roots=%d", err, client.uploads, client.roots)
	}
}
