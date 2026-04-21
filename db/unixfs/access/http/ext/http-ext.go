package unixfs_access_http_ext

import (
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/go-brotli-decoder"
)

// NewFileServerExt builds a new http.FileServer which has extended content-type support.
func NewFileServerExt(hfs http.FileSystem) http.Handler {
	handler := http.FileServer(hfs)
	handlerFunc := func(rw http.ResponseWriter, req *http.Request) {
		hasSuffix := func(suffix string) bool {
			return strings.HasSuffix(req.URL.Path, suffix)
		}

		if hasSuffix(".wasm") || hasSuffix(".wasm.br") {
			rw.Header().Set("Content-Type", "application/wasm")
		}

		// fetch() does not set Accept-Encoding nor does it handle Content-Encoding.
		// https://stackoverflow.com/questions/78295701/fetch-set-accept-encoding-and-honor-content-encoding
		// If the content is brotli encoded (ends with .br) and not in Accept-Encoding let's decompress it here.
		if hasSuffix(".br") {
			acceptsBr := slices.ContainsFunc(
				strings.Split(req.Header.Get("accept-encoding"), ","),
				func(val string) bool {
					return strings.ToLower(strings.TrimSpace(val)) == "br"
				},
			)
			if !acceptsBr {
				f, err := hfs.Open(path.Clean(req.URL.Path))
				if err != nil {
					msg, code := ToHTTPError(err)
					http.Error(rw, msg, code)
					return
				}

				// Omit sending the Content-Length header since we don't know the decompressed length.
				brReader := brotli.NewReader(f)
				_, err = io.Copy(rw, brReader)
				if err != nil {
					http.Error(rw, err.Error(), 500)
				}

				// done
				return
			} else {
				rw.Header().Set("Content-Encoding", "br")
			}
		}

		if rw.Header().Get("Content-Encoding") != "" {
			rw.Header().Add("Access-Control-Expose-Headers", "Content-Encoding")
			rw.Header().Add("Vary", "Content-Encoding")

			// XXX: When setting Content-Encoding, Go will drop the Content-Length header.
			// https://github.com/golang/go/issues/66735
			// https://github.com/golang/go/blob/9f136650/src/net/http/fs.go#L377
			var st fs.FileInfo
			f, err := hfs.Open(path.Clean(req.URL.Path))
			if err == nil {
				st, err = f.Stat()
			}
			if err != nil {
				msg, code := ToHTTPError(err)
				http.Error(rw, msg, code)
				return
			}
			rw.Header().Add("Content-Length", strconv.Itoa(int(st.Size())))
		}

		handler.ServeHTTP(rw, req)
	}

	return http.HandlerFunc(handlerFunc)
}

// https://github.com/golang/go/blob/9f1366508/src/net/http/fs.go#L723
func ToHTTPError(err error) (msg string, httpStatus int) {
	if errors.Is(err, fs.ErrNotExist) {
		return "404 page not found", http.StatusNotFound
	}
	if errors.Is(err, fs.ErrPermission) {
		return "403 Forbidden", http.StatusForbidden
	}
	// Default:
	return "500 Internal Server Error", http.StatusInternalServerError
}
