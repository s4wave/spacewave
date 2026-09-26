//go:build js && tinygo && bldr_tinygo_js_imports

package provider_spacewave

import (
	"net/http"

	"github.com/s4wave/spacewave/net/httpclient"
)

func newProviderHTTPTransport(buf *CacheSeedBuffer) http.RoundTripper {
	return NewCacheSeedRecordingTransport(httpclient.NewFetchTransport(nil), buf)
}
