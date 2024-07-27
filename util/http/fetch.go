//go:build js

package http

import (
	"context"
	"io"
	"net/http"
	"net/url"

	"github.com/aperturerobotics/bldr/util/cloneurl"
	httplog_fetch "github.com/aperturerobotics/util/httplog/fetch"
	fetch "github.com/aperturerobotics/util/js/fetch"
	"github.com/sirupsen/logrus"
)

// Opts are common fetch options.
type Opts = fetch.CommonOpts

// Client is the http client type (struct).
//
// Values set on the Request override values set on Client.
type Client struct {
	Opts
}

// Request is the http request type (struct).
type Request struct {
	fetch.Opts

	// URL specifies the URL to access.
	URL *url.URL
}

// Response is the http response type.
type Response = fetch.Response

// DefaultClient is the default client.
var DefaultClient *Client = &Client{}

// NewRequest constructs a new http request.
func NewRequest(method, urlStr string, body io.Reader) (*Request, error) {
	var req http.Request
	req.Context()
	urlo, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}

	return &Request{
		Opts: fetch.Opts{
			Method: method,
		},
		URL: urlo,
	}, nil
}

// NewRequestWithContext constructs a new http request with a context.
func NewRequestWithContext(ctx context.Context, method, url string, body io.Reader) (*Request, error) {
	req, err := NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Opts.Signal = ctx
	return req, nil
}

// DoRequest performs a request with logging.
//
// If verbose=true, logs successful cases as well as errors.
// le can be nil to disable logging
func DoRequest(le *logrus.Entry, client *Client, req *Request, verbose bool) (*Response, error) {
	var urlStr string
	var opts fetch.Opts
	if req != nil {
		opts = req.Opts
		if req.URL != nil {
			urlStr = req.URL.String()
		}
	}

	return httplog_fetch.Fetch(le, urlStr, &opts, verbose)
}

// Context returns the request's context. To change the context, use
// [Request.Clone] or [Request.WithContext].
//
// The returned context is always non-nil; it defaults to the
// background context.
//
// The context controls cancelation.
func (r *Request) Context() context.Context {
	if r.Opts.Signal != nil {
		return r.Opts.Signal
	}
	return context.Background()
}

// WithContext returns a shallow copy of r with its context changed
// to ctx. The provided ctx must be non-nil.
//
// For outgoing client request, the context controls the entire
// lifetime of a request and its response: obtaining a connection,
// sending the request, and reading the response headers and body.
//
// To create a new request with a context, use [NewRequestWithContext].
// To make a deep copy of a request with a new context, use [Request.Clone].
func (r *Request) WithContext(ctx context.Context) *Request {
	if ctx == nil {
		panic("nil context")
	}
	r2 := new(Request)
	*r2 = *r
	r2.Opts.Signal = ctx
	return r2
}

// Clone returns a deep copy of r with its context changed to ctx.
// The provided ctx must be non-nil.
//
// Clone only makes a shallow copy of the Body field.
//
// The context controls the entire lifetime of a request and its response:
// obtaining a connection, sending the request, and reading the response headers
// and body.
func (r *Request) Clone(ctx context.Context) *Request {
	if ctx == nil {
		panic("nil context")
	}
	r2 := new(Request)
	*r2 = *r
	r2.Opts.Signal = ctx
	r2.URL = cloneurl.CloneURL(r.URL)
	if r.Header != nil {
		r2.Header = r.Header.Clone()
	}

	return r2
}
