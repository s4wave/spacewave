package cdn_bstore

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	packedmsg "github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/core/cdn"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
)

// MaxRootPackedmsgBytes caps anonymous root.packedmsg fetches.
const MaxRootPackedmsgBytes int64 = 4 << 20

// rootPointerPath formats the anonymous CDN root pointer URL path.
func rootPointerPath(spaceID string) string {
	return "/" + spaceID + "/root.packedmsg"
}

// RootPointerBaseURL returns the origin serving root.packedmsg: rootBaseURL
// when set, otherwise cdnBaseURL.
func RootPointerBaseURL(cdnBaseURL, rootBaseURL string) string {
	if rootBaseURL != "" {
		return rootBaseURL
	}
	return cdnBaseURL
}

// waitPointerChange returns the root pointer guarded by bcast once it differs
// from prev. current reads the pointer under the lock and reports whether the
// store has closed.
func waitPointerChange(
	ctx context.Context,
	bcast *broadcast.Broadcast,
	prev *cdn.CdnRootPointer,
	current func() (*cdn.CdnRootPointer, bool),
) (*cdn.CdnRootPointer, error) {
	for {
		// Read the pointer and its next change event under the same lock.
		var ptr *cdn.CdnRootPointer
		var closed bool
		var changed <-chan struct{}
		bcast.HoldLock(func(_ func(), wait func() <-chan struct{}) {
			ptr, closed = current()
			changed = wait()
		})

		// Return a changed pointer, or wait for the next publication.
		if closed {
			return nil, packfile_store.ErrPackfileStoreClosed
		}
		if !ptr.EqualVT(prev) {
			return ptr, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-changed:
		}
	}
}

// FetchRootPointer fetches and decodes root.packedmsg for a CDN Space.
// Returns nil, nil on 404 so callers can treat fresh Spaces as empty.
func FetchRootPointer(ctx context.Context, httpCli *http.Client, cdnBaseURL, spaceID string) (*cdn.CdnRootPointer, error) {
	// Validate the requested Space and select its HTTP client.
	if spaceID == "" {
		return nil, errors.New("cdn bstore: space id required")
	}
	if httpCli == nil {
		httpCli = http.DefaultClient
	}

	// Open the CDN root pointer response for the requested Space.
	url := strings.TrimRight(cdnBaseURL, "/") + rootPointerPath(spaceID)
	resp, err := fetchRootPointerResponse(ctx, httpCli, url)
	if err != nil {
		return nil, errors.Wrap(err, "fetching root pointer")
	}
	defer resp.Close()

	// Treat an absent CDN pointer as an empty Space and reject other failed responses.
	if resp.StatusCode() == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, errors.Errorf("cdn root pointer status %d", resp.StatusCode())
	}

	// Read the CDN pointer within the anonymous response size limit.
	body, err := io.ReadAll(io.LimitReader(resp.Body(), MaxRootPackedmsgBytes+1))
	if err != nil {
		return nil, errors.Wrap(err, "reading root pointer body")
	}
	if int64(len(body)) > MaxRootPackedmsgBytes {
		return nil, errors.Errorf("cdn root pointer exceeds %d bytes", MaxRootPackedmsgBytes)
	}

	// Extract the protobuf payload from the packed root pointer.
	raw, ok := packedmsg.DecodePackedMessage(string(body))
	if !ok {
		return nil, errors.New("cdn root pointer failed packedmsg decode")
	}

	// Decode the CDN pointer and verify that it belongs to the requested Space.
	pointer := &cdn.CdnRootPointer{}
	if err := pointer.UnmarshalVT(raw); err != nil {
		return nil, errors.Wrap(err, "unmarshaling cdn root pointer")
	}
	if pointer.GetSpaceId() != spaceID {
		return nil, errors.Errorf("cdn root pointer space id mismatch: want %q got %q", spaceID, pointer.GetSpaceId())
	}
	return pointer, nil
}

// rootPointerResponse is a platform HTTP response for root.packedmsg.
type rootPointerResponse interface {
	// StatusCode returns the HTTP status code.
	StatusCode() int
	// Body returns the response body.
	Body() io.Reader
	// Close releases the response.
	Close()
}
