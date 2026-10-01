package cdn_world_controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/pkg/errors"
)

// testCDNHosts maps each test's CDN host to its handler.
var testCDNHosts sync.Map

// testCDNHostSeq numbers the test CDN hosts.
var testCDNHostSeq atomic.Int64

// installTestCDN routes http.DefaultClient, which the controller uses for CDN
// reads, through hostTransport for the rest of the test binary.
var installTestCDN sync.Once

// hostTransport serves each request with the handler registered for its host.
// A browser cannot listen on a socket, so the tests serve the CDN through this
// transport in place of an httptest server and run the same way in native Go
// and js/wasm. Each test has its own host, so a request from another test's
// controller never reaches this test's handler.
type hostTransport struct{}

// RoundTrip runs the host's handler and returns its recorded response, or the
// request context error when the request is canceled first.
func (hostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Refuse hosts with no live test CDN.
	handler, ok := testCDNHosts.Load(req.URL.Host)
	if !ok {
		return nil, errors.Errorf("no test CDN serves %s", req.URL.Host)
	}

	// Run the handler apart from the caller so a blocked handler cannot outlive
	// the request context.
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.(http.Handler).ServeHTTP(rec, req)
	}()

	// Return the response once the handler finishes.
	select {
	case <-done:
		return rec.Result(), nil
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
}

// serveTestCDN serves handler through http.DefaultClient at a host unique to
// this test until the test ends, and returns its base URL.
func serveTestCDN(t *testing.T, handler http.Handler) string {
	// Route CDN reads through the test transport.
	installTestCDN.Do(func() { http.DefaultClient.Transport = hostTransport{} })

	// Register the handler at a fresh host for the test's lifetime.
	host := fmt.Sprintf("cdn-%d.test", testCDNHostSeq.Add(1))
	testCDNHosts.Store(host, handler)
	t.Cleanup(func() { testCDNHosts.Delete(host) })
	return "http://" + host
}
