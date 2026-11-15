package bunlock

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha512"
	"encoding/base64"
	"maps"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestFetchRefusesIntegrityBeforeStorage rejects a large, wrong tarball without
// writing chunks, an integrity key or any underlying storage record.
func TestFetchRefusesIntegrityBeforeStorage(t *testing.T) {
	// Serve a tarball large enough to require block chunking if it were accepted.
	tarball := fixtureTarball(t, fixturePayload(900000))
	registry, counts := fixtureRegistry(t, map[string][]byte{"/bad/-/bad-1.0.0.tgz": tarball})
	fetcher := fixtureFetcher(t, registry)

	// Pin different bytes and fetch into an empty, real block-backed tree.
	storage := store_kvtx_inmem.NewStore()
	store := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), storage, 0)
	dependencies, _ := fixtureDependencies(t, store, nil)
	lock := fixtureLock([]Package{{Key: "bad", Name: "bad", Version: "1.0.0", Integrity: fixtureIntegrity([]byte("different"))}})
	if _, err := fetcher.Fetch(t.Context(), lock, dependencies); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("expected integrity refusal, got %v", err)
	}
	if got := counts(); !maps.Equal(got, map[string]int{"/bad/-/bad-1.0.0.tgz": 1}) {
		t.Fatalf("unexpected requests: %v", got)
	}

	// Check that the pending dependency index contains no refused pin.
	size, err := dependencies.Size(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if size != 0 {
		t.Fatalf("refused tarball created %d integrity keys", size)
	}

	// Inspect the underlying block store independently of the dependency index.
	stored, err := storage.NewTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stored.Discard)
	size, err = stored.Size(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if size != 0 {
		t.Fatalf("refused tarball wrote %d storage records", size)
	}
}

// TestFetchRebuildUsesStoredPins reopens the persisted dependency tree with a new
// fetcher and proves that changing one dependency downloads only its new tarball.
func TestFetchRebuildUsesStoredPins(t *testing.T) {
	// Serve a scoped package and two versions of another package.
	util := fixtureTarball(t, fixturePayload(900000))
	first := fixtureTarball(t, []byte("export const answer = 1"))
	second := fixtureTarball(t, []byte("export const answer = 2"))
	registry, counts := fixtureRegistry(t, map[string][]byte{
		"/@acme/util/-/util-1.0.0.tgz": util,
		"/answer/-/answer-1.0.0.tgz":   first,
		"/answer/-/answer-2.0.0.tgz":   second,
	})
	packages := []Package{
		{Key: "@acme/util", Name: "@acme/util", Version: "1.0.0", Integrity: fixtureIntegrity(util)},
		{Key: "answer", Name: "answer", Version: "1.0.0", Integrity: fixtureIntegrity(first)},
		{Key: "alias/@acme/util", Name: "@acme/util", Version: "1.0.0", Integrity: fixtureIntegrity(util)},
	}

	// Fetch the first build's tarballs through the normal KVTX and block APIs.
	storage := store_kvtx_inmem.NewStore()
	store := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), storage, 0)
	dependencies, root := fixtureDependencies(t, store, nil)
	got, err := fixtureFetcher(t, registry).Fetch(t.Context(), fixtureLock(packages), dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, packages) {
		t.Fatalf("unexpected parsed packages: %v", got)
	}

	// Persist the index and its tarballs before discarding the first cursor graph.
	if err := dependencies.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	ref, _, err := root.GetTransaction().Write(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	if got := counts(); !maps.Equal(got, map[string]int{"/@acme/util/-/util-1.0.0.tgz": 1, "/answer/-/answer-1.0.0.tgz": 1}) {
		t.Fatalf("initial build requests: %v", got)
	}

	// Reopen from addressed blocks and verify a rebuild transfers no tarballs.
	dependencies, root = fixtureDependencies(t, store, ref)
	fetcher := fixtureFetcher(t, registry)
	if _, err := fetcher.Fetch(t.Context(), fixtureLock(packages), dependencies); err != nil {
		t.Fatal(err)
	}
	if got := counts(); !maps.Equal(got, map[string]int{"/@acme/util/-/util-1.0.0.tgz": 1, "/answer/-/answer-1.0.0.tgz": 1}) {
		t.Fatalf("unchanged build fetched packages: %v", got)
	}

	// Change one pin, using its explicit URL as Bun does for a custom registry.
	packages[1].Version = "2.0.0"
	packages[1].URL = registry.URL + "/answer/-/answer-2.0.0.tgz"
	packages[1].Integrity = fixtureIntegrity(second)
	if _, err := fetcher.Fetch(t.Context(), fixtureLock(packages), dependencies); err != nil {
		t.Fatal(err)
	}
	if got := counts(); !maps.Equal(got, map[string]int{
		"/@acme/util/-/util-1.0.0.tgz": 1,
		"/answer/-/answer-1.0.0.tgz":   1,
		"/answer/-/answer-2.0.0.tgz":   1,
	}) {
		t.Fatalf("changed build requests: %v", got)
	}
	if err := dependencies.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	ref, _, err = root.GetTransaction().Write(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}

	// Read every current tarball from a fresh tree, without registry access.
	dependencies, _ = fixtureDependencies(t, store, ref)
	for i, want := range [][]byte{util, second, util} {
		data, found, err := dependencies.Get(t.Context(), []byte(packages[i].Integrity))
		if err != nil {
			t.Fatal(err)
		}
		if !found || !bytes.Equal(data, want) {
			t.Fatalf("stored tarball differs for %s", packages[i].Key)
		}
	}
}

// fixturePayload creates deterministic incompressible bytes for chunked tarballs.
func fixturePayload(size int) []byte {
	// Keep the compressed tarball near the requested size.
	random := rand.New(rand.NewPCG(1, 2))
	payload := make([]byte, size)
	for i := range payload {
		payload[i] = byte(random.Uint32())
	}
	return payload
}

// fixtureTarball creates a deterministic npm archive with one source file.
func fixtureTarball(t *testing.T, source []byte) []byte {
	t.Helper()

	// Wrap one package file in the npm tarball layout.
	var buf bytes.Buffer
	compressed := gzip.NewWriter(&buf)
	archive := tar.NewWriter(compressed)
	if err := archive.WriteHeader(&tar.Header{Name: "package/index.js", Mode: 0o644, Size: int64(len(source))}); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write(source); err != nil {
		t.Fatal(err)
	}

	// Finish both archive layers before computing the lock pin.
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fixtureIntegrity calculates the SRI string for a fixture tarball.
func fixtureIntegrity(data []byte) string {
	sum := sha512.Sum512(data)
	return "sha512-" + base64.StdEncoding.EncodeToString(sum[:])
}

// fixtureLock writes the npm tuples and trailing commas that Bun emits.
func fixtureLock(packages []Package) []byte {
	// Preserve package keys and resolved identities independently.
	var lock strings.Builder
	lock.WriteString(`{"lockfileVersion":1,"packages":{`)
	for _, pkg := range packages {
		lock.WriteString(strconv.Quote(pkg.Key) + ":[" + strconv.Quote(pkg.Name+"@"+pkg.Version) + "," + strconv.Quote(pkg.URL) + `,{},` + strconv.Quote(pkg.Integrity) + "],")
	}
	lock.WriteString("}}")
	return []byte(lock.String())
}

// fixtureRegistry serves only local fixture tarballs and counts every request.
func fixtureRegistry(t *testing.T, tarballs map[string][]byte) (*httptest.Server, func() map[string]int) {
	t.Helper()

	// Protect request counts while the local registry serves HTTP requests.
	var mtx sync.Mutex
	counts := make(map[string]int)
	registry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Count even unknown paths so a wrong URL cannot pass unnoticed.
		mtx.Lock()
		counts[req.URL.Path]++
		mtx.Unlock()

		// Serve only bytes included in the fixture.
		data, found := tarballs[req.URL.Path]
		if !found {
			http.NotFound(w, req)
			return
		}
		if _, err := w.Write(data); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(registry.Close)
	return registry, func() map[string]int {
		mtx.Lock()
		defer mtx.Unlock()
		return maps.Clone(counts)
	}
}

// fixtureFetcher bounds each local fixture download to two MiB.
func fixtureFetcher(t *testing.T, registry *httptest.Server) *Fetcher {
	t.Helper()

	// Use the fixture server's transport and an explicit default registry.
	fetcher, err := NewFetcher(registry.Client(), registry.URL, 2<<20)
	if err != nil {
		t.Fatal(err)
	}
	return fetcher
}

// fixtureDependencies opens a real block-backed tree at ref, or an empty one.
func fixtureDependencies(t *testing.T, store block.StoreOps, ref *block.BlockRef) (kvtx.BlockTx, *block.Cursor) {
	t.Helper()

	// Create a new cursor graph so a rebuild cannot rely on the old in-memory tree.
	_, root := block.NewTransaction(store, nil, ref, nil)
	if ref == nil {
		root.SetBlock(kvtx_block.NewKeyValueStore(0), true)
	}
	dependencies, err := kvtx_block.BuildKvTransaction(t.Context(), root, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dependencies.Discard)
	return dependencies, root
}
