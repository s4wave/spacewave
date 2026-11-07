package cdn_writer_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
	cdn_rootserve "github.com/s4wave/spacewave/core/cdn/rootserve"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
	cdn_writer "github.com/s4wave/spacewave/core/cdn/writer"
	cdn_writer_world "github.com/s4wave/spacewave/core/cdn/writer/world"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
	"github.com/s4wave/spacewave/core/release"
	"github.com/s4wave/spacewave/core/release/publisher"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// testEnv is the signing environment the tests' writers take.
const testEnv = "spacewave-cdn-writer-test"

// testSpace is the CDN Space the tests publish.
const testSpace = "cdn-writer-test-space"

// rig is a writer over a file-backed World, an in-process bucket and a
// pointer handler, each served on loopback.
type rig struct {
	// t is the test.
	t *testing.T
	// dir holds the writer's World.
	dir string
	// bucket is the in-process bucket.
	bucket *s3test.Bucket
	// s3 reaches the bucket through the counting proxy.
	s3 *block_store_s3.Client
	// bucketRequests counts requests reaching the bucket from the writer.
	bucketRequests atomic.Int64
	// roots serves the Space's pointer.
	roots *cdn_rootserve.Handler
	// rootsURL is where roots serves.
	rootsURL string
	// world is the writer's open World.
	world *publisher.LocalWorld
	// store counts the writer's Store calls.
	store *countingStore
	// writer is the writer under test.
	writer *cdn_writer.Writer
	// writerURL is where the writer serves.
	writerURL string
	// owner signs checkpoints and requests.
	owner crypto.PrivKey
	// ownerPem is the path of owner's PEM file.
	ownerPem string
	// transport carries every client request.
	transport *hookTransport
}

// newRig starts a rig with a new Space owned by a new key.
func newRig(t *testing.T) *rig {
	// Serve the bucket behind a counting proxy and the pointers on their own
	// host.
	t.Helper()
	r := &rig{t: t, dir: t.TempDir(), transport: &hookTransport{}}
	r.bucket, _ = s3test.New(t)
	target, err := url.Parse(r.bucket.URL())
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.bucketRequests.Add(1)
		proxy.ServeHTTP(w, req)
	}))
	t.Cleanup(proxySrv.Close)
	r.s3, err = block_store_s3.BuildClient(&block_store_s3.ClientConfig{
		Endpoint:    strings.TrimPrefix(proxySrv.URL, "http://"),
		DisableSsl:  true,
		Credentials: &block_store_s3.Credentials{AccessKeyId: "key", SecretAccessKey: "secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	r.roots = cdn_rootserve.NewHandler("", nil)
	rootsSrv := httptest.NewServer(r.roots)
	t.Cleanup(rootsSrv.Close)
	r.rootsURL = rootsSrv.URL

	// Write the owner key where the publisher reads it.
	r.owner = newKey(t)
	pem, err := keypem.MarshalPrivKeyPem(r.owner)
	if err != nil {
		t.Fatal(err)
	}
	r.ownerPem = filepath.Join(r.dir, "owner.pem")
	if err := os.WriteFile(r.ownerPem, pem, 0o600); err != nil {
		t.Fatal(err)
	}

	// Start the writer and create the Space.
	r.start()
	genesis, err := cdn_writer.NewGenesis(testSpace, r.owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.writer.Create(t.Context(), genesis, peerOf(t, r.owner)); err != nil {
		t.Fatal(err)
	}
	return r
}

// start opens the World and serves a writer over it.
func (r *rig) start() {
	// Open the World the writer's Store keeps.
	t := r.t
	w, err := publisher.OpenLocalWorld(t.Context(), logrus.NewEntry(logrus.New()), filepath.Join(r.dir, "writer.s4wave"), "cdn-writer", "cdn-writer")
	if err != nil {
		t.Fatal(err)
	}
	r.world = w
	t.Cleanup(r.stop)

	// Serve the writer.
	r.store = &countingStore{Store: cdn_writer_world.NewStore(w.Engine, peerOf(t, r.owner))}
	r.writer, err = cdn_writer.New(cdn_writer.Config{
		Store:      r.store,
		Bucket:     r.s3,
		BucketName: s3test.BucketName,
		Roots:      r.roots,
		SigningEnv: testEnv,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(r.writer)
	t.Cleanup(srv.Close)
	r.writerURL = srv.URL
}

// stop closes the writer's World once.
func (r *rig) stop() {
	if r.world != nil {
		r.world.Release()
		r.world = nil
	}
}

// restart stops the writer and starts a new one over the same World and
// bucket, owning the Space again.
func (r *rig) restart() {
	r.stop()
	r.start()
	if err := r.writer.Own(r.t.Context(), testSpace, peerOf(r.t, r.owner)); err != nil {
		r.t.Fatal(err)
	}
}

// client returns a session client signing as key.
func (r *rig) client(key crypto.PrivKey) *provider_spacewave.SessionClient {
	return provider_spacewave.NewSessionClient(&http.Client{Transport: r.transport}, r.writerURL, testEnv, key, peerOf(r.t, key).String())
}

// packBase is where readers fetch the Space's packs.
func (r *rig) packBase() string {
	return r.bucket.URL() + "/" + s3test.BucketName
}

// pointer returns the pointer the handler serves.
func (r *rig) pointer() cdn_rootserve.Pointer {
	ptr, err := r.roots.Pointer(r.t.Context(), testSpace)
	if err != nil {
		r.t.Fatal(err)
	}
	return ptr
}

// countingStore counts a Store's calls.
type countingStore struct {
	cdn_writer.Store
	// roots, imports and commits count each call.
	roots, imports, commits atomic.Int64
}

// Root counts and forwards.
func (s *countingStore) Root(ctx context.Context, spaceID string) (*cdn.CdnRootPointer, error) {
	s.roots.Add(1)
	return s.Store.Root(ctx, spaceID)
}

// Import counts and forwards.
func (s *countingStore) Import(ctx context.Context, root *cdn.CdnRootPointer) (*cdn.CdnRootPointer, error) {
	s.imports.Add(1)
	return s.Store.Import(ctx, root)
}

// Commit counts and forwards.
func (s *countingStore) Commit(ctx context.Context, c *cdn_writer.Commit) (*cdn.CdnRootPointer, error) {
	s.commits.Add(1)
	return s.Store.Commit(ctx, c)
}

// hookTransport lets a test change or hold a client's uploads.
type hookTransport struct {
	// mtx guards put.
	mtx sync.Mutex
	// put, when set, replaces each PUT before it is sent.
	put func(*http.Request) (*http.Request, error)
}

// setPut sets the PUT hook.
func (h *hookTransport) setPut(put func(*http.Request) (*http.Request, error)) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	h.put = put
}

// RoundTrip applies the PUT hook and sends the request.
func (h *hookTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	h.mtx.Lock()
	put := h.put
	h.mtx.Unlock()
	if put != nil && req.Method == http.MethodPut {
		var err error
		if req, err = put(req); err != nil {
			return nil, err
		}
	}
	return http.DefaultTransport.RoundTrip(req)
}

// replaceBody returns req sending data instead of its body.
func replaceBody(req *http.Request, data []byte) *http.Request {
	req = req.Clone(req.Context())
	req.Body = io.NopCloser(bytes.NewReader(data))
	req.ContentLength = int64(len(data))
	req.GetBody = nil
	return req
}

// newKey returns a new Ed25519 key.
func newKey(t *testing.T) crypto.PrivKey {
	t.Helper()
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// peerOf returns key's peer ID.
func peerOf(t *testing.T, key crypto.PrivKey) peer.ID {
	t.Helper()
	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

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
	return c.SyncPushData(ctx, testSpace, p.id, 1, p.data, p.sum, []byte{0xff, 0x01}, 1)
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
	metadata, err := publisher.StageChannel(ctx, tx, manifestKey, &release.ReleaseMetadata{ProjectId: "test", Version: "0.1.0", ChannelKey: "alpha", Rev: 1}, newKey(t))
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
	r := newRig(t)
	rel, metadata := stageRelease(t)
	if _, err := publisher.Publish(ctx, rel.Engine, metadata, cdn_publish.Options{
		Client:      r.client(r.owner),
		DstSpaceID:  testSpace,
		OwnerKeyPem: r.ownerPem,
		CdnBaseURL:  r.packBase(),
		KeepPacks:   true,
	}); err != nil {
		t.Fatal(err)
	}
	packs := r.writer.Root(testSpace).GetPacks()
	if len(packs) == 0 {
		t.Fatal("the publish listed no pack")
	}
	for _, pack := range packs {
		if keys := r.bucket.Keys(r.writer.PackKey(testSpace, pack.GetId())); len(keys) != 1 {
			t.Fatalf("pack %s is not in the bucket", pack.GetId())
		}
	}

	// A pull after the first sequence lists only the later packs.
	page, err := r.client(r.owner).SyncPull(ctx, testSpace, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetEntries()) != len(packs)-1 || page.GetLatestSequence() != uint64(len(packs)) {
		t.Fatalf("pull since 1: %d entries, latest %d, of %d packs", len(page.GetEntries()), page.GetLatestSequence(), len(packs))
	}

	// Mount the Space as a reader and read the channel directory.
	conf := cdn_world_controller.NewConfig("release", testSpace, r.packBase())
	conf.RootPointerBaseUrl = r.rootsURL
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
	before := r.pointer()
	r.restart()
	if after := r.pointer(); after.ETag != before.ETag {
		t.Fatal("restart changed the served pointer")
	}
	if n := len(r.writer.Root(testSpace).GetPacks()); n != len(packs) {
		t.Fatalf("restarted writer lists %d packs, not %d", n, len(packs))
	}
}

// TestRefusals checks each refused write leaves the pointer as it was.
func TestRefusals(t *testing.T) {
	// Push one good pack.
	ctx := t.Context()
	r := newRig(t)
	owner := r.client(r.owner)
	if err := push(ctx, owner, newPack(t, 512)); err != nil {
		t.Fatal(err)
	}
	before := r.pointer()

	// A repeated commit of a listed pack succeeds without a change, as a
	// retry whose first answer was lost does.
	listed := r.writer.Root(testSpace).GetPacks()[0].GetId()
	commit, err := http.NewRequestWithContext(ctx, http.MethodPost, r.writerURL+"/api/bstore/"+testSpace+"/sync/push/"+listed+"/commit", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := owner.Do(commit)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || r.pointer().ETag != before.ETag {
		t.Fatalf("repeated commit: %s", resp.Status)
	}
	unchanged := func(what string) {
		t.Helper()
		if r.pointer().ETag != before.ETag {
			t.Fatalf("%s changed the pointer", what)
		}
	}

	// A stored pack of another SHA-256 or size is refused.
	p := newPack(t, 512)
	r.transport.setPut(func(req *http.Request) (*http.Request, error) {
		return replaceBody(req, bytes.Repeat([]byte{1}, len(p.data))), nil
	})
	if err := push(ctx, owner, p); err == nil || !strings.Contains(err.Error(), cdn_writer.PackMismatchCode) {
		t.Fatalf("SHA-256 mismatch: %v", err)
	}
	unchanged("a SHA-256 mismatch")
	p = newPack(t, 512)
	r.transport.setPut(func(req *http.Request) (*http.Request, error) {
		return replaceBody(req, p.data[:100]), nil
	})
	if err := push(ctx, owner, p); err == nil || !strings.Contains(err.Error(), cdn_writer.PackMismatchCode) {
		t.Fatalf("size mismatch: %v", err)
	}
	r.transport.setPut(nil)
	unchanged("a size mismatch")

	// A replacing push is refused.
	p = newPack(t, 512)
	err = owner.SyncReplaceData(ctx, testSpace, p.id, 1, p.data, p.sum, []byte{1}, 1, []string{r.writer.Root(testSpace).GetPacks()[0].GetId()})
	if err == nil || !strings.Contains(err.Error(), clouderror.PackReplacementConflictCode) {
		t.Fatalf("replace: %v", err)
	}
	unchanged("a replacing push")

	// A writer whose admission ended is refused, and one admitted now is not.
	late := newKey(t)
	if err := r.writer.Admit(testSpace, peerOf(t, late), time.Now().Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := push(ctx, r.client(late), newPack(t, 512)); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("expired admission: %v", err)
	}
	unchanged("an expired admission")

	// A stranger's signature and an unsigned request are refused.
	if err := push(ctx, r.client(newKey(t)), newPack(t, 512)); err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("stranger: %v", err)
	}
	resp, err = http.Post(r.writerURL+"/api/bstore/"+testSpace+"/sync/push", "application/octet-stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unsigned push: %s", resp.Status)
	}

	// A checkpoint that does not follow the recorded one is refused.
	state := r.writer.Root(testSpace)
	inner, _, err := state.GetCheckpoint().Verify(testSpace)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := sobject.BuildSOCheckpoint(r.owner, &sobject.SOCheckpointInner{
		SharedObjectId:     testSpace,
		Height:             inner.GetHeight() + 2,
		PrevCheckpointHash: state.GetCheckpoint().Hash(),
		ConfigHash:         state.GetConfigChainHash(),
		ReplayVersion:      sobject.SOReplayVersion,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.PostCheckpoint(ctx, testSpace, stale); err == nil || !strings.Contains(err.Error(), cdn_writer.CheckpointRefusedCode) {
		t.Fatalf("stale checkpoint: %v", err)
	}
	unchanged("a stale checkpoint")
}

// TestConcurrentPushesOverlap holds four uploads until all four are granted,
// so the pushes run together, and checks all four commit.
func TestConcurrentPushesOverlap(t *testing.T) {
	// Hold each upload until four are in flight.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	r := newRig(t)
	const n = 4
	var arrived sync.WaitGroup
	arrived.Add(n)
	all := make(chan struct{})
	go func() { arrived.Wait(); close(all) }()
	r.transport.setPut(func(req *http.Request) (*http.Request, error) {
		arrived.Done()
		select {
		case <-all:
			return req, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})

	// Push four packs at once.
	owner := r.client(r.owner)
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
	for _, pack := range r.writer.Root(testSpace).GetPacks() {
		seen[pack.GetSequence()] = true
	}
	if len(seen) != n {
		t.Fatalf("listed %d sequences, not %d", len(seen), n)
	}
}

// TestCommitCostFlat holds bucket requests and Store reads per commit flat
// between a Space of n packs and one of 10n.
func TestCommitCostFlat(t *testing.T) {
	// Measure one push's commit at each size.
	type cost struct{ bucket, reads, bytes, mallocs int64 }
	measure := func(packs int) cost {
		// Create a Space already listing packs entries.
		ctx := t.Context()
		r := newRig(t)
		genesis, err := cdn_writer.NewGenesis(fmt.Sprintf("%s-%d", testSpace, packs), r.owner)
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
		if err := r.writer.Create(ctx, genesis, peerOf(t, r.owner)); err != nil {
			t.Fatal(err)
		}

		// Warm the path with one push, then count the bucket requests, Store
		// reads and allocations of the next.
		c := r.client(r.owner)
		pushOne := func() {
			p := newPack(t, 4096)
			if err := c.SyncPushData(ctx, genesis.SpaceId, p.id, 1, p.data, p.sum, []byte{1}, 1); err != nil {
				t.Fatal(err)
			}
		}
		pushOne()
		var before, after runtime.MemStats
		bucketBefore, readsBefore := r.bucketRequests.Load(), r.store.roots.Load()
		runtime.GC()
		runtime.ReadMemStats(&before)
		pushOne()
		runtime.ReadMemStats(&after)
		if got := len(r.writer.Root(genesis.SpaceId).GetPacks()); got != packs+2 {
			t.Fatalf("Space lists %d packs, not %d", got, packs+2)
		}
		return cost{
			bucket:  r.bucketRequests.Load() - bucketBefore,
			reads:   r.store.roots.Load() - readsBefore,
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
