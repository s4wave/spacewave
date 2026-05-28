//go:build js

package nethttp

import (
	"io"
	"net/http"
	"slices"

	fetch "github.com/aperturerobotics/util/js/fetch"
	"github.com/pkg/errors"
)

// FetchTransport implements net/http RoundTripper over util/js/fetch.
type FetchTransport struct {
	CommonOpts fetch.CommonOpts
}

// NewFetchTransport constructs a browser fetch-backed RoundTripper.
func NewFetchTransport(opts *fetch.CommonOpts) *FetchTransport {
	t := &FetchTransport{}
	if opts != nil {
		t.CommonOpts = *opts
	}
	return t
}

// RoundTrip implements http.RoundTripper.
func (t *FetchTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, errors.New("nil request")
	}
	if req.URL == nil {
		return nil, errors.New("nil request URL")
	}

	opts := &fetch.Opts{
		CommonOpts: t.CommonOpts,
		Method:     req.Method,
		Header:     fetchHeaders(req.Header),
		Signal:     req.Context(),
	}
	if req.Body != nil {
		opts.Body = req.Body
		defer req.Body.Close()
	}

	resp, err := fetch.Fetch(req.URL.String(), opts)
	if err != nil {
		return nil, err
	}

	header := http.Header{}
	for key, values := range resp.Header {
		for _, value := range values {
			header.Add(key, value)
		}
	}
	header.Del("Content-Encoding")
	header.Del("Content-Length")
	body := resp.Body
	if body == nil {
		body = io.NopCloser(nilReader{})
	}
	return &http.Response{
		Status:        resp.Status,
		StatusCode:    resp.StatusCode,
		Header:        header,
		Body:          body,
		ContentLength: -1,
		Request:       req,
	}, nil
}

type nilReader struct{}

func (nilReader) Read([]byte) (int, error) {
	return 0, io.EOF
}

func fetchHeaders(header http.Header) fetch.Header {
	if header == nil {
		return nil
	}
	out := make(fetch.Header, len(header))
	for key, values := range header {
		out[key] = slices.Clone(values)
	}
	return out
}

// _ is a type assertion
var _ http.RoundTripper = (*FetchTransport)(nil)
