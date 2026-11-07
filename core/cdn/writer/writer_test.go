package cdn_writer_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
	cdn_writer "github.com/s4wave/spacewave/core/cdn/writer"
	"github.com/s4wave/spacewave/core/cdn/writer/writertest"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	"github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/core/release/publisher"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// testPack is a pack body with its push arguments.
type testPack struct {
	// id names the pack.
	id string
	// data is its body.
	data []byte
	// sum is its SHA-256.
	sum []byte
}

// newPack returns a pack of n random bytes.
func newPack(t *testing.T, n int) testPack {
	t.Helper()
	data := make([]byte, n)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return testPack{id: hex.EncodeToString(sum[:16]), data: data, sum: sum[:]}
}

// push pushes p through c.
func push(ctx context.Context, c *provider_spacewave.SessionClient, p testPack) error {
	return c.SyncPushData(ctx, writertest.Space, p.id, 1, p.data, p.sum, []byte{0xff, 0x01}, 1)
}

// stageRelease stages a release channel in a new local World and returns it
// with its metadata.
func stageRelease(t *testing.T) (*publisher.LocalWorld, *release.ReleaseMetadata) {
	// Open the release World.
	ctx := t.Context()
	w, err := publisher.OpenLocalWorld(ctx, logrus.NewEntry(logrus.New()), filepath.Join(t.TempDir(), "release.s4wave"), "test-release", "test-release")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Release)
	tx, err := w.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Store a manifest under an application key in a manifest store.
	const manifestKey = "test/releases"
	meta := &bldr_manifest.ManifestMeta{ManifestId: "test-desktop", BuildType: "release", PlatformId: "desktop/linux/amd64", Rev: 1}
	ref, err := world.AccessObject(ctx, tx.AccessWorldState, nil, func(cursor *block.Cursor) error {
		return bldr_manifest.CreateManifestWithIoFS(ctx, cursor, bldr_manifest.NewManifest(meta, "app"), fstest.MapFS{"app": {Data: []byte("installed executable"), Mode: 0o755}}, nil, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manifest_world.CreateManifestStore(ctx, tx, manifestKey); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manifest_world.SetManifest(ctx, tx, "", "test/native", ref); err != nil {
		t.Fatal(err)
	}
	if err := tx.SetGraphQuad(ctx, manifest_world.NewManifestQuad(manifestKey, "test/native", meta.GetManifestId())); err != nil {
		t.Fatal(err)
	}

	// Stage the channel and commit.
	metadata, err := publisher.StageChannel(ctx, tx, manifestKey, &release.ReleaseMetadata{ProjectId: "test", Version: "0.1.0", ChannelKey: "alpha", Rev: 1}, writertest.NewKey(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return w, metadata
}

// TestPublishThroughWriter publishes a release World through the writer to
// the bucket, mounts it as a reader does through the pointer handler, and
// keeps the pointer across a restart.
func TestPublishThroughWriter(t *testing.T) {
	// Publish the release through the writer.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	r := writertest.NewRig(t)
	rel, metadata := stageRelease(t)
	if _, err := publisher.Publish(ctx, rel.Engine, metadata, cdn_publish.Options{
		Client:      r.Client(r.Owner),
		DstSpaceID:  writertest.Space,
		OwnerKeyPem: r.OwnerPem,
		CdnBaseURL:  r.PackBase(),
		KeepPacks:   true,
	}); err != nil {
		t.Fatal(err)
	}
	packs := r.Writer.Root(writertest.Space).GetPacks()
	if len(packs) == 0 {
		t.Fatal("the publish listed no pack")
	}
	for _, pack := range packs {
		if keys := r.Bucket.Keys(r.Writer.PackKey(writertest.Space, pack.GetId())); len(keys) != 1 {
			t.Fatalf("pack %s is not in the bucket", pack.GetId())
		}
	}

	// A pull after the first sequence lists only the later packs.
	page, err := r.Client(r.Owner).SyncPull(ctx, writertest.Space, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetEntries()) != len(packs)-1 || page.GetLatestSequence() != uint64(len(packs)) {
		t.Fatalf("pull since 1: %d entries, latest %d, of %d packs", len(page.GetEntries()), page.GetLatestSequence(), len(packs))
	}

	// Mount the writertest.Space as a reader and read the channel directory.
	conf := cdn_world_controller.NewConfig("release", writertest.Space, r.PackBase())
	conf.RootPointerBaseUrl = r.RootsURL
	ctrl := cdn_world_controller.NewController(logrus.NewEntry(logrus.New()), nil, conf)
	done := make(chan error, 1)
	go func() { done <- ctrl.Execute(ctx) }()
	defer func() { cancel(); <-done }()
	eng, err := ctrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	_, _, err = world.AccessWorldObject(ctx, tx, publisher.ChannelDirectoryKey, false, func(cursor *block.Cursor) error {
		dir, err := block.UnmarshalBlock[release.ChannelDirectory](ctx, cursor)
		if err == nil && len(dir.GetChannels()) != 1 {
			t.Errorf("reader sees %d channels", len(dir.GetChannels()))
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// A restarted writer serves the same pointer from its Store.
	before := r.Pointer()
	r.Restart()
	if after := r.Pointer(); after.ETag != before.ETag {
		t.Fatal("restart changed the served pointer")
	}
	if n := len(r.Writer.Root(writertest.Space).GetPacks()); n != len(packs) {
		t.Fatalf("restarted writer lists %d packs, not %d", n, len(packs))
	}
}

// TestRefusals checks each refused write leaves the pointer as it was.
func TestRefusals(t *testing.T) {
	// Push one good pack.
	ctx := t.Context()
	r := writertest.NewRig(t)
	owner := r.Client(r.Owner)
	if err := push(ctx, owner, newPack(t, 512)); err != nil {
		t.Fatal(err)
	}
	before := r.Pointer()

	// A repeated commit of a listed pack succeeds without a change, as a
	// retry whose first answer was lost does.
	listed := r.Writer.Root(writertest.Space).GetPacks()[0].GetId()
	commit, err := http.NewRequestWithContext(ctx, http.MethodPost, r.WriterURL+"/api/bstore/"+writertest.Space+"/sync/push/"+listed+"/commit", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := owner.Do(commit)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || r.Pointer().ETag != before.ETag {
		t.Fatalf("repeated commit: %s", resp.Status)
	}
	unchanged := func(what string) {
		t.Helper()
		if r.Pointer().ETag != before.ETag {
			t.Fatalf("%s changed the pointer", what)
		}
	}

	// A stored pack of another SHA-256 or size is refused.
	p := newPack(t, 512)
	r.Transport.SetPut(func(req *http.Request) (*http.Request, error) {
		return writertest.ReplaceBody(req, bytes.Repeat([]byte{1}, len(p.data))), nil
	})
	if err := push(ctx, owner, p); err == nil || !strings.Contains(err.Error(), cdn_writer.PackMismatchCode) {
		t.Fatalf("SHA-256 mismatch: %v", err)
	}
	unchanged("a SHA-256 mismatch")
	p = newPack(t, 512)
	r.Transport.SetPut(func(req *http.Request) (*http.Request, error) {
		return writertest.ReplaceBody(req, p.data[:100]), nil
	})
	if err := push(ctx, owner, p); err == nil || !strings.Contains(err.Error(), cdn_writer.PackMismatchCode) {
		t.Fatalf("size mismatch: %v", err)
	}
	r.Transport.SetPut(nil)
	unchanged("a size mismatch")

	// A push replacing a pack the Space does not list is refused.
	p = newPack(t, 512)
	err = owner.SyncReplaceData(ctx, writertest.Space, p.id, 1, p.data, p.sum, []byte{1}, 1, []string{"unlisted"})
	if err == nil || !strings.Contains(err.Error(), clouderror.PackReplacementConflictCode) {
		t.Fatalf("replace: %v", err)
	}
	unchanged("a push replacing an unlisted pack")

	// A writer whose admission ended is refused, and one admitted now is not.
	late := writertest.NewKey(t)
	if err := r.Writer.Admit(writertest.Space, writertest.PeerOf(t, late), time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := push(ctx, r.Client(late), newPack(t, 512)); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("expired admission: %v", err)
	}
	unchanged("an expired admission")

	// A stranger's signature and an unsigned request are refused.
	if err := push(ctx, r.Client(writertest.NewKey(t)), newPack(t, 512)); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("stranger: %v", err)
	}
	resp, err = http.Post(r.WriterURL+"/api/bstore/"+writertest.Space+"/sync/push", "application/octet-stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unsigned push: %s", resp.Status)
	}

	// A checkpoint that does not follow the recorded one is refused.
	state := r.Writer.Root(writertest.Space)
	inner, _, err := state.GetCheckpoint().Verify(writertest.Space)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := sobject.BuildSOCheckpoint(r.Owner, &sobject.SOCheckpointInner{
		SharedObjectId:     writertest.Space,
		Height:             inner.GetHeight() + 2,
		PrevCheckpointHash: state.GetCheckpoint().Hash(),
		ConfigHash:         state.GetConfigChainHash(),
		ReplayVersion:      sobject.SOReplayVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PostCheckpoint(ctx, writertest.Space, stale); err == nil || !strings.Contains(err.Error(), cdn_writer.CheckpointRefusedCode) {
		t.Fatalf("stale checkpoint: %v", err)
	}
	unchanged("a stale checkpoint")
}

// TestReplacedPacksRetireAfterGrace supersedes a pack with a replacement,
// keeps its object through the grace period, and has the first commit after
// the grace period delete the object and drop the pack from the pointer.
func TestReplacedPacksRetireAfterGrace(t *testing.T) {
	// Push two packs, then replace the first.
	ctx := t.Context()
	r := writertest.NewRig(t)
	owner := r.Client(r.Owner)
	old, kept, repl := newPack(t, 512), newPack(t, 512), newPack(t, 512)
	for _, p := range []testPack{old, kept} {
		if err := push(ctx, owner, p); err != nil {
			t.Fatal(err)
		}
	}
	if err := owner.SyncReplaceData(ctx, writertest.Space, repl.id, 1, repl.data, repl.sum, []byte{1}, 1, []string{old.id}); err != nil {
		t.Fatal(err)
	}

	// The old pack is superseded but its object stays for older pointers.
	packs := func() map[string]*packfile.PackfileEntry {
		out := make(map[string]*packfile.PackfileEntry)
		for _, pack := range r.Writer.Root(writertest.Space).GetPacks() {
			out[pack.GetId()] = pack
		}
		return out
	}
	if got := packs(); got[old.id].GetSupersededBy() != repl.id || got[kept.id].IsSuperseded() || got[repl.id].IsSuperseded() {
		t.Fatalf("after the replacement: %v", got)
	}
	oldKey := r.Writer.PackKey(writertest.Space, old.id)
	if _, err := r.S3.HeadObject(ctx, s3test.BucketName, oldKey); err != nil {
		t.Fatalf("the superseded pack's object is gone within the grace period: %v", err)
	}

	// A superseded pack is neither replaced again nor pushed again.
	again := newPack(t, 512)
	if err := owner.SyncReplaceData(ctx, writertest.Space, again.id, 1, again.data, again.sum, []byte{1}, 1, []string{old.id}); err == nil || !strings.Contains(err.Error(), clouderror.PackReplacementConflictCode) {
		t.Fatalf("second replacement: %v", err)
	}
	if err := push(ctx, owner, old); err == nil || !strings.Contains(err.Error(), clouderror.PackReplacementConflictCode) {
		t.Fatalf("push of a superseded pack: %v", err)
	}

	// A replacement that borrows a current pack's blocks alone has that
	// pack's ID; it supersedes the others and the borrowed pack stays.
	empty := newPack(t, 512)
	if err := push(ctx, owner, empty); err != nil {
		t.Fatal(err)
	}
	if err := owner.SyncReplaceData(ctx, writertest.Space, kept.id, 1, kept.data, kept.sum, []byte{1}, 1, []string{empty.id, kept.id}); err != nil {
		t.Fatal(err)
	}
	if got := packs(); got[empty.id].GetSupersededBy() != kept.id || got[kept.id].IsSuperseded() {
		t.Fatalf("after the borrowing replacement: %v", got)
	}

	// After the grace period, a restarted writer's next commit deletes the
	// objects and drops the packs.
	r.Grace = time.Millisecond
	r.Restart()
	time.Sleep(2 * time.Millisecond)
	if err := push(ctx, r.Client(r.Owner), newPack(t, 512)); err != nil {
		t.Fatal(err)
	}
	if got := packs(); got[old.id] != nil || got[empty.id] != nil || got[kept.id] == nil || got[repl.id] == nil {
		t.Fatalf("after the grace period: %v", got)
	}
	if _, err := r.S3.HeadObject(ctx, s3test.BucketName, oldKey); err == nil {
		t.Fatal("the retired pack's object remains")
	}
}

// TestConcurrentPushesOverlap holds four uploads until all four are granted,
// so the pushes run together, and checks all four commit.
func TestConcurrentPushesOverlap(t *testing.T) {
	// Hold each upload until four are in flight.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	r := writertest.NewRig(t)
	const n = 4
	var arrived sync.WaitGroup
	arrived.Add(n)
	all := make(chan struct{})
	go func() { arrived.Wait(); close(all) }()
	r.Transport.SetPut(func(req *http.Request) (*http.Request, error) {
		arrived.Done()
		select {
		case <-all:
			return req, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})

	// Push four packs at once.
	owner := r.Client(r.Owner)
	errs := make(chan error, n)
	for range n {
		p := newPack(t, 4096)
		go func() { errs <- push(ctx, owner, p) }()
	}
	for range n {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}

	// Each pack is listed once with its own sequence.
	seen := make(map[uint64]bool)
	for _, pack := range r.Writer.Root(writertest.Space).GetPacks() {
		seen[pack.GetSequence()] = true
	}
	if len(seen) != n {
		t.Fatalf("listed %d sequences, not %d", len(seen), n)
	}
}

// TestCommitCostFlat holds bucket requests and Store reads per commit flat
// between a writertest.Space of n packs and one of 10n.
func TestCommitCostFlat(t *testing.T) {
	// Measure one push's commit at each size.
	type cost struct{ bucket, reads, bytes, mallocs int64 }
	measure := func(packs int) cost {
		// Create a writertest.Space already listing packs entries.
		ctx := t.Context()
		r := writertest.NewRig(t)
		genesis, err := cdn_writer.NewGenesis(fmt.Sprintf("%s-%d", writertest.Space, packs), r.Owner)
		if err != nil {
			t.Fatal(err)
		}
		for i := range packs {
			genesis.Packs = append(genesis.Packs, &packfile.PackfileEntry{
				Id:                 fmt.Sprintf("pack%05d", i),
				Sequence:           uint64(i + 1), //nolint:gosec // a small test count.
				SizeBytes:          1,
				BlockCount:         1,
				BloomFilter:        bytes.Repeat([]byte{0xa5}, 1024),
				BloomFormatVersion: 1,
			})
		}
		if err := r.Writer.Create(ctx, genesis, writertest.PeerOf(t, r.Owner)); err != nil {
			t.Fatal(err)
		}

		// Warm the path with one push, then count the bucket requests, Store
		// reads and allocations of the next.
		c := r.Client(r.Owner)
		pushOne := func() {
			p := newPack(t, 4096)
			if err := c.SyncPushData(ctx, genesis.SpaceId, p.id, 1, p.data, p.sum, []byte{1}, 1); err != nil {
				t.Fatal(err)
			}
		}
		pushOne()
		var before, after runtime.MemStats
		bucketBefore, readsBefore := r.BucketRequests.Load(), r.Store.Roots.Load()
		runtime.GC()
		runtime.ReadMemStats(&before)
		pushOne()
		runtime.ReadMemStats(&after)
		if got := len(r.Writer.Root(genesis.SpaceId).GetPacks()); got != packs+2 {
			t.Fatalf("writertest.Space lists %d packs, not %d", got, packs+2)
		}
		return cost{
			bucket:  r.BucketRequests.Load() - bucketBefore,
			reads:   r.Store.Roots.Load() - readsBefore,
			bytes:   int64(after.TotalAlloc - before.TotalAlloc), //nolint:gosec // a test's allocation fits.
			mallocs: int64(after.Mallocs - before.Mallocs),       //nolint:gosec // a test's allocation fits.
		}
	}
	const n = 100
	small, large := measure(n), measure(10*n)

	// Bucket requests and Store reads stay flat; allocations grow only by a
	// bounded cost per listed entry, the pointer's own rewrite.
	if small.bucket != large.bucket || small.reads != large.reads {
		t.Fatalf("per commit: %d bucket requests and %d Store reads at n, %d and %d at 10n", small.bucket, small.reads, large.bucket, large.reads)
	}
	perEntryBytes := (large.bytes - small.bytes) / (9 * n)
	perEntryMallocs := (large.mallocs - small.mallocs) / (9 * n)
	t.Logf("per commit: %d bucket requests, %d Store reads; %d and %d bytes, %d and %d allocations at n and 10n; %d bytes and %d allocations per listed entry",
		large.bucket, large.reads, small.bytes, large.bytes, small.mallocs, large.mallocs, perEntryBytes, perEntryMallocs)
	if perEntryBytes > 16<<10 || perEntryMallocs > 8 {
		t.Fatalf("a commit allocates %d bytes and %d allocations per listed entry", perEntryBytes, perEntryMallocs)
	}
}
