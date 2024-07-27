//go:build !js

package http

import (
	"context"
	"io"
	"net/http"

	httplog "github.com/aperturerobotics/util/httplog"
	"github.com/sirupsen/logrus"
)

// Client is the http client type (struct).
type Client = http.Client

// Request is the http request type (struct).
type Request = http.Request

// Response is the http response type.
type Response = http.Response

// DefaultClient is the default client.
var DefaultClient *Client = http.DefaultClient

// NewRequest constructs a new http request.
func NewRequest(method, url string, body io.Reader) (*Request, error) {
	return http.NewRequest(method, url, body)
}

// NewRequestWithContext constructs a new http request with a context.
func NewRequestWithContext(ctx context.Context, method, url string, body io.Reader) (*Request, error) {
	return http.NewRequestWithContext(ctx, method, url, body)
}

// DoRequest performs a request with logging.
//
// If verbose=true, logs successful cases as well as errors.
// le can be nil to disable logging
func DoRequest(le *logrus.Entry, client *Client, req *Request, verbose bool) (*Response, error) {
	return httplog.DoRequest(le, client, req, verbose)
}
