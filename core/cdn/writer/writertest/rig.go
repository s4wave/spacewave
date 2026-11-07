// Package writertest runs a CDN Space writer for tests: an in-process bucket
// behind a counting proxy, a pointer handler, and a writer over a file-backed
// World, each served on loopback, with a new Space owned by a new key.
package writertest

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/cdn"
	cdn_rootserve "github.com/s4wave/spacewave/core/cdn/rootserve"
	cdn_writer "github.com/s4wave/spacewave/core/cdn/writer"
	cdn_writer_world "github.com/s4wave/spacewave/core/cdn/writer/world"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/release/publisher"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Env is the signing environment the tests' writers take.
const Env = "spacewave-cdn-writer-test"

// Space is the CDN Space the tests publish.
const Space = "cdn-writer-test-space"

// Rig is a writer over a file-backed World, an in-process bucket and a
// pointer handler, each served on loopback.
type Rig struct {
	// t is the test.
	t *testing.T
	// Dir holds the writer's World.
	Dir string
	// Bucket is the in-process bucket.
	Bucket *s3test.Bucket
	// S3 reaches the bucket through the counting proxy.
	S3 *block_store_s3.Client
	// BucketRequests counts requests reaching the bucket from the writer.
	BucketRequests atomic.Int64
	// Roots serves the Space's pointer.
	Roots *cdn_rootserve.Handler
	// RootsURL is where roots serves.
	RootsURL string
	// World is the writer's open World.
	World *publisher.LocalWorld
	// Store counts the writer's Store calls.
	Store *CountingStore
	// Writer is the writer under test.
	Writer *cdn_writer.Writer
	// WriterURL is where the writer serves.
	WriterURL string
	// Owner signs checkpoints and requests.
	Owner crypto.PrivKey
	// OwnerPem is the path of owner's PEM file.
	OwnerPem string
	// Transport carries every client request.
	Transport *HookTransport
	// Grace is the writer's superseded pack grace period from the next
	// Start, or zero for the default.
	Grace time.Duration
}

// NewRig starts a rig with a new Space owned by a new key.
func NewRig(t *testing.T) *Rig {
	// Serve the bucket behind a counting proxy and the pointers on their own
	// host.
	t.Helper()
	r := &Rig{t: t, Dir: t.TempDir(), Transport: &HookTransport{}}
	r.Bucket, _ = s3test.New(t)
	target, err := url.Parse(r.Bucket.URL())
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.BucketRequests.Add(1)
		proxy.ServeHTTP(w, req)
	}))
	t.Cleanup(proxySrv.Close)

	// Reach the bucket through the proxy, and serve pointers on their own host.
	r.S3, err = block_store_s3.BuildClient(&block_store_s3.ClientConfig{
		Endpoint:    strings.TrimPrefix(proxySrv.URL, "http://"),
		DisableSsl:  true,
		Credentials: &block_store_s3.Credentials{AccessKeyId: "key", SecretAccessKey: "secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	r.Roots = cdn_rootserve.NewHandler("", nil)
	rootsSrv := httptest.NewServer(r.Roots)
	t.Cleanup(rootsSrv.Close)
	r.RootsURL = rootsSrv.URL

	// Write the owner key where the publisher reads it.
	r.Owner = NewKey(t)
	pem, err := keypem.MarshalPrivKeyPem(r.Owner)
	if err != nil {
		t.Fatal(err)
	}
	r.OwnerPem = filepath.Join(r.Dir, "owner.pem")
	if err := os.WriteFile(r.OwnerPem, pem, 0o600); err != nil {
		t.Fatal(err)
	}

	// Start the writer and create the Space.
	r.Start()
	genesis, err := cdn_writer.NewGenesis(Space, r.Owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Writer.Create(t.Context(), genesis, PeerOf(t, r.Owner)); err != nil {
		t.Fatal(err)
	}
	return r
}

// Start opens the World and serves a writer over it.
func (r *Rig) Start() {
	// Open the World the writer's Store keeps.
	t := r.t
	w, err := publisher.OpenLocalWorld(t.Context(), logrus.NewEntry(logrus.New()), filepath.Join(r.Dir, "writer.s4wave"), "cdn-writer", "cdn-writer")
	if err != nil {
		t.Fatal(err)
	}
	r.World = w
	t.Cleanup(r.Stop)

	// Serve the writer.
	r.Store = &CountingStore{Store: cdn_writer_world.NewStore(w.Engine, PeerOf(t, r.Owner))}
	r.Writer, err = cdn_writer.New(cdn_writer.Config{
		Store:           r.Store,
		Bucket:          r.S3,
		BucketName:      s3test.BucketName,
		Roots:           r.Roots,
		SigningEnv:      Env,
		SupersededGrace: r.Grace,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(r.Writer)
	t.Cleanup(srv.Close)
	r.WriterURL = srv.URL
}

// Stop closes the writer's World once.
func (r *Rig) Stop() {
	if r.World != nil {
		r.World.Release()
		r.World = nil
	}
}

// Restart stops the writer and starts a new one over the same World and
// bucket, owning the Space again.
func (r *Rig) Restart() {
	r.Stop()
	r.Start()
	if err := r.Writer.Own(r.t.Context(), Space, PeerOf(r.t, r.Owner)); err != nil {
		r.t.Fatal(err)
	}
}

// Client returns a session client signing as key.
func (r *Rig) Client(key crypto.PrivKey) *provider_spacewave.SessionClient {
	return provider_spacewave.NewSessionClient(&http.Client{Transport: r.Transport}, r.WriterURL, Env, key, PeerOf(r.t, key).String())
}

// PackBase is where readers fetch the Space's packs.
func (r *Rig) PackBase() string {
	return r.Bucket.URL() + "/" + s3test.BucketName
}

// Pointer returns the pointer the handler serves.
func (r *Rig) Pointer() cdn_rootserve.Pointer {
	ptr, err := r.Roots.Pointer(r.t.Context(), Space)
	if err != nil {
		r.t.Fatal(err)
	}
	return ptr
}

// CountingStore counts a Store's calls.
type CountingStore struct {
	cdn_writer.Store
	// Roots, Imports and Commits count each call.
	Roots, Imports, Commits atomic.Int64
}

// Root counts and forwards.
func (s *CountingStore) Root(ctx context.Context, spaceID string) (*cdn.CdnRootPointer, error) {
	s.Roots.Add(1)
	return s.Store.Root(ctx, spaceID)
}

// Import counts and forwards.
func (s *CountingStore) Import(ctx context.Context, root *cdn.CdnRootPointer) (*cdn.CdnRootPointer, error) {
	s.Imports.Add(1)
	return s.Store.Import(ctx, root)
}

// Commit counts and forwards.
func (s *CountingStore) Commit(ctx context.Context, c *cdn_writer.Commit) (*cdn.CdnRootPointer, error) {
	s.Commits.Add(1)
	return s.Store.Commit(ctx, c)
}

// HookTransport lets a test change or hold a client's uploads.
type HookTransport struct {
	// mtx guards put.
	mtx sync.Mutex
	// put, when set, replaces each PUT before it is sent.
	put func(*http.Request) (*http.Request, error)
}

// SetPut sets the PUT hook.
func (h *HookTransport) SetPut(put func(*http.Request) (*http.Request, error)) {
	h.mtx.Lock()
	defer h.mtx.Unlock()
	h.put = put
}

// RoundTrip applies the PUT hook and sends the request.
func (h *HookTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Read the hook, and apply it to an upload.
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

// ReplaceBody returns req sending data instead of its body.
func ReplaceBody(req *http.Request, data []byte) *http.Request {
	// Clone the request with the new body and its length.
	req = req.Clone(req.Context())
	req.Body = io.NopCloser(bytes.NewReader(data))
	req.ContentLength = int64(len(data))
	req.GetBody = nil
	return req
}

// NewKey returns a new Ed25519 key.
func NewKey(t *testing.T) crypto.PrivKey {
	t.Helper()
	key, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

// PeerOf returns key's peer ID.
func PeerOf(t *testing.T, key crypto.PrivKey) peer.ID {
	t.Helper()
	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
