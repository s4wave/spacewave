package bldr_manifest_pack

import (
	"bytes"
	"io/fs"
	"net/http"
	"time"
)

const (
	// ArtifactPackFilename is the manifest-pack kvfile artifact filename.
	ArtifactPackFilename = "manifest.pack.kvf"
	// ArtifactMetadataFilename is the manifest-pack metadata artifact filename.
	ArtifactMetadataFilename = "manifest-pack.bin"
	// DefaultHTTPMetadataPath is the manifest-pack metadata endpoint path.
	DefaultHTTPMetadataPath = "/manifest-pack.json"
	// DefaultHTTPPackPath is the manifest-pack kvfile endpoint path.
	DefaultHTTPPackPath = "/manifest.pack.kvf"

	manifestPackContentType         = "application/octet-stream"
	manifestPackMetadataContentType = "application/json"
)

// HTTPHandlers serves a manifest-pack metadata document and its kvfile pack.
type HTTPHandlers struct {
	// Metadata serves the ManifestPackMetadata JSON document.
	Metadata http.Handler
	// Pack serves the kvfile pack bytes with HTTP range support.
	Pack http.Handler
}

// ServeHTTP routes the default manifest-pack endpoints.
func (h *HTTPHandlers) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	switch req.URL.Path {
	case DefaultHTTPMetadataPath:
		h.Metadata.ServeHTTP(rw, req)
	case DefaultHTTPPackPath:
		h.Pack.ServeHTTP(rw, req)
	default:
		http.NotFound(rw, req)
	}
}

// NewHTTPHandlersFromFS builds browser-compatible handlers from a manifest-pack artifact tree.
func NewHTTPHandlersFromFS(fsys fs.FS) (*HTTPHandlers, error) {
	metaData, err := fs.ReadFile(fsys, ArtifactMetadataFilename)
	if err != nil {
		return nil, err
	}
	meta := &ManifestPackMetadata{}
	if err := meta.UnmarshalVT(metaData); err != nil {
		return nil, err
	}
	packBytes, err := fs.ReadFile(fsys, ArtifactPackFilename)
	if err != nil {
		return nil, err
	}
	return NewHTTPHandlers(meta, packBytes)
}

// NewHTTPHandlers builds browser-compatible handlers for a manifest-pack artifact.
func NewHTTPHandlers(meta *ManifestPackMetadata, packBytes []byte) (*HTTPHandlers, error) {
	if err := meta.Validate(); err != nil {
		return nil, err
	}
	if err := verifyPackBytes(meta, packBytes); err != nil {
		return nil, err
	}
	metadata, err := meta.MarshalJSON()
	if err != nil {
		return nil, err
	}
	pack := bytes.Clone(packBytes)
	return &HTTPHandlers{
		Metadata: http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			serveBytes(rw, req, "manifest-pack.json", manifestPackMetadataContentType, metadata)
		}),
		Pack: http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
			serveBytes(rw, req, meta.GetPack().GetId(), manifestPackContentType, pack)
		}),
	}, nil
}

// serveBytes serves in-memory bytes over HTTP with browser-readable headers.
func serveBytes(rw http.ResponseWriter, req *http.Request, name, contentType string, data []byte) {
	setBrowserReadableHeaders(rw.Header(), contentType)
	if req.Method == http.MethodOptions {
		rw.WriteHeader(http.StatusNoContent)
		return
	}
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		rw.Header().Set("Allow", "GET, HEAD, OPTIONS")
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	http.ServeContent(rw, req, name, time.Time{}, bytes.NewReader(data))
}

// setBrowserReadableHeaders sets the headers needed for browser access to the served bytes.
func setBrowserReadableHeaders(h http.Header, contentType string) {
	h.Set("Accept-Ranges", "bytes")
	h.Set("Content-Type", contentType)
	h.Set("Access-Control-Allow-Headers", "Range")
	h.Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS")
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Expose-Headers", "Accept-Ranges, Content-Range, Content-Length, Content-Type")
	h.Set("Cross-Origin-Resource-Policy", "cross-origin")
}
