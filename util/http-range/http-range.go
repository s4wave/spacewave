//go:build !js

package http_range

import (
	"context"
	"net/http"

	http_range_http "github.com/aperturerobotics/hydra/util/http-range/http"
)

// HTTPRangeReader uses HTTP requests with Range headers to implement
// io.ReadSeeker and io.ReaderAt. It is not concurrency safe.
//
// Uses net/http on native and http_range_fetch on js.
//
// The method of the request is changed to HEAD for Size().
// Call SetSize to avoid a HEAD request.
type HTTPRangeReader = http_range_http.HTTPRangeReader

// HTTPRangeReader uses HTTP requests with Range headers to implement
// io.ReadSeeker and io.ReaderAt. It is not concurrency safe.
//
// Uses net/http on native and http_range_fetch on js.
//
// The method of the request is changed to HEAD for Size().
// Call SetSize to avoid a HEAD request.
func NewHTTPRangeReader(ctx context.Context, fileUrl string, disableCache bool) (*HTTPRangeReader, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fileUrl, nil)
	if err != nil {
		return nil, err
	}

	return http_range_http.NewHTTPRangeReader(req, http.DefaultClient), nil
}
