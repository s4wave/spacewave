package clouderror

import (
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
)

// TestBlockedErrorBuildsTypedHealth checks that a blocked cloud code, not its
// wording, selects the blocked health and its retry action.
func TestBlockedErrorBuildsTypedHealth(t *testing.T) {
	layer := sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT
	blocked := errors.Wrap(&Error{StatusCode: http.StatusForbidden, Code: "dmca_blocked", Message: "access withdrawn"}, "mount")
	health := sobject.BuildSharedObjectHealthFromError(layer, blocked)
	if health.GetCommonReason() != sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_RESOURCE_BLOCKED ||
		health.GetRemediationHint() != sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_RETRY {
		t.Fatalf("blocked health = %v", health)
	}

	// The former trigger text without the code stays unknown with no action.
	for _, err := range []error{
		errors.New("403 dmca_blocked: resource is blocked"),
		&Error{StatusCode: http.StatusForbidden, Code: "forbidden", Message: "resource is blocked"},
	} {
		health := sobject.BuildSharedObjectHealthFromError(layer, err)
		if health.GetCommonReason() != sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_UNKNOWN ||
			health.GetRemediationHint() != sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_NONE {
			t.Fatalf("health for %q = %v", err, health)
		}
	}
}

func TestRetryAfterSecondsSaturates(t *testing.T) {
	delay := time.Duration(math.MaxUint32)*time.Second + time.Nanosecond
	if got := retryAfterSeconds(delay); got != math.MaxUint32 {
		t.Fatalf("retry-after seconds: got %d, want %d", got, math.MaxUint32)
	}
}

func TestRetryDelayPrefersStructuredRetryAfter(t *testing.T) {
	// Parse a retryable error that asks for a 3 s delay.
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

	// The structured hint outlasts the local backoff.
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

// TestParseClassifiesCodes checks that Parse never retries a permanent code,
// even when the server marks it retryable, and that each code answers exactly
// one classifier.
func TestParseClassifiesCodes(t *testing.T) {
	for _, tc := range []struct {
		code                               string
		unauth, deleted, blocked, conflict bool
	}{
		{code: "unknown_session", unauth: true},
		{code: "invalid_signature", unauth: true},
		{code: "unknown_keypair", unauth: true},
		{code: "account_not_found", deleted: true},
		{code: "invalid_peer_id", deleted: true},
		{code: "unknown_entity", deleted: true},
		{code: "dmca_blocked", blocked: true},
		{code: PackReplacementConflictCode, conflict: true},
		{code: "duplicate_connection"},
		{code: "rate_limited"},
	} {
		body, err := (&api.ErrorResponse{Code: tc.code, Message: "test", Retryable: true}).MarshalJSON()
		if err != nil {
			t.Fatalf("marshal error response: %v", err)
		}
		err = errors.Wrap(Parse(http.StatusConflict, body), "request")
		permanent := tc.unauth || tc.deleted || tc.blocked || tc.conflict
		if IsNonRetryable(err) != permanent || IsUnauth(err) != tc.unauth || IsAccountDeleted(err) != tc.deleted ||
			IsBlocked(err) != tc.blocked || IsPackReplacementConflict(err) != tc.conflict {
			t.Errorf("Parse(%q) = %v: final %v unauth %v deleted %v blocked %v conflict %v",
				tc.code, err, IsNonRetryable(err), IsUnauth(err), IsAccountDeleted(err), IsBlocked(err), IsPackReplacementConflict(err))
		}
	}

	// An error from outside the cloud answers no classifier.
	plain := errors.New("generic error")
	if IsNonRetryable(plain) || IsUnauth(plain) || IsAccountDeleted(plain) || IsBlocked(plain) || IsPackReplacementConflict(plain) {
		t.Fatal("a non-cloud error was classified")
	}
}

func TestGatewayErrorIsRetryable(t *testing.T) {
	for _, tc := range []struct {
		status int
		body   string
		want   bool
	}{
		{http.StatusBadGateway, "", true},
		{http.StatusServiceUnavailable, "<html>unavailable</html>", true},
		{http.StatusTooManyRequests, "", true},
		{http.StatusNotFound, "", false},
	} {
		err := Parse(tc.status, []byte(tc.body))
		if IsNonRetryable(err) == tc.want {
			t.Errorf("Parse(%d, %q) retryable = %v, want %v", tc.status, tc.body, !tc.want, tc.want)
		}
	}
}
