package cdn_bstore

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"

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

// RootPointerFetcher fetches one CDN Space's root.packedmsg and revalidates
// the last pointer it decoded with that response's ETag. An unchanged pointer
// then costs a 304 with no body instead of the whole manifest, which every
// reader refetches each time its pointer TTL expires. Safe for concurrent use.
type RootPointerFetcher struct {
	// cli sends the root pointer requests.
	cli *http.Client
	// url is the Space's root.packedmsg URL.
	url string
	// spaceID is the Space the pointer must belong to.
	spaceID string

	// mtx guards etag and pointer.
	mtx sync.Mutex
	// etag is the validator of the response that produced pointer; empty when
	// the origin sent none or nothing has been fetched.
	etag string
	// pointer is the last decoded pointer; callers receive clones of it.
	pointer *cdn.CdnRootPointer
}

// NewRootPointerFetcher constructs a fetcher for spaceID's root pointer under
// cdnBaseURL. A nil httpCli selects http.DefaultClient.
func NewRootPointerFetcher(httpCli *http.Client, cdnBaseURL, spaceID string) *RootPointerFetcher {
	if httpCli == nil {
		httpCli = http.DefaultClient
	}
	return &RootPointerFetcher{
		cli:     httpCli,
		url:     strings.TrimRight(cdnBaseURL, "/") + rootPointerPath(spaceID),
		spaceID: spaceID,
	}
}

// Fetch fetches and decodes the Space's root pointer, sending If-None-Match
// with the last response's ETag so the origin can answer 304 Not Modified, in
// which case Fetch returns the last decoded pointer. Returns nil, nil on 404 so
// callers can treat fresh Spaces as empty. A failed fetch keeps the last
// pointer and validator for the next attempt.
func (f *RootPointerFetcher) Fetch(ctx context.Context) (*cdn.CdnRootPointer, error) {
	// Validate the requested Space.
	if f.spaceID == "" {
		return nil, errors.New("cdn bstore: space id required")
	}

	// Revalidate the last decoded pointer when its response carried an ETag.
	f.mtx.Lock()
	etag, prev := f.etag, f.pointer
	f.mtx.Unlock()
	resp, err := fetchRootPointerResponse(ctx, f.cli, f.url, etag)
	if err != nil {
		return nil, errors.Wrap(err, "fetching root pointer")
	}
	defer resp.Close()

	// Serve an unchanged pointer from the last decode, forget the validator of
	// an absent pointer, and reject other failed responses.
	switch resp.StatusCode() {
	case http.StatusNotModified:
		if prev == nil {
			return nil, errors.New("cdn root pointer not modified before any fetch")
		}
		return prev.CloneVT(), nil
	case http.StatusNotFound:
		f.store("", nil)
		return nil, nil
	case http.StatusOK:
	default:
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
	if pointer.GetSpaceId() != f.spaceID {
		return nil, errors.Errorf("cdn root pointer space id mismatch: want %q got %q", f.spaceID, pointer.GetSpaceId())
	}

	// Remember the decoded pointer under its validator for the next fetch.
	f.store(resp.ETag(), pointer)
	return pointer.CloneVT(), nil
}

// store replaces the remembered validator and pointer.
func (f *RootPointerFetcher) store(etag string, pointer *cdn.CdnRootPointer) {
	f.mtx.Lock()
	f.etag, f.pointer = etag, pointer
	f.mtx.Unlock()
}

// rootPointerResponse is a platform HTTP response for root.packedmsg.
type rootPointerResponse interface {
	// StatusCode returns the HTTP status code.
	StatusCode() int
	// ETag returns the response's ETag header, or empty.
	ETag() string
	// Body returns the response body.
	Body() io.Reader
	// Close releases the response.
	Close()
}
