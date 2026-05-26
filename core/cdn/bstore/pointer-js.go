//go:build js

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

func fetchRootPointerResponse(ctx context.Context, _ *http.Client, url string) (rootPointerResponse, error) {
	resp, err := fetch.Fetch(url, &fetch.Opts{
		Method: http.MethodGet,
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

func (r jsRootPointerResponse) Body() io.Reader {
	return r.resp.Body
}

func (r jsRootPointerResponse) Close() {
	if r.resp.Body == nil {
		return
	}
	_ = r.resp.Body.Close()
}
