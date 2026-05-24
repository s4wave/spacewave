package provider_spacewave

import (
	"net/http"

	"github.com/s4wave/spacewave/core/provider/spacewave/cacheseedbuffer"
)

// DefaultCacheSeedBufferCapacity is the default ring buffer size for the
// cache-seed inspector.
const DefaultCacheSeedBufferCapacity = cacheseedbuffer.DefaultCapacity

// CacheSeedEntry is a single recorded HTTP request tagged with a seed reason.
type CacheSeedEntry = cacheseedbuffer.Entry

// CacheSeedBuffer is a goroutine-safe bounded ring buffer recording tagged HTTP
// calls the provider issues.
type CacheSeedBuffer = cacheseedbuffer.Buffer

// NewCacheSeedBuffer constructs a new CacheSeedBuffer with the given capacity.
func NewCacheSeedBuffer(capacity int) *CacheSeedBuffer {
	return cacheseedbuffer.New(capacity)
}

// NewCacheSeedRecordingTransport wraps base so tagged requests are recorded
// before forwarding.
func NewCacheSeedRecordingTransport(base http.RoundTripper, buf *CacheSeedBuffer) http.RoundTripper {
	return cacheseedbuffer.NewRecordingTransport(base, buf)
}
