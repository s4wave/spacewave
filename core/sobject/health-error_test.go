package sobject

import (
	"testing"

	"github.com/pkg/errors"
)

func TestBuildSharedObjectHealthFromErrorNil(t *testing.T) {
	t.Parallel()

	health := BuildSharedObjectHealthFromError(
		SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		nil,
	)
	if health.GetStatus() != SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_LOADING {
		t.Fatalf("expected loading health, got %v", health.GetStatus())
	}
	if health.GetCommonReason() != SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_UNKNOWN {
		t.Fatalf("expected unknown common reason, got %v", health.GetCommonReason())
	}
	if health.GetRemediationHint() != SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_NONE {
		t.Fatalf("expected no remediation hint, got %v", health.GetRemediationHint())
	}
	if health.GetError() != "" {
		t.Fatalf("expected empty error detail, got %q", health.GetError())
	}
}

func TestBuildSharedObjectHealthFromErrorKnownReasons(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		err    error
		reason SharedObjectHealthCommonReason
		hint   SharedObjectHealthRemediationHint
		detail string
	}{
		{
			name:   "not found",
			err:    ErrSharedObjectNotFound,
			reason: SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_NOT_FOUND,
			hint:   SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_CONTACT_OWNER,
			detail: ErrSharedObjectNotFound.Error(),
		},
		{
			name:   "access revoked",
			err:    ErrNotParticipant,
			reason: SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_ACCESS_REVOKED,
			hint:   SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REQUEST_ACCESS,
			detail: ErrNotParticipant.Error(),
		},
		{
			name:   "transform config",
			err:    ErrEmptyTransformConfig,
			reason: SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_TRANSFORM_CONFIG_DECODE_FAILED,
			hint:   SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA,
			detail: ErrEmptyTransformConfig.Error(),
		},
		{
			name:   "body config",
			err:    ErrEmptyBodyType,
			reason: SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BODY_CONFIG_DECODE_FAILED,
			hint:   SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA,
			detail: ErrEmptyBodyType.Error(),
		},
		{
			name:   "block missing",
			err:    errors.New("build cdn world engine: block not found"),
			reason: SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BLOCK_NOT_FOUND,
			hint:   SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA,
			detail: "build cdn world engine: block not found",
		},
		{
			name:   "unsupported type",
			err:    errors.New("unsupported shared object type: weird.body"),
			reason: SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BODY_CONFIG_DECODE_FAILED,
			hint:   SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA,
			detail: "unsupported shared object type: weird.body",
		},
		{
			name:   "unknown preserved",
			err:    errors.New("local mount failed: disk offline"),
			reason: SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_UNKNOWN,
			hint:   SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_NONE,
			detail: "local mount failed: disk offline",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			health := BuildSharedObjectHealthFromError(
				SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
				tc.err,
			)
			if health.GetStatus() != SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_CLOSED {
				t.Fatalf("expected closed health, got %v", health.GetStatus())
			}
			if health.GetLayer() != SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY {
				t.Fatalf("expected body layer, got %v", health.GetLayer())
			}
			if health.GetCommonReason() != tc.reason {
				t.Fatalf("expected common reason %v, got %v", tc.reason, health.GetCommonReason())
			}
			if health.GetRemediationHint() != tc.hint {
				t.Fatalf("expected remediation hint %v, got %v", tc.hint, health.GetRemediationHint())
			}
			if health.GetError() != tc.detail {
				t.Fatalf("expected detail %q, got %q", tc.detail, health.GetError())
			}
		})
	}
}

func TestWrapSharedObjectHealthErrorPreservesHealth(t *testing.T) {
	t.Parallel()

	cause := errors.New("build cdn world engine: block not found")
	err := WrapSharedObjectHealthError(
		SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
		cause,
	)
	if err == nil {
		t.Fatal("expected wrapped error")
	}
	if !errors.Is(err, cause) {
		t.Fatal("expected wrapped error to match original cause")
	}

	health, ok := GetSharedObjectHealthFromError(err)
	if !ok {
		t.Fatal("expected wrapped health error")
	}
	if health.GetLayer() != SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY {
		t.Fatalf("expected body layer, got %v", health.GetLayer())
	}
	if health.GetCommonReason() != SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BLOCK_NOT_FOUND {
		t.Fatalf("expected block-not-found reason, got %v", health.GetCommonReason())
	}
	if health.GetError() != cause.Error() {
		t.Fatalf("expected detail %q, got %q", cause.Error(), health.GetError())
	}
}
