//go:build !tinygo && !js

package block_store_s3

import "net/http"

// newHTTPClient returns the HTTP client for native platforms.
func newHTTPClient() *http.Client {
	return http.DefaultClient
}
