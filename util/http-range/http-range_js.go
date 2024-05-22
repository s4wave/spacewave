//go:build js

package http_range

import (
	"context"

	fetch "github.com/aperturerobotics/bifrost/util/js-fetch"
	http_range_fetch "github.com/aperturerobotics/hydra/util/http-range/fetch"
	"github.com/sirupsen/logrus"
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
//
// if le is set, requests will be logged
// verbose logs successful as well as errored http requests
func NewHTTPRangeReader(
	ctx context.Context,
	le *logrus.Entry,
	fileUrl string,
	headers map[string]string,
	disableCache,
	verbose bool,
) (*HTTPRangeReader, error) {
	opts := &fetch.Opts{
		Signal:  ctx,
		Headers: headers,
	}
	if disableCache {
		opts.Cache = "no-store"
	}
	return http_range_fetch.NewFetchRangeReader(le, fileUrl, opts, verbose), nil
}
