package bldr_manifest_pack

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestHTTPHandlersServePackRangeHeadAndMetadata(t *testing.T) {
	// Prepare a manifest pack and its HTTP handlers.
	ctx := context.Background()
	meta, packBytes := testManifestPackArtifact(t, ctx)
	handlers, err := NewHTTPHandlers(meta, packBytes)
	if err != nil {
		t.Fatal(err)
	}

	// Request the first four pack bytes through an HTTP range.
	rangeReq := httptest.NewRequest(http.MethodGet, DefaultHTTPPackPath, nil)
	rangeReq.Header.Set("Range", "bytes=0-3")
	rangeRes := httptest.NewRecorder()
	handlers.ServeHTTP(rangeRes, rangeReq)

	// Verify range bytes and cross-origin response metadata.
	if rangeRes.Code != http.StatusPartialContent {
		t.Fatalf("range status = %d", rangeRes.Code)
	}
	if got := rangeRes.Body.Bytes(); !bytes.Equal(got, packBytes[:4]) {
		t.Fatalf("range body = %q want %q", string(got), string(packBytes[:4]))
	}
	if got := rangeRes.Header().Get("Content-Range"); got != "bytes 0-3/"+strconv.Itoa(len(packBytes)) {
		t.Fatalf("content-range = %q", got)
	}
	if got := rangeRes.Header().Get("Cross-Origin-Resource-Policy"); got != "cross-origin" {
		t.Fatalf("corp header = %q", got)
	}
	if got := rangeRes.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("cors origin header = %q", got)
	}

	// Verify the range preflight request succeeds.
	optionsReq := httptest.NewRequest(http.MethodOptions, DefaultHTTPPackPath, nil)
	optionsReq.Header.Set("Access-Control-Request-Headers", "Range")
	optionsRes := httptest.NewRecorder()
	handlers.ServeHTTP(optionsRes, optionsReq)
	if optionsRes.Code != http.StatusNoContent {
		t.Fatalf("options status = %d", optionsRes.Code)
	}

	// Verify HEAD reports the pack length without sending bytes.
	headReq := httptest.NewRequest(http.MethodHead, DefaultHTTPPackPath, nil)
	headRes := httptest.NewRecorder()
	handlers.ServeHTTP(headRes, headReq)
	if headRes.Code != http.StatusOK {
		t.Fatalf("head status = %d", headRes.Code)
	}
	if headRes.Body.Len() != 0 {
		t.Fatalf("HEAD body length = %d", headRes.Body.Len())
	}
	if got := headRes.Header().Get("Content-Length"); got != strconv.Itoa(len(packBytes)) {
		t.Fatalf("content-length = %q", got)
	}

	// Verify metadata identifies the cache schema and producer target.
	metaReq := httptest.NewRequest(http.MethodGet, DefaultHTTPMetadataPath, nil)
	metaRes := httptest.NewRecorder()
	handlers.ServeHTTP(metaRes, metaReq)
	if metaRes.Code != http.StatusOK {
		t.Fatalf("metadata status = %d", metaRes.Code)
	}
	body := metaRes.Body.String()
	if !strings.Contains(body, "\"cacheSchema\":\"manifest-pack-v1\"") {
		t.Fatalf("metadata body missing cache schema: %s", body)
	}
	if !strings.Contains(body, "\"producerTarget\":\"spacewave-web-js\"") {
		t.Fatalf("metadata body missing producer target: %s", body)
	}
}

func TestNewHTTPHandlersFromFS(t *testing.T) {
	// Encode a manifest pack fixture for filesystem-backed handlers.
	ctx := context.Background()
	meta, packBytes := testManifestPackArtifact(t, ctx)
	metaBytes, err := meta.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	// Construct handlers over the fixture filesystem.
	handlers, err := NewHTTPHandlersFromFS(fstest.MapFS{
		ArtifactMetadataFilename: &fstest.MapFile{Data: metaBytes},
		ArtifactPackFilename:     &fstest.MapFile{Data: packBytes},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the filesystem-backed handler serves HEAD.
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodHead, DefaultHTTPPackPath, nil)
	handlers.ServeHTTP(res, req)
	if res.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d", res.Code)
	}
}

func TestNewHTTPPackfileStoreServesManifestBundleBlock(t *testing.T) {
	// Serve a manifest pack through an isolated HTTP server.
	ctx := context.Background()
	meta, packBytes := testManifestPackArtifact(t, ctx)
	handlers, err := NewHTTPHandlers(meta, packBytes)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handlers)
	defer server.Close()

	// Read the manifest bundle root through the HTTP packfile store.
	store, err := NewHTTPPackfileStore(ctx, meta, server.Client(), server.URL+DefaultHTTPPackPath, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := store.GetBlock(ctx, meta.GetManifestBundleRef().GetRootRef())
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("manifest bundle root not found")
	}
	if len(got) == 0 {
		t.Fatal("manifest bundle root block is empty")
	}
}

func testManifestPackArtifact(t *testing.T, ctx context.Context) (*ManifestPackMetadata, []byte) {
	// Store a manifest bundle in the test World.
	t.Helper()
	source := newTestWorld(t, ctx, logrus.NewEntry(logrus.New()))
	sender := peer.ID("sender")
	tuple := testManifestPackTuple()
	manifestRef := storeTestManifest(t, ctx, source, tuple, false, true)
	_, bundleRef, err := StoreManifestBundle(ctx, source, sender, tuple, manifestRef, timestamppb.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Pack the stored bundle and construct its publication metadata.
	var buf bytes.Buffer
	entry, packDigest, err := PackManifestBundle(ctx, source, "ci-release", bundleRef, &buf)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := NewMetadata(
		"0123456789abcdef0123456789abcdef01234567",
		"production",
		"spacewave-web-js",
		false,
		"manifest-pack-v1",
		[]*ManifestTuple{tuple},
		bundleRef,
		entry,
		packDigest,
	)
	if err != nil {
		t.Fatal(err)
	}

	return meta, buf.Bytes()
}
