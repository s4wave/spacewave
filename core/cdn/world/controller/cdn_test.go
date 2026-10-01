package cdn_world_controller

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// testCDNBaseURL is the base URL the in-process test CDN answers.
const testCDNBaseURL = "http://cdn.test"

// handlerTransport serves each request with an in-process handler. A browser
// cannot listen on a socket, so the tests serve the CDN through this transport
// in place of an httptest server and run the same way in native Go and js/wasm.
type handlerTransport struct {
	handler http.Handler
}

// RoundTrip runs the handler and returns its recorded response, or the request
// context error when the request is canceled first.
func (t handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Run the handler apart from the caller so a blocked handler cannot outlive
	// the request context.
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		t.handler.ServeHTTP(rec, req)
	}()

	// Return the response once the handler finishes.
	select {
	case <-done:
		return rec.Result(), nil
	case <-req.Context().Done():
		return nil, req.Context().Err()
	}
}

// serveTestCDN serves handler at testCDNBaseURL through http.DefaultClient,
// which the controller uses for CDN reads, until the test ends. It returns a
// client that reaches the same handler.
func serveTestCDN(t *testing.T, handler http.Handler) *http.Client {
	// Route the default client to the handler and restore it after the test.
	transport := handlerTransport{handler: handler}
	prev := http.DefaultClient.Transport
	http.DefaultClient.Transport = transport
	t.Cleanup(func() { http.DefaultClient.Transport = prev })
	return &http.Client{Transport: transport}
}
