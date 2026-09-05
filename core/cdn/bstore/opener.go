package cdn_bstore

import (
	"net/http"
	"strings"

	"github.com/pkg/errors"
	packfile_store "github.com/s4wave/spacewave/core/provider/spacewave/packfile/store"
)

const (
	// readAheadSize permits the pack reader's sparse cold window. Hash-ordered
	// packs do not imply locality between consecutive file-content blocks;
	// the reader grows its window only after observing nearby requests.
	readAheadSize = 128 * 1024
	// anonymousReaderPageSize sets resident cache granularity.
	anonymousReaderPageSize = 4 * 1024
)

// packURL formats the anonymous CDN pack URL path.
func packURL(cdnBaseURL, spaceID, packID string) string {
	shard := packID
	if len(shard) > 2 {
		shard = shard[:2]
	}
	return strings.TrimRight(cdnBaseURL, "/") + "/" + spaceID + "/packs/" + shard + "/" + packID + ".kvf"
}

// NewAnonymousOpener returns a packfile_store.Opener that builds shared pack
// readers for anonymous HTTP Range requests against the public CDN. The opener
// does not sign requests and does not issue HEAD requests; the pack size must
// be passed in from the manifest entry.
func NewAnonymousOpener(httpCli *http.Client, cdnBaseURL, spaceID string) packfile_store.Opener {
	// Retain the caller's transport or use the shared default client.
	if httpCli == nil {
		httpCli = http.DefaultClient
	}

	// Open known-size immutable packs without a separate metadata request.
	return func(packID string, size int64) (*packfile_store.PackReader, error) {
		if size <= 0 {
			return nil, errors.New("pack size must be known from the manifest")
		}
		return packfile_store.NewHTTPRangeReader(
			httpCli,
			packURL(cdnBaseURL, spaceID, packID),
			size,
			readAheadSize,
			anonymousReaderPageSize,
			nil,
			nil,
		), nil
	}
}
