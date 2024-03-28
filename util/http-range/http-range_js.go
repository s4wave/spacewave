package http_range

import (
	"context"

	http_range_fetch "github.com/aperturerobotics/hydra/util/http-range/fetch"
	fetch "github.com/aperturerobotics/hydra/util/js-fetch"
)

// HTTPRangeReader uses HTTP requests with Range headers to implement
// io.ReadSeeker and io.ReaderAt. It is not concurrency safe.
//
// Uses net/http on native and http_range_fetch on js.
//
// The method of the request is changed to HEAD for Size().
// Call SetSize to avoid a HEAD request.
type HTTPRangeReader = http_range_fetch.FetchRangeReader

// HTTPRangeReader uses HTTP requests with Range headers to implement
// io.ReadSeeker and io.ReaderAt. It is not concurrency safe.
//
// Uses net/http on native and http_range_fetch on js.
//
// The method of the request is changed to HEAD for Size().
// Call SetSize to avoid a HEAD request.
func NewHTTPRangeReader(ctx context.Context, fileUrl string, disableCache bool) (*HTTPRangeReader, error) {
	opts := &fetch.Opts{
		Signal: ctx,
	}
	if disableCache {
		opts.Cache = "no-store"
	}
	return http_range_fetch.NewFetchRangeReader(fileUrl, opts), nil
}
