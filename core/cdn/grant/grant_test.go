package cdn_grant_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/fastjson"
	"github.com/s4wave/spacewave/core/cdn"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	cdn_grant "github.com/s4wave/spacewave/core/cdn/grant"
	cdn_mirror "github.com/s4wave/spacewave/core/cdn/mirror"
	cdn_publish "github.com/s4wave/spacewave/core/cdn/publish"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
	"github.com/s4wave/spacewave/core/cdn/writer/writertest"
	"github.com/s4wave/spacewave/core/release/publisher"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/sirupsen/logrus"
)

// fsKey is the World object the published tree sits at.
const fsKey = "media"

// fakeB2 serves the B2 native API calls an authorizer makes and downloads
// by name over the in-process bucket, each refused unless its token covers
// the file's prefix and has not expired.
type fakeB2 struct {
	// bucketURL is where the bucket serves its objects, path style.
	bucketURL string
	// srv serves the API and the downloads.
	srv *httptest.Server

	mtx sync.Mutex
	// tokens maps each download token to the prefix it covers and its end.
	tokens map[string]b2Token
	// next numbers the tokens.
	next int

	// served and refused count downloads.
	served, refused atomic.Int64
}

// b2Token is one download authorization.
type b2Token struct {
	prefix  string
	expires time.Time
}

const (
	b2Key, b2Secret = "key-id", "app-key"
	b2Account       = "account-token"
	b2BucketID      = "bucket-id"
)

// newFakeB2 serves a fake B2 over r's bucket.
func newFakeB2(t *testing.T, r *writertest.Rig) *fakeB2 {
	f := &fakeB2{bucketURL: r.Bucket.URL() + "/" + s3test.BucketName, tokens: map[string]b2Token{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /b2api/v3/b2_authorize_account", f.authorizeAccount)
	mux.HandleFunc("POST /b2api/v3/b2_get_download_authorization", f.downloadAuthorization)
	mux.HandleFunc("GET /file/"+s3test.BucketName+"/", f.download)
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

// authorizeAccount answers the application key with the account token.
func (f *fakeB2) authorizeAccount(w http.ResponseWriter, req *http.Request) {
	if id, secret, ok := req.BasicAuth(); !ok || id != b2Key || secret != b2Secret {
		http.Error(w, "bad key", http.StatusUnauthorized)
		return
	}
	writeJSON(w, `{"authorizationToken":`+strconv.Quote(b2Account)+`,"apiInfo":{"storageApi":{"apiUrl":`+strconv.Quote(f.srv.URL)+`,"downloadUrl":`+strconv.Quote(f.srv.URL)+`}}}`)
}

// downloadAuthorization mints a token for the requested prefix.
func (f *fakeB2) downloadAuthorization(w http.ResponseWriter, req *http.Request) {
	if req.Header.Get("Authorization") != b2Account {
		http.Error(w, "bad account token", http.StatusUnauthorized)
		return
	}
	data, _ := io.ReadAll(req.Body)
	body, err := fastjson.ParseBytes(data)
	if err != nil || string(body.GetStringBytes("bucketId")) != b2BucketID || body.GetInt64("validDurationInSeconds") < 1 {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	prefix, seconds := string(body.GetStringBytes("fileNamePrefix")), body.GetInt64("validDurationInSeconds")
	f.mtx.Lock()
	f.next++
	tok := fmt.Sprintf("download-%d", f.next)
	f.tokens[tok] = b2Token{prefix: prefix, expires: time.Now().Add(time.Duration(seconds) * time.Second)}
	f.mtx.Unlock()
	writeJSON(w, `{"authorizationToken":`+strconv.Quote(tok)+`}`)
}

// download serves a file whose name the request's token covers.
func (f *fakeB2) download(w http.ResponseWriter, req *http.Request) {
	// Refuse a token that is unknown, expired, or for another prefix.
	name := strings.TrimPrefix(req.URL.Path, "/file/"+s3test.BucketName+"/")
	f.mtx.Lock()
	tok, ok := f.tokens[req.Header.Get("Authorization")]
	f.mtx.Unlock()
	if !ok || !time.Now().Before(tok.expires) || !strings.HasPrefix(name, tok.prefix) {
		f.refused.Add(1)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Serve the range from the bucket.
	f.served.Add(1)
	up, err := http.NewRequestWithContext(req.Context(), http.MethodGet, f.bucketURL+"/"+name, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if rng := req.Header.Get("Range"); rng != "" {
		up.Header.Set("Range", rng)
	}
	resp, err := http.DefaultClient.Do(up)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for _, h := range []string{"Content-Length", "Content-Range", "Content-Type"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

// writeJSON answers the JSON body.
func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

// fixture is a writer issuing grants through a fake B2.
type fixture struct {
	rig    *writertest.Rig
	b2     *fakeB2
	issuer *cdn_grant.Issuer
}

// newFixture starts a writer and an issuer over a fake B2.
func newFixture(t *testing.T) *fixture {
	r := writertest.NewRig(t)
	f := newFakeB2(t, r)
	auth, err := cdn_grant.NewB2(cdn_grant.B2Config{
		KeyID: b2Key, ApplicationKey: b2Secret,
		BucketID: b2BucketID, BucketName: s3test.BucketName,
		APIURL: f.srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{rig: r, b2: f, issuer: &cdn_grant.Issuer{Writer: r.Writer, Downloads: auth}}
}

// holder is a client holding grants for the Space.
type holder struct {
	key    crypto.PrivKey
	pem    string
	grants cdn_bstore.GrantSourceFunc
	issued atomic.Int64
}

// newHolder returns a client whose grants the issuer mints, writing when
// mayWrite is set.
func (fx *fixture) newHolder(t *testing.T, mayWrite bool) *holder {
	h := &holder{key: writertest.NewKey(t)}
	data, err := keypem.MarshalPrivKeyPem(h.key)
	if err != nil {
		t.Fatal(err)
	}
	h.pem = filepath.Join(t.TempDir(), "holder.pem")
	if err := os.WriteFile(h.pem, data, 0o600); err != nil {
		t.Fatal(err)
	}
	pid := writertest.PeerOf(t, h.key)
	h.grants = func(ctx context.Context, spaceID string) (*cdn.CdnSpaceGrant, error) {
		h.issued.Add(1)
		return fx.issuer.Issue(ctx, spaceID, pid, mayWrite)
	}
	return h
}

// publish mirrors tree into the Space as h.
func (fx *fixture) publish(ctx context.Context, h *holder, tree *bucket.ObjectRef, src block.StoreOps) error {
	_, err := cdn_mirror.Mirror(ctx, src, tree, cdn_mirror.Options{
		Publish: cdn_publish.Options{
			Client:      fx.rig.Client(h.key),
			DstSpaceID:  writertest.Space,
			OwnerKeyPem: h.pem,
			CdnBaseURL:  fx.b2.srv.URL,
		},
		Grants: h.grants,
	})
	return err
}

// TestGrantReadsAndWritesPrivateSpace issues a writing grant, publishes a
// tree with it, mounts the Space from grants on the bus and reads the tree
// back through the download front, which refuses any read its token does not
// cover.
func TestGrantReadsAndWritesPrivateSpace(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	fx := newFixture(t)
	files, src, tree := newTree(t, 12)

	// A read-only holder may not publish.
	reader := fx.newHolder(t, false)
	if err := fx.publish(ctx, reader, tree, src); err == nil || !strings.Contains(err.Error(), "does not allow publishing") {
		t.Fatalf("publish without write: %v", err)
	}

	// A writing holder publishes, reading the Space's indexes through its
	// grant.
	h := fx.newHolder(t, true)
	if err := fx.publish(ctx, h, tree, src); err != nil {
		t.Fatal(err)
	}
	if fx.b2.refused.Load() != 0 {
		t.Fatalf("%d granted reads refused", fx.b2.refused.Load())
	}

	// The holder mounts the private Space and reads its own commit.
	readTree(ctx, t, fx, h, files)
	if fx.b2.served.Load() == 0 || fx.b2.refused.Load() != 0 {
		t.Fatalf("served %d, refused %d", fx.b2.served.Load(), fx.b2.refused.Load())
	}

	// A read outside the Space's prefix, or with no grant, is refused.
	g, err := h.grants(ctx, writertest.Space)
	if err != nil {
		t.Fatal(err)
	}
	pack := fx.rig.Writer.Root(writertest.Space).GetPacks()[0].GetId()
	for name, req := range map[string]struct{ space, auth string }{
		"another Space": {"other-space", g.GetAuthorization()},
		"no grant":      {writertest.Space, ""},
	} {
		u := g.GetPackBaseUrl() + "/" + req.space + "/packs/" + pack[:2] + "/" + pack + ".kvf"
		get, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if req.auth != "" {
			get.Header.Set("Authorization", req.auth)
		}
		resp, err := http.DefaultClient.Do(get)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("read with %s: %s", name, resp.Status)
		}
	}
}

// TestGrantRenewsBeforeExpiry reads the Space with two-second grants across
// several of their lives through one opener: every read is authorized, so
// each grant was renewed before it expired.
func TestGrantRenewsBeforeExpiry(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	fx := newFixture(t)
	_, src, tree := newTree(t, 6)
	h := fx.newHolder(t, true)
	if err := fx.publish(ctx, h, tree, src); err != nil {
		t.Fatal(err)
	}

	// Read the tree's root from the Space's packs every quarter second for
	// five seconds, each time with a fresh index, through one opener whose
	// grants last two seconds.
	fx.issuer.TTL = 2 * time.Second
	opener := cdn_bstore.NewGrantKeeper(h.grants, writertest.Space).Opener(nil)
	packs := fx.rig.Writer.Root(writertest.Space).GetPacks()
	issued := h.issued.Load()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		store := packfile_store.NewPackfileStore(opener, nil)
		store.UpdateManifest(packs)
		exists, err := store.GetBlockExistsBatch(ctx, []*block.BlockRef{tree.GetRootRef()})
		store.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !exists[0] {
			t.Fatal("the Space lacks the tree's root")
		}
	}
	if n := h.issued.Load() - issued; n < 3 {
		t.Fatalf("%d grants issued over five seconds of two-second grants", n)
	}
	if fx.b2.refused.Load() != 0 {
		t.Fatalf("%d reads refused", fx.b2.refused.Load())
	}
}

// file is one file of a published tree.
type file struct {
	data []byte
}

// newTree writes n random files as one UnixFS object and returns them, the
// World's block store and the World head.
func newTree(t *testing.T, n int) (map[string]*file, block.StoreOps, *bucket.ObjectRef) {
	// Make the files.
	ctx := t.Context()
	files := map[string]*file{}
	fsys := fstest.MapFS{}
	for i := range n {
		data := make([]byte, 20<<10+i*256)
		if _, err := rand.Read(data); err != nil {
			t.Fatal(err)
		}
		name := fmt.Sprintf("dir%d/file%d.bin", i%2, i)
		files[name] = &file{data: data}
		fsys[name] = &fstest.MapFile{Data: data, Mode: 0o644}
	}

	// Write them as one UnixFS object in a World.
	w, err := publisher.OpenLocalWorld(ctx, logrus.NewEntry(logrus.New()), filepath.Join(t.TempDir(), "source.s4wave"), "grant-source", "grant-source")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(w.Release)
	tx, err := w.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	ref, err := world.AccessObject(ctx, tx.AccessWorldState, nil, func(bcs *block.Cursor) error {
		return unixfs_block.CreateFromFS(ctx, bcs, fsys, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	obj, err := tx.CreateObject(ctx, fsKey, ref)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Read the committed head and its block store.
	var head *bucket.ObjectRef
	var store block.StoreOps
	err = w.Engine.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		head, store = cursor.GetRefWithOpArgs(), cursor.GetBlockStore()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files, store, head
}

// readTree mounts the private Space with h's grants on the bus and requires
// every file.
func readTree(ctx context.Context, t *testing.T, fx *fixture, h *holder, files map[string]*file) {
	// Resolve the holder's grants on a bus.
	le := logrus.NewEntry(logrus.New())
	b, _, err := controllerbus_core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	release, err := b.AddController(ctx, &grantResolver{src: h.grants}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Mount the Space's World from its pointer host and the grants.
	ctx, cancel := context.WithCancel(ctx)
	conf := cdn_world_controller.NewConfig("private", writertest.Space, fx.rig.RootsURL)
	conf.RootPointerBaseUrl = fx.rig.RootsURL
	conf.PrivateSpace = true
	ctrl := cdn_world_controller.NewController(le, b, conf)
	done := make(chan error, 1)
	go func() { done <- ctrl.Execute(ctx) }()
	defer func() { cancel(); <-done }()
	eng, err := ctrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Read each file through a UnixFS handle.
	ws := world.NewEngineWorldState(eng, false)
	fsc := unixfs_world.NewFSCursor(le, ws, fsKey, unixfs_world.FSType_FSType_FS_NODE, nil, false)
	fsh, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		fsc.Release()
		t.Fatal(err)
	}
	defer fsh.Release()
	for name, f := range files {
		child, _, err := fsh.LookupPath(ctx, name)
		if err != nil {
			t.Fatalf("lookup %s: %v", name, err)
		}
		got, err := unixfs.ReadFile(ctx, child)
		child.Release()
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !bytes.Equal(got, f.data) {
			t.Fatalf("%s differs from its source", name)
		}
	}
}

// grantResolver resolves LookupCdnSpaceGrants with one source.
type grantResolver struct {
	src cdn_bstore.GrantSource
}

// GetControllerInfo names the resolver.
func (r *grantResolver) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/cdn-grants", controller.MustParseVersion("0.0.1"), "CDN grant test")
}

// Execute does nothing.
func (r *grantResolver) Execute(context.Context) error { return nil }

// Close releases nothing.
func (r *grantResolver) Close() error { return nil }

// HandleDirective resolves grant lookups for the test Space.
func (r *grantResolver) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	d, ok := inst.GetDirective().(cdn_bstore.LookupCdnSpaceGrants)
	if !ok || d.LookupCdnSpaceGrantsSpaceID() != writertest.Space {
		return nil, nil
	}
	return directive.Resolvers(directive.NewValueResolver([]cdn_bstore.GrantSource{r.src})), nil
}
