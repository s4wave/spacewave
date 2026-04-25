package sobject

import (
	"strings"

	"github.com/pkg/errors"
)

// SharedObjectHealthError exposes a SharedObjectHealth snapshot through an error wrapper.
type SharedObjectHealthError interface {
	error

	// GetSharedObjectHealth returns the attached SharedObjectHealth snapshot.
	GetSharedObjectHealth() *SharedObjectHealth
}

type sharedObjectHealthError struct {
	health *SharedObjectHealth
	cause  error
}

// Error returns the wrapped error string.
func (e *sharedObjectHealthError) Error() string {
	if e.cause != nil {
		return e.cause.Error()
	}
	if e.health != nil {
		return e.health.GetError()
	}
	return ""
}

// Unwrap returns the wrapped cause.
func (e *sharedObjectHealthError) Unwrap() error {
	return e.cause
}

// GetSharedObjectHealth returns the attached SharedObjectHealth snapshot.
func (e *sharedObjectHealthError) GetSharedObjectHealth() *SharedObjectHealth {
	return e.health
}

// BuildSharedObjectHealthFromError maps a known error into a SharedObjectHealth snapshot.
func BuildSharedObjectHealthFromError(
	layer SharedObjectHealthLayer,
	err error,
) *SharedObjectHealth {
	if err == nil {
		return NewSharedObjectLoadingHealth(layer)
	}

	hint := SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_NONE
	reason := SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_UNKNOWN
	msg := err.Error()
	lmsg := strings.ToLower(msg)

	switch {
	case errors.Is(err, ErrSharedObjectNotFound):
		reason = SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_NOT_FOUND
		hint = SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_CONTACT_OWNER
	case errors.Is(err, ErrNotParticipant):
		reason = SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_ACCESS_REVOKED
		hint = SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REQUEST_ACCESS
	case errors.Is(err, ErrEmptyTransformConfig) || strings.Contains(lmsg, "transform config"):
		reason = SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_TRANSFORM_CONFIG_DECODE_FAILED
		hint = SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA
	case errors.Is(err, ErrEmptyBodyType) || strings.Contains(lmsg, "unsupported shared object type"):
		reason = SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BODY_CONFIG_DECODE_FAILED
		hint = SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA
	case strings.Contains(lmsg, "block not found"):
		reason = SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BLOCK_NOT_FOUND
		hint = SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA
	}

	return NewSharedObjectClosedHealth(layer, reason, hint, msg)
}

// GetSharedObjectHealthFromError returns an attached SharedObjectHealth snapshot when available.
func GetSharedObjectHealthFromError(err error) (*SharedObjectHealth, bool) {
	var healthErr SharedObjectHealthError
	if !errors.As(err, &healthErr) {
		return nil, false
	}
	return healthErr.GetSharedObjectHealth(), true
}

// WrapSharedObjectHealthError wraps err with an attached SharedObjectHealth snapshot.
func WrapSharedObjectHealthError(
	layer SharedObjectHealthLayer,
	err error,
) error {
	if err == nil {
		return nil
	}
	return &sharedObjectHealthError{
		health: BuildSharedObjectHealthFromError(layer, err),
		cause:  err,
	}
}

// _ is a type assertion
var _ SharedObjectHealthError = (*sharedObjectHealthError)(nil)
