//go:build !tinygo

package block_store_s3

import "net/http"

// newHTTPClient returns the HTTP client for S3 requests. It keeps an idle
// connection for each concurrent batch request, so a batch over HTTP/1.1
// reuses its connections instead of paying a TLS handshake per request. In the
// browser the transport sends requests with fetch, so the bucket must allow the
// app's origin through CORS.
func newHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = batchConcurrency
	return &http.Client{Transport: transport}
}
