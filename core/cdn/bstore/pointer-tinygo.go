//go:build js && tinygo

package cdn_bstore

import (
	"context"
	"io"
	"net/http"

	fetch "github.com/aperturerobotics/util/js/fetch"
)

type jsRootPointerResponse struct {
	resp *fetch.Response
}

func fetchRootPointerResponse(ctx context.Context, _ *http.Client, url, etag string) (rootPointerResponse, error) {
	resp, err := fetch.Fetch(url, &fetch.Opts{
		Method: http.MethodGet,
		Header: rootPointerFetchHeader(etag),
		Signal: ctx,
	})
	if err != nil {
		return nil, err
	}
	return jsRootPointerResponse{resp: resp}, nil
}

func (r jsRootPointerResponse) StatusCode() int {
	return r.resp.StatusCode
}

func (r jsRootPointerResponse) ETag() string {
	return r.resp.Header.Get("ETag")
}

// rootPointerFetchHeader returns the request header that revalidates etag.
func rootPointerFetchHeader(etag string) fetch.Header {
	if etag == "" {
		return nil
	}
	return fetch.Header{"If-None-Match": {etag}}
}

func (r jsRootPointerResponse) Body() io.Reader {
	return r.resp.Body
}

func (r jsRootPointerResponse) Close() {
	if r.resp.Body == nil {
		return
	}
	_ = r.resp.Body.Close()
}
