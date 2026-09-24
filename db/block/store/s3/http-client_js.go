//go:build !tinygo && js

package block_store_s3

import (
	"net/http"

	"github.com/s4wave/spacewave/net/httpclient"
)

// newHTTPClient returns an HTTP client that sends requests with the browser's
// fetch. The bucket must allow the app's origin through CORS.
func newHTTPClient() *http.Client {
	return &http.Client{Transport: httpclient.NewFetchTransport(nil)}
}
