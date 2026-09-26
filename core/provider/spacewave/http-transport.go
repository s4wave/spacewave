//go:build !js || !tinygo || !bldr_tinygo_js_imports

package provider_spacewave

import "net/http"

func newProviderHTTPTransport(buf *CacheSeedBuffer) http.RoundTripper {
	return NewCacheSeedRecordingTransport(nil, buf)
}
