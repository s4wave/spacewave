//go:build !js

package bifrost_http

import (
	"io"
	"net/http"
	"strings"

	brotli "github.com/aperturerobotics/go-brotli-decoder"
)

// maybeServeDecodedBrotli decodes a precompressed ".br" asset for a client that
// does not advertise brotli support, so an explicit ".br" URL still serves
// usable bytes. It returns true when it has written the response. The brotli
// decoder is excluded from the js/wasm browser build because browsers always
// send Accept-Encoding: br, so this fallback is unreachable in the browser and
// would otherwise add the decoder and its dictionary to the runtime bundle.
func maybeServeDecodedBrotli(rw http.ResponseWriter, req *http.Request, hfs http.FileSystem) bool {
	if !strings.HasSuffix(req.URL.Path, ".br") || acceptsEncoding(req, "br") {
		return false
	}
	f, err := openHTTPFile(hfs, req.URL.Path)
	if err != nil {
		msg, code := ToHTTPError(err)
		http.Error(rw, msg, code)
		return true
	}
	defer f.Close()

	_, err = io.Copy(rw, brotli.NewReader(f))
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
	}
	return true
}
