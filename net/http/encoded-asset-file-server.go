package bifrost_http

import (
	"errors"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"

	brotli "github.com/aperturerobotics/go-brotli-decoder"
)

const encodedAssetVary = "Accept-Encoding"

// NewEncodedAssetFileServer serves an http.FileSystem with browser release
// asset headers for precompressed immutable files.
func NewEncodedAssetFileServer(hfs http.FileSystem) http.Handler {
	fileServer := http.FileServer(hfs)
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		applyEncodedAssetHeaders(rw, req, hfs)
		if maybeServeDecodedBrotli(rw, req, hfs) {
			return
		}
		fileServer.ServeHTTP(rw, req)
	})
}

func applyEncodedAssetHeaders(rw http.ResponseWriter, req *http.Request, hfs http.FileSystem) {
	reqPath := req.URL.Path
	contentType, encoded := encodedAssetContentType(reqPath)
	if contentType == "" {
		return
	}
	st, err := statHTTPFile(hfs, reqPath)
	if err != nil {
		return
	}
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
	if rw.Header().Get("Content-Encoding") == "" {
		return
	}
	rw.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
}

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

func encodedAssetContentType(name string) (string, string) {
	switch {
	case strings.HasSuffix(name, ".gz"):
		return contentTypeForAsset(strings.TrimSuffix(name, ".gz")), "gzip"
	case strings.HasSuffix(name, ".br"):
		return contentTypeForAsset(strings.TrimSuffix(name, ".br")), "br"
	default:
		if strings.HasSuffix(name, ".wasm") {
			return "application/wasm", ""
		}
		return "", ""
	}
}

func contentTypeForAsset(name string) string {
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
	if ext := path.Ext(name); ext != "" {
		if contentType := mime.TypeByExtension(ext); contentType != "" {
			return contentType
		}
	}
	return "application/octet-stream"
}

func acceptsEncoding(req *http.Request, want string) bool {
	for val := range strings.SplitSeq(req.Header.Get("Accept-Encoding"), ",") {
		encoding, params, _ := strings.Cut(strings.TrimSpace(val), ";")
		if strings.EqualFold(encoding, want) {
			return encodingQuality(params) > 0
		}
	}
	return false
}

func encodingQuality(params string) float64 {
	if params == "" {
		return 1
	}
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

func statHTTPFile(hfs http.FileSystem, name string) (fs.FileInfo, error) {
	f, err := openHTTPFile(hfs, name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Stat()
}

func openHTTPFile(hfs http.FileSystem, name string) (http.File, error) {
	clean := path.Clean(name)
	f, err := hfs.Open(clean)
	if err == nil || !strings.HasPrefix(clean, "/") {
		return f, err
	}
	return hfs.Open(strings.TrimPrefix(clean, "/"))
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
