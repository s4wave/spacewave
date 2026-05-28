//go:build js && tinygo && bldr_tinygo_js_imports

package provider_spacewave

import (
	"net/http"

	alpha_nethttp "github.com/s4wave/spacewave/core/nethttp"
)

func newProviderHTTPTransport(buf *CacheSeedBuffer) http.RoundTripper {
	return NewCacheSeedRecordingTransport(alpha_nethttp.NewFetchTransport(nil), buf)
}
