package clouderror

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// Error is a structured error from the Spacewave cloud API.
type Error struct {
	// StatusCode is the HTTP status code.
	StatusCode int
	// Code is the error code from the cloud.
	Code string
	// Message is the human-readable error message.
	Message string
	// Retryable indicates whether the client should retry.
	Retryable bool
	// RetryAfterSeconds is the suggested retry delay.
	RetryAfterSeconds uint32
}

// Error returns a string representation of the cloud error.
func (e *Error) Error() string {
	msg := strconv.Itoa(e.StatusCode) + " " + e.Code + ": " + e.Message
	if e.RetryAfterSeconds > 0 {
		msg += " [retry_after=" + strconv.FormatUint(uint64(e.RetryAfterSeconds), 10) + "]"
	}
	return msg
}

// unauthCodes are error codes indicating the session key is stale but the
// account still exists. These are recoverable via reauthentication.
var unauthCodes = map[string]bool{
	"unknown_session":   true,
	"invalid_signature": true,
	"unknown_keypair":   true,
}

// refreshableWriteTicketCodes are error codes indicating a write ticket is
// stale, expired, or otherwise refreshable without full session
// reauthentication. These are handled by write-ticket refresh-and-retry paths,
// not by the account deletion or session reauthentication flows.
var refreshableWriteTicketCodes = map[string]bool{
	"invalid_write_ticket":               true,
	"expired_write_ticket":               true,
	"stale_write_ticket":                 true,
	"stale_session_account_write_ticket": true,
	"stale_resource_write_ticket":        true,
}

// deletedCodes are error codes indicating the account itself is gone.
// These are permanent and trigger the account deletion cascade.
var deletedCodes = map[string]bool{
	"account_not_found": true,
	"invalid_peer_id":   true,
	"unknown_entity":    true,
}

// blockedCodes are error codes indicating a resource is blocked (e.g. DMCA
// takedown). These are permanent until manually retried by the user.
var blockedCodes = map[string]bool{
	"dmca_blocked": true,
}

// permanentCodes is the union of unauthCodes, deletedCodes, and blockedCodes.
var permanentCodes = func() map[string]bool {
	m := make(map[string]bool, len(unauthCodes)+len(deletedCodes)+len(blockedCodes))
	for k := range unauthCodes {
		m[k] = true
	}
	for k := range deletedCodes {
		m[k] = true
	}
	for k := range blockedCodes {
		m[k] = true
	}
	return m
}()

// Parse parses a cloud API error response body into an Error.
func Parse(statusCode int, body []byte) *Error {
	ce := &Error{StatusCode: statusCode}
	var resp api.ErrorResponse
	if err := resp.UnmarshalJSON(body); err == nil {
		ce.Code = resp.GetCode()
		ce.Message = resp.GetMessage()
		ce.Retryable = resp.GetRetryable()
		ce.RetryAfterSeconds = resp.GetRetryAfterSeconds()
	}
	if permanentCodes[ce.Code] {
		ce.Retryable = false
	}
	return ce
}

// ParseResponse parses a cloud API error response and retry hints.
func ParseResponse(resp *http.Response, body []byte) *Error {
	ce := Parse(resp.StatusCode, body)
	headerDelay := ParseRetryAfterHeader(resp.Header.Get("Retry-After"), time.Now())
	if headerDelay <= 0 {
		return ce
	}
	headerSeconds := uint32(headerDelay / time.Second)
	if headerDelay%time.Second != 0 {
		headerSeconds++
	}
	if headerSeconds > ce.RetryAfterSeconds {
		ce.RetryAfterSeconds = headerSeconds
	}
	return ce
}

// ParseRetryAfterHeader parses a Retry-After header as delay seconds or date.
func ParseRetryAfterHeader(header string, now time.Time) time.Duration {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0
	}
	seconds, err := strconv.ParseUint(header, 10, 32)
	if err == nil {
		return time.Duration(seconds) * time.Second
	}
	at, err := http.ParseTime(header)
	if err != nil {
		return 0
	}
	if !at.After(now) {
		return 0
	}
	return at.Sub(now)
}

// IsNonRetryable checks if an error is a non-retryable cloud error.
func IsNonRetryable(err error) bool {
	var ce *Error
	if errors.As(err, &ce) {
		return !ce.Retryable
	}
	return false
}

// IsUnauth checks if an error indicates a stale session key.
func IsUnauth(err error) bool {
	var ce *Error
	if errors.As(err, &ce) {
		return unauthCodes[ce.Code]
	}
	return false
}

// IsAccountDeleted checks if an error indicates the account is gone.
func IsAccountDeleted(err error) bool {
	var ce *Error
	if errors.As(err, &ce) {
		return deletedCodes[ce.Code]
	}
	return false
}

// IsRefreshableWriteTicket checks if an error should refresh a write ticket.
func IsRefreshableWriteTicket(err error) bool {
	var ce *Error
	if errors.As(err, &ce) {
		return refreshableWriteTicketCodes[ce.Code]
	}
	return false
}

// IsBlocked checks if an error indicates a manually retryable block.
func IsBlocked(err error) bool {
	var ce *Error
	if errors.As(err, &ce) {
		return blockedCodes[ce.Code]
	}
	return false
}

// IsAccessGated checks if a cloud error should wait for access-state
// invalidation instead of retrying.
func IsAccessGated(err error) bool {
	var ce *Error
	if !errors.As(err, &ce) {
		return false
	}
	switch ce.Code {
	case "account_not_found",
		"account_read_only",
		"dmca_blocked",
		"insufficient_role",
		"rbac_denied",
		"resource_not_found",
		"subscription_readonly",
		"subscription_required":
		return true
	}
	return false
}

// IsStatus returns true when err is a cloud error with the given HTTP status.
func IsStatus(err error, statusCode int) bool {
	var ce *Error
	return errors.As(err, &ce) && ce.StatusCode == statusCode
}

// RetryAfter returns the server-provided retry delay, if any.
func RetryAfter(err error) time.Duration {
	var ce *Error
	if !errors.As(err, &ce) || ce.RetryAfterSeconds == 0 {
		return 0
	}
	return time.Duration(ce.RetryAfterSeconds) * time.Second
}

// RetryDelay prefers the server-provided retry delay when it is longer than the
// local fallback backoff.
func RetryDelay(err error, fallback time.Duration) time.Duration {
	retryAfter := RetryAfter(err)
	if retryAfter > 0 && (fallback == cbackoff.Stop || retryAfter > fallback) {
		return retryAfter
	}
	return fallback
}
