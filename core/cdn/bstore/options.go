package cdn_bstore

import (
	"net/http"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
)

// DefaultPointerTTL is the fallback TTL for cached root pointers when the
// caller does not override it.
const DefaultPointerTTL = 30 * time.Second

// Options configure a CdnBlockStore.
type Options struct {
	// CdnBaseURL is the public CDN origin (e.g. https://cdn.spacewave.app).
	CdnBaseURL string
	// RootPointerBaseURL optionally serves root.packedmsg from another origin.
	// Empty reads the pointer from CdnBaseURL.
	RootPointerBaseURL string
	// SpaceID is the CDN Space ULID.
	SpaceID string
	// HttpClient overrides the default http.Client.
	HttpClient *http.Client
	// PointerTTL is the cache TTL for the decoded root pointer. Zero falls
	// back to DefaultPointerTTL. Negative disables the TTL (pointer is cached
	// until explicitly invalidated).
	PointerTTL time.Duration
	// IndexCache optionally persists raw packfile index tails across restarts.
	IndexCache packfile_store.IndexCache
	// PointerStore optionally persists the last fetched root pointer, so cached
	// blocks of its manifest stay readable while the CDN is unreachable.
	PointerStore kvtx.Store
}

// PackIndexObjectStoreID returns the stable metadata ObjectStore ID for a CDN
// Space's durable packfile index cache.
func PackIndexObjectStoreID(spaceID string) string {
	return "cdn/" + spaceID + "/pack-index"
}
