package bifrost_http

import (
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

// encodedAssetVary is the Vary header value advertised for encoded assets.
const encodedAssetVary = "Accept-Encoding"

// NewEncodedAssetFileServer serves an http.FileSystem with browser release
// asset headers for precompressed immutable files.
func NewEncodedAssetFileServer(hfs http.FileSystem) http.Handler {
	// Keep directory, conditional, and range handling in the standard file server.
	fileServer := http.FileServer(hfs)

	// Apply immutable asset metadata before serving or decoding the file.
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		applyEncodedAssetHeaders(rw, req, hfs)

		// Serve Brotli assets directly when the client lacks Brotli support.
		if maybeServeDecodedBrotli(rw, req, hfs) {
			return
		}
		fileServer.ServeHTTP(rw, req)
	})
}

// applyEncodedAssetHeaders sets the content type and encoding headers for a
// precompressed asset request.
func applyEncodedAssetHeaders(rw http.ResponseWriter, req *http.Request, hfs http.FileSystem) {
	// Classify the requested asset and its optional content encoding.
	reqPath := req.URL.Path
	contentType, encoded := encodedAssetContentType(reqPath)
	if contentType == "" {
		return
	}

	// Plain assets need no metadata read; FileServer supplies the size and
	// replaces Content-Type when it returns an HTTP error.
	if encoded == "" {
		rw.Header().Set("Content-Type", contentType)
		return
	}

	// Verify the encoded file exists before advertising an encoded response.
	st, err := statHTTPFile(hfs, reqPath)
	if err != nil {
		return
	}

	// Advertise the selected encoding and cache-vary behavior.
	rw.Header().Set("Content-Type", contentType)
	switch encoded {
	case "gzip":
		rw.Header().Set("Content-Encoding", "gzip")
		rw.Header().Set("Vary", encodedAssetVary)
	case "br":
		if acceptsEncoding(req, "br") {
			rw.Header().Set("Content-Encoding", "br")
			rw.Header().Set("Vary", encodedAssetVary)
		}
	}

	// Set the encoded size after content negotiation succeeds.
	if rw.Header().Get("Content-Encoding") == "" {
		return
	}
	rw.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
}

// encodedAssetContentType returns the content type and content encoding for an
// asset name. The content type is empty when the name is not a served asset.
func encodedAssetContentType(name string) (string, string) {
	switch {
	case strings.HasSuffix(name, ".gz"):
		return contentTypeForAsset(strings.TrimSuffix(name, ".gz")), "gzip"
	case strings.HasSuffix(name, ".br"):
		return contentTypeForAsset(strings.TrimSuffix(name, ".br")), "br"
	default:
		contentType := contentTypeForAsset(name)
		if contentType != "application/octet-stream" {
			return contentType, ""
		}
		return "", ""
	}
}

// contentTypeForAsset returns the content type for an asset file name.
func contentTypeForAsset(name string) string {
	// Keep browser module and release formats independent of host MIME tables.
	switch {
	case strings.HasSuffix(name, ".wasm"):
		return "application/wasm"
	case strings.HasSuffix(name, ".mjs"), strings.HasSuffix(name, ".js"):
		return "application/javascript"
	case strings.HasSuffix(name, ".css"):
		return "text/css; charset=utf-8"
	case strings.HasSuffix(name, ".html"):
		return "text/html; charset=utf-8"
	case strings.HasSuffix(name, ".json"):
		return "application/json; charset=utf-8"
	case strings.HasSuffix(name, ".kvfile"), strings.HasSuffix(name, ".packedmsg"):
		return "application/octet-stream"
	}

	// Resolve other extensions through the registered MIME types.
	if ext := path.Ext(name); ext != "" {
		if contentType := mime.TypeByExtension(ext); contentType != "" {
			return contentType
		}
	}
	return "application/octet-stream"
}

// acceptsEncoding returns true if the request accepts the encoding with a
// non-zero quality.
func acceptsEncoding(req *http.Request, want string) bool {
	for val := range strings.SplitSeq(req.Header.Get("Accept-Encoding"), ",") {
		encoding, params, _ := strings.Cut(strings.TrimSpace(val), ";")
		if strings.EqualFold(encoding, want) {
			return encodingQuality(params) > 0
		}
	}
	return false
}

// encodingQuality parses the q parameter from one Accept-Encoding element. A
// missing q parameter means quality 1.
func encodingQuality(params string) float64 {
	// An omitted quality parameter accepts the encoding at full quality.
	if params == "" {
		return 1
	}

	// Honor an explicit quality and reject malformed values.
	for param := range strings.SplitSeq(params, ";") {
		key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
		if !ok || !strings.EqualFold(key, "q") {
			continue
		}
		q, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			return 0
		}
		return q
	}
	return 1
}

// statHTTPFile stats a file in the filesystem without keeping it open.
func statHTTPFile(hfs http.FileSystem, name string) (fs.FileInfo, error) {
	f, err := openHTTPFile(hfs, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}

// openHTTPFile opens a cleaned path in the filesystem.
func openHTTPFile(hfs http.FileSystem, name string) (http.File, error) {
	clean, err := cleanHTTPFilePath(name)
	if err != nil {
		return nil, err
	}
	return hfs.Open(clean)
}

// cleanHTTPFilePath normalizes a request path and rejects traversal outside
// the filesystem root.
func cleanHTTPFilePath(name string) (string, error) {
	// HTTP asset paths use slash separators on every host platform.
	if strings.Contains(name, "\\") {
		return "", fs.ErrPermission
	}

	// Reject traversal components before normalizing the filesystem path.
	for part := range strings.SplitSeq(name, "/") {
		if part == ".." {
			return "", fs.ErrPermission
		}
	}
	clean := strings.TrimPrefix(path.Clean("/"+strings.TrimPrefix(name, "/")), "/")
	if clean == "" {
		clean = "."
	}
	if !fs.ValidPath(clean) {
		return "", fs.ErrPermission
	}
	return clean, nil
}

// ToHTTPError maps filesystem errors to the HTTP status used by http.FileServer.
func ToHTTPError(err error) (msg string, httpStatus int) {
	if errors.Is(err, fs.ErrNotExist) {
		return "404 page not found", http.StatusNotFound
	}
	if errors.Is(err, fs.ErrPermission) {
		return "403 Forbidden", http.StatusForbidden
	}
	return "500 Internal Server Error", http.StatusInternalServerError
}
