package clouderror

import (
	"net/http"
	"testing"
	"time"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestRetryDelayPrefersStructuredRetryAfter(t *testing.T) {
	body, err := (&api.ErrorResponse{
		Code:              "temporary_unavailable",
		Message:           "retry later",
		Retryable:         true,
		RetryAfterSeconds: 3,
	}).MarshalJSON()
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}
	err = ParseResponse(&http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     make(http.Header),
	}, body)

	delay := RetryDelay(err, 500*time.Millisecond)
	if delay != 3*time.Second {
		t.Fatalf("retry delay: got %s, want %s", delay, 3*time.Second)
	}
}

func TestRetryDelayPrefersHTTPRetryAfter(t *testing.T) {
	delay := RetryDelay(ParseResponse(&http.Response{
		StatusCode: http.StatusTooManyRequests,
		Header: http.Header{
			"Retry-After": []string{"4"},
		},
	}, nil), 500*time.Millisecond)
	if delay != 4*time.Second {
		t.Fatalf("retry delay: got %s, want %s", delay, 4*time.Second)
	}
}

func TestRetryDelayUsesLongerRetryAfterHint(t *testing.T) {
	body, err := (&api.ErrorResponse{
		Code:              "temporary_unavailable",
		Message:           "retry later",
		Retryable:         true,
		RetryAfterSeconds: 2,
	}).MarshalJSON()
	if err != nil {
		t.Fatalf("marshal error response: %v", err)
	}

	delay := RetryDelay(ParseResponse(&http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header: http.Header{
			"Retry-After": []string{"6"},
		},
	}, body), 500*time.Millisecond)
	if delay != 6*time.Second {
		t.Fatalf("retry delay: got %s, want %s", delay, 6*time.Second)
	}
}

func TestRetryDelayKeepsLongerLocalBackoff(t *testing.T) {
	delay := RetryDelay(&Error{
		StatusCode:        503,
		Code:              "temporary_unavailable",
		Message:           "retry later",
		Retryable:         true,
		RetryAfterSeconds: 1,
	}, 5*time.Second)
	if delay != 5*time.Second {
		t.Fatalf("retry delay: got %s, want %s", delay, 5*time.Second)
	}
}
