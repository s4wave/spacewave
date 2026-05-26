package cdn_bstore

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/pkg/errors"
	packedmsg "github.com/s4wave/spacewave/bldr/util/packedmsg"
	"github.com/s4wave/spacewave/core/cdn"
)

// MaxRootPackedmsgBytes caps anonymous root.packedmsg fetches.
const MaxRootPackedmsgBytes int64 = 4 << 20

// rootPointerPath formats the anonymous CDN root pointer URL path.
func rootPointerPath(spaceID string) string {
	return "/" + spaceID + "/root.packedmsg"
}

// FetchRootPointer fetches and decodes root.packedmsg for a CDN Space.
// Returns nil, nil on 404 so callers can treat fresh Spaces as empty.
func FetchRootPointer(ctx context.Context, httpCli *http.Client, cdnBaseURL, spaceID string) (*cdn.CdnRootPointer, error) {
	if spaceID == "" {
		return nil, errors.New("cdn bstore: space id required")
	}
	if httpCli == nil {
		httpCli = http.DefaultClient
	}

	url := strings.TrimRight(cdnBaseURL, "/") + rootPointerPath(spaceID)
	resp, err := fetchRootPointerResponse(ctx, httpCli, url)
	if err != nil {
		return nil, errors.Wrap(err, "fetching root pointer")
	}
	defer resp.Close()

	if resp.StatusCode() == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, errors.Errorf("cdn root pointer status %d", resp.StatusCode())
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body(), MaxRootPackedmsgBytes+1))
	if err != nil {
		return nil, errors.Wrap(err, "reading root pointer body")
	}
	if int64(len(body)) > MaxRootPackedmsgBytes {
		return nil, errors.Errorf("cdn root pointer exceeds %d bytes", MaxRootPackedmsgBytes)
	}

	raw, ok := packedmsg.DecodePackedMessage(string(body))
	if !ok {
		return nil, errors.New("cdn root pointer failed packedmsg decode")
	}

	pointer := &cdn.CdnRootPointer{}
	if err := pointer.UnmarshalVT(raw); err != nil {
		return nil, errors.Wrap(err, "unmarshaling cdn root pointer")
	}
	if pointer.GetSpaceId() != spaceID {
		return nil, errors.Errorf("cdn root pointer space id mismatch: want %q got %q", spaceID, pointer.GetSpaceId())
	}
	return pointer, nil
}

type rootPointerResponse interface {
	StatusCode() int
	Body() io.Reader
	Close()
}
