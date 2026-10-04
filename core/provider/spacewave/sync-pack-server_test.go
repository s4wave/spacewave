package provider_spacewave

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
)

// newSyncTestPackServer retains uploaded packs for lower-store duplicate probes.
func newSyncTestPackServer(t *testing.T, handler http.Handler) (*httptest.Server, *packfile_store.PackfileStore) {
	// Keep uploaded pack bytes behind the existing cloud fixture's reader.
	t.Helper()
	cloud := &compactTestCloud{packs: make(map[string][]byte)}
	lower := packfile_store.NewPackfileStore(cloud.open, nil)
	t.Cleanup(lower.Close)

	// Preserve handler gates while retaining each pack before acknowledging it.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Let the test handler control when the upload body is read.
		var body bytes.Buffer
		r.Body = io.NopCloser(io.TeeReader(r.Body, &body))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)

		// Drain any unread upload bytes before publishing its success response.
		if id := r.Header.Get("X-Pack-ID"); id != "" {
			if _, err := io.Copy(io.Discard, r.Body); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			cloud.mtx.Lock()
			cloud.packs[id] = body.Bytes()
			cloud.mtx.Unlock()
		}

		// Forward the test response only after its uploaded pack can be opened.
		for key, values := range response.Header() {
			w.Header()[key] = slices.Clone(values)
		}
		w.WriteHeader(response.Code)
		if _, err := w.Write(response.Body.Bytes()); err != nil {
			t.Error(err)
		}
	}))
	return server, lower
}
