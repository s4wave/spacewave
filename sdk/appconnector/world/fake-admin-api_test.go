package s4wave_appconnector_world_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// fakeAdminAPI is a local HTTP server that stands in for the application's
// admin API. It accepts only the API token until the token is revoked.
type fakeAdminAPI struct {
	// server serves the admin paths.
	server *httptest.Server
	// requests counts every request that reached a handler.
	requests atomic.Int32
	// revoked rejects every request, as a revoked token does.
	revoked atomic.Bool
}

func newFakeAdminAPI(t *testing.T) *fakeAdminAPI {
	// Report failures at the caller.
	t.Helper()

	// Serve each path behind the bearer token check.
	api := &fakeAdminAPI{}
	mux := http.NewServeMux()
	serve := func(contentType, body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			api.requests.Add(1)
			if api.revoked.Load() || r.Header.Get("Authorization") != "Bearer "+apiToken {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", contentType)
			_, _ = w.Write([]byte(body))
		}
	}
	mux.HandleFunc(statsPath, serve("application/json", statsBody))
	mux.HandleFunc(bigPath, serve("text/plain", strings.Repeat("x", 2*maxBodyBytes)))
	api.server = httptest.NewServer(mux)
	t.Cleanup(api.server.Close)
	return api
}
