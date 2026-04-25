package sobject

// NewSharedObjectLoadingHealth constructs a loading SharedObjectHealth snapshot.
func NewSharedObjectLoadingHealth(
	layer SharedObjectHealthLayer,
) *SharedObjectHealth {
	return NewSharedObjectHealth(
		SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_LOADING,
		layer,
		SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_UNKNOWN,
		SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_NONE,
		"",
	)
}

// NewSharedObjectReadyHealth constructs a ready SharedObjectHealth snapshot.
func NewSharedObjectReadyHealth(
	layer SharedObjectHealthLayer,
) *SharedObjectHealth {
	return NewSharedObjectHealth(
		SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_READY,
		layer,
		SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_UNKNOWN,
		SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_NONE,
		"",
	)
}

// NewSharedObjectClosedHealth constructs a closed SharedObjectHealth snapshot.
func NewSharedObjectClosedHealth(
	layer SharedObjectHealthLayer,
	commonReason SharedObjectHealthCommonReason,
	remediationHint SharedObjectHealthRemediationHint,
	errText string,
) *SharedObjectHealth {
	return NewSharedObjectHealth(
		SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_CLOSED,
		layer,
		commonReason,
		remediationHint,
		errText,
	)
}

// NewSharedObjectHealth constructs a SharedObjectHealth snapshot.
func NewSharedObjectHealth(
	status SharedObjectHealthStatus,
	layer SharedObjectHealthLayer,
	commonReason SharedObjectHealthCommonReason,
	remediationHint SharedObjectHealthRemediationHint,
	errText string,
) *SharedObjectHealth {
	return &SharedObjectHealth{
		Status:          status,
		Layer:           layer,
		CommonReason:    commonReason,
		RemediationHint: remediationHint,
		Error:           errText,
	}
}
