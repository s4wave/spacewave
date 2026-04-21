//go:build !js

package http_range

import (
	"context"
	"net/http"

	http_range_http "github.com/s4wave/spacewave/db/util/http-range/http"
	"github.com/sirupsen/logrus"
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
//
// if le is set, requests will be logged
// verbose logs successful as well as errored http requests
func NewHTTPRangeReader(
	ctx context.Context,
	le *logrus.Entry,
	fileUrl string,
	headers map[string][]string,
	disableCache,
	verbose bool,
) (*HTTPRangeReader, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fileUrl, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		for _, v := range v {
			req.Header.Add(k, v)
		}
	}

	return http_range_http.NewHTTPRangeReader(le, req, http.DefaultClient, verbose), nil
}
