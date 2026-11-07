package publish

import (
	"context"
	"io"
	"net/url"
	"syscall"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/clouderror"
)

// timeoutError is a network timeout.
type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// TestRetryableSyncPushError retries transient answers and lost connections
// by type, and never because an error's text, such as a URL or pack ID,
// holds a status code's digits.
func TestRetryableSyncPushError(t *testing.T) {
	put := func(err error) error {
		return errors.Wrap(&url.Error{Op: "Put", URL: "http://127.0.0.1:50023/b/s/packs/50/500504.kvf", Err: err}, "sync push upload")
	}
	for name, c := range map[string]struct {
		err   error
		retry bool
	}{
		"service 503":             {&clouderror.Error{StatusCode: 503, Code: "unavailable"}, true},
		"service 429":             {errors.Wrap(&clouderror.Error{StatusCode: 429, Code: "rate_limited"}, "sync push"), true},
		"service marks retry":     {&clouderror.Error{StatusCode: 409, Retryable: true}, true},
		"service 403":             {&clouderror.Error{StatusCode: 403, Code: "forbidden", Message: "pack 500 refused"}, false},
		"storage 502":             {&clouderror.Error{StatusCode: 502, Code: "storage_refused"}, true},
		"timeout":                 {put(timeoutError{}), true},
		"connection reset":        {put(syscall.ECONNRESET), true},
		"truncated body":          {put(io.ErrUnexpectedEOF), true},
		"failure with 500 in URL": {put(errors.New("cut")), false},
		"canceled":                {put(context.Canceled), false},
	} {
		if got := isRetryableSyncPushError(c.err); got != c.retry {
			t.Errorf("%s: retry %v, want %v", name, got, c.retry)
		}
	}
}
