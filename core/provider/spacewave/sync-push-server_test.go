package provider_spacewave

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
)

// testChecksumHeader is the storage header carrying the base64 SHA-256 of an
// upload body.
const testChecksumHeader = "X-Amz-Checksum-Sha256"

// testPushedPack is one pack committed to a testPushServer.
type testPushedPack struct {
	// req is the admitted push request.
	req *packfile.PushRequest
	// data is the uploaded pack body.
	data []byte
}

// testPushServer fakes the cloud side of the pack push protocol. It admits
// each push with an upload URL under base, checks the uploaded bytes against
// the admitted size and digest, and catalogs the pack on commit. As in the
// cloud, an upload or commit of a pack already cataloged succeeds, so a
// retried push racing its original commit completes.
type testPushServer struct {
	t    *testing.T
	base string

	// onUpload runs on each upload before it is accepted.
	onUpload func(req *packfile.PushRequest)

	mu       sync.Mutex
	pending  map[string]*packfile.PushRequest
	uploaded map[string][]byte
	packs    []*testPushedPack
}

// newTestPushServer returns a push server whose upload URLs start with base.
func newTestPushServer(t *testing.T, base string) *testPushServer {
	return &testPushServer{
		t:        t,
		base:     base,
		pending:  make(map[string]*packfile.PushRequest),
		uploaded: make(map[string][]byte),
	}
}

// startTestPushServer serves a testPushServer over HTTP until the test ends.
// Its base is the server URL.
func startTestPushServer(t *testing.T) *testPushServer {
	// Serve a push server and point its uploads at the listener.
	s := newTestPushServer(t, "")
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	s.base = srv.URL
	return s
}

// findCommitted returns the committed pack with packID, or nil. The caller
// holds mu.
func (s *testPushServer) findCommitted(packID string) *testPushedPack {
	for _, pack := range s.packs {
		if pack.req.GetPackId() == packID {
			return pack
		}
	}
	return nil
}

// committed returns the committed packs in commit order.
func (s *testPushServer) committed() []*testPushedPack {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.packs)
}

// lower returns a pack store that reads the committed packs, so later dirty
// marks can probe published entries.
func (s *testPushServer) lower(t *testing.T) *packfile_store.PackfileStore {
	// Open each committed pack from its retained body.
	store := packfile_store.NewPackfileStore(func(packID string, size int64) (*packfile_store.PackReader, error) {
		// Find the committed body and serve reads from it.
		s.mu.Lock()
		defer s.mu.Unlock()
		var data []byte
		if pack := s.findCommitted(packID); pack != nil {
			data = pack.data
		}
		return packfile_store.NewPackReader(packID, size, &syncPackTransport{data: data}), nil
	}, nil)
	t.Cleanup(store.Close)
	return store
}

// testPushedBody returns the body of the only committed pack.
func testPushedBody(t *testing.T, s *testPushServer) []byte {
	// Require exactly one committed pack.
	t.Helper()
	packs := s.committed()
	if len(packs) != 1 {
		t.Fatalf("committed %d packs, want 1", len(packs))
	}
	return packs[0].data
}

// uploadTotals returns the count and total size of the committed packs.
func (s *testPushServer) uploadTotals() (packs int, size int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, pack := range s.packs {
		size += int64(len(pack.data))
	}
	return len(s.packs), size
}

// ServeHTTP serves push, upload and commit requests.
func (s *testPushServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Read the request body.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.t.Errorf("read body: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Route the request by method and path.
	p := r.URL.Path
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/sync/push"):
		s.servePush(w, body)
	case r.Method == http.MethodPut && strings.HasPrefix(p, "/upload/"):
		s.serveUpload(w, r, strings.TrimPrefix(p, "/upload/"), body)
	case r.Method == http.MethodPost && strings.HasSuffix(p, "/commit"):
		s.serveCommit(w, path.Base(strings.TrimSuffix(p, "/commit")))
	default:
		s.t.Errorf("unexpected request: %s %s", r.Method, p)
		w.WriteHeader(http.StatusNotFound)
	}
}

// servePush admits a push, skipping the upload for a committed pack.
func (s *testPushServer) servePush(w http.ResponseWriter, body []byte) {
	// Decode the push request.
	req := &packfile.PushRequest{}
	if err := req.UnmarshalVT(body); err != nil {
		s.t.Errorf("unmarshal push request: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Answer a committed pack without an upload and admit any other.
	resp := &packfile.PushResponse{PackId: req.GetPackId(), SizeBytes: req.GetSizeBytes()}
	s.mu.Lock()
	resp.AlreadyExists = s.findCommitted(req.GetPackId()) != nil
	if !resp.AlreadyExists {
		s.pending[req.GetPackId()] = req
		resp.Upload = &packfile.PushUpload{
			Url:     s.base + "/upload/" + req.GetPackId(),
			Headers: map[string]string{testChecksumHeader: base64.StdEncoding.EncodeToString(req.GetSha256())},
		}
	}
	s.mu.Unlock()
	writeTestPushResponse(s.t, w, resp)
}

// serveUpload stores an upload body that matches its admitted push. An
// upload of a committed pack overwrites the same bytes, as a signed upload
// URL does until it expires.
func (s *testPushServer) serveUpload(w http.ResponseWriter, r *http.Request, packID string, body []byte) {
	// Find the admitted or committed push and run the upload hook.
	s.mu.Lock()
	req := s.pending[packID]
	committed := s.findCommitted(packID)
	if req == nil && committed != nil {
		req = committed.req
	}
	onUpload := s.onUpload
	s.mu.Unlock()
	if req == nil {
		s.t.Errorf("upload of unadmitted pack %q", packID)
		w.WriteHeader(http.StatusForbidden)
		return
	}
	if onUpload != nil {
		onUpload(req)
	}

	// Check the body against the admitted size and digest.
	sum := sha256.Sum256(body)
	switch {
	case r.ContentLength != int64(req.GetSizeBytes()): //nolint:gosec // test sizes are small
		s.t.Errorf("upload content length %d, admitted %d", r.ContentLength, req.GetSizeBytes())
	case r.Header.Get(testChecksumHeader) != base64.StdEncoding.EncodeToString(req.GetSha256()):
		s.t.Errorf("upload checksum header %q", r.Header.Get(testChecksumHeader))
	case !bytes.Equal(sum[:], req.GetSha256()):
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("checksum mismatch"))
		return
	}

	// Store the body until the commit, unless the pack is already cataloged.
	s.mu.Lock()
	if s.findCommitted(packID) == nil {
		s.uploaded[packID] = body
	}
	s.mu.Unlock()
	w.WriteHeader(http.StatusOK)
}

// serveCommit catalogs an uploaded pack. A repeated commit of a cataloged
// pack succeeds.
func (s *testPushServer) serveCommit(w http.ResponseWriter, packID string) {
	// Answer a cataloged pack, or move an uploaded pack into the catalog.
	s.mu.Lock()
	req, data := s.pending[packID], s.uploaded[packID]
	if pack := s.findCommitted(packID); pack != nil {
		req, data = pack.req, pack.data
	} else if req != nil && data != nil {
		delete(s.pending, packID)
		delete(s.uploaded, packID)
		s.packs = append(s.packs, &testPushedPack{req: req, data: data})
	}
	s.mu.Unlock()

	// Refuse a commit without an upload.
	if req == nil || data == nil {
		s.t.Errorf("commit of pack %q without an upload", packID)
		w.WriteHeader(http.StatusConflict)
		return
	}
	writeTestPushResponse(s.t, w, &packfile.PushResponse{PackId: packID, SizeBytes: req.GetSizeBytes()})
}

// writeTestPushResponse writes a binary push response.
func writeTestPushResponse(t *testing.T, w http.ResponseWriter, resp *packfile.PushResponse) {
	data, err := resp.MarshalVT()
	if err != nil {
		t.Errorf("marshal push response: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(data)
}
