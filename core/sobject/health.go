package sobject

import "slices"

// AuthoritativeSyncDenied reports whether a retained owner has denied this
// object identity. Denial from a non-owner cannot revoke another participant.
func AuthoritativeSyncDenied(config *SharedObjectConfig, health *SharedObjectHealth) bool {
	if config == nil || health == nil {
		return false
	}
	return slices.ContainsFunc(config.GetParticipants(), func(participant *SOParticipantConfig) bool {
		return IsOwner(participant.GetRole()) && slices.Contains(health.GetSyncDeniedPeerIds(), participant.GetPeerId())
	})
}

// WithSyncPeerAdmission returns a health snapshot reflecting a verified peer's
// admission response without changing local mount readiness or authority.
// The caller must have verified that peerID is readable under local authority.
func (h *SharedObjectHealth) WithSyncPeerAdmission(peerID string, accepted bool) *SharedObjectHealth {
	if slices.Contains(h.GetSyncDeniedPeerIds(), peerID) == !accepted {
		return h
	}

	// Retain immutable snapshots for concurrent health watchers.
	next := h.CloneVT()
	if next == nil {
		next = NewSharedObjectLoadingHealth(SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT)
	}
	if accepted {
		next.SyncDeniedPeerIds = slices.DeleteFunc(next.SyncDeniedPeerIds, func(id string) bool { return id == peerID })
		return next
	}
	next.SyncDeniedPeerIds = append(next.SyncDeniedPeerIds, peerID)
	slices.Sort(next.SyncDeniedPeerIds)
	return next
}

// WithoutSyncDenials returns a snapshot that forgets every recorded admission
// denial. A newly installed authenticated checkpoint replaces the authority those
// denials answered; later sync rounds record fresh decisions.
func (h *SharedObjectHealth) WithoutSyncDenials() *SharedObjectHealth {
	if len(h.GetSyncDeniedPeerIds()) == 0 {
		return h
	}
	next := h.CloneVT()
	next.SyncDeniedPeerIds = nil
	return next
}

// WithSyncPeerRecovery records a recovery requirement without closing local access.
// The caller supplies an authenticated participant; only verified convergence clears it.
func (h *SharedObjectHealth) WithSyncPeerRecovery(peerID string, required bool) *SharedObjectHealth {
	if slices.Contains(h.GetSyncRecoveryPeerIds(), peerID) == required {
		return h
	}

	// Keep source recovery separate from admission decisions and mount readiness.
	next := h.CloneVT()
	if next == nil {
		next = NewSharedObjectLoadingHealth(SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT)
	}
	if !required {
		next.SyncRecoveryPeerIds = slices.DeleteFunc(next.SyncRecoveryPeerIds, func(id string) bool { return id == peerID })
		return next
	}
	next.SyncRecoveryPeerIds = append(next.SyncRecoveryPeerIds, peerID)
	slices.Sort(next.SyncRecoveryPeerIds)
	return next
}

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
