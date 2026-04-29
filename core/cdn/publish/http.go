package publish

import (
	"context"
	"io"
	"net/http"

	"github.com/pkg/errors"
)

// MaxRootPackedmsgBytes is the maximum public root pointer response size.
const MaxRootPackedmsgBytes = 1 << 20

// FetchBytesStatus fetches at most maxBytes from url and returns the HTTP status.
func FetchBytesStatus(ctx context.Context, url string, maxBytes int64) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, errors.Wrap(err, "build request")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, errors.Wrap(err, "fetch")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		return nil, resp.StatusCode, errors.Wrap(err, "read response")
	}
	if int64(len(body)) > maxBytes {
		return nil, resp.StatusCode, errors.New("response exceeds size limit")
	}
	return body, resp.StatusCode, nil
}
