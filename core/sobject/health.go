package sobject

import (
	"slices"
	"strings"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

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
	next := h.edit()
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
	next := h.edit()
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
	next := h.edit()
	if !required {
		next.SyncRecoveryPeerIds = slices.DeleteFunc(next.SyncRecoveryPeerIds, func(id string) bool { return id == peerID })
		return next
	}
	next.SyncRecoveryPeerIds = append(next.SyncRecoveryPeerIds, peerID)
	slices.Sort(next.SyncRecoveryPeerIds)
	return next
}

// WithSyncPeerConnection returns a snapshot recording that the sync stream with
// an authenticated participant started, or ended with err, at now. An
// unchanged connection and error keep the snapshot and its time.
func (h *SharedObjectHealth) WithSyncPeerConnection(peerID string, connected bool, err error, now time.Time) *SharedObjectHealth {
	// Keep the snapshot when the peer's state is unchanged.
	var errText string
	if err != nil {
		errText = err.Error()
	}
	peers := h.GetSyncPeers()
	i, found := slices.BinarySearchFunc(peers, peerID, func(p *SOSyncPeer, id string) int {
		return strings.Compare(p.GetPeerId(), id)
	})
	if found && peers[i].GetConnected() == connected && peers[i].GetError() == errText {
		return h
	}

	// Copy the snapshot with the peer's new state.
	next := h.edit()
	syncPeer := &SOSyncPeer{PeerId: peerID, Connected: connected, Since: timestamppb.New(now), Error: errText}
	if found {
		next.SyncPeers[i] = syncPeer
		return next
	}
	next.SyncPeers = slices.Insert(next.SyncPeers, i, syncPeer)
	return next
}

// WithRejectedEdits returns a snapshot listing edits as this device's edits that
// replay no longer applies.
func (h *SharedObjectHealth) WithRejectedEdits(edits []*SORejectedEdit) *SharedObjectHealth {
	// Keep the snapshot when the edits are unchanged.
	if slices.EqualFunc(h.GetRejectedEdits(), edits, (*SORejectedEdit).EqualVT) {
		return h
	}

	// Copy the snapshot with the new edits.
	next := h.edit()
	next.RejectedEdits = edits
	return next
}

// WithCheckpointMismatch returns a snapshot reporting mismatch, or no mismatch
// when it is nil.
func (h *SharedObjectHealth) WithCheckpointMismatch(mismatch *SOCheckpointMismatch) *SharedObjectHealth {
	// Keep the snapshot when the mismatch is unchanged.
	if h.GetCheckpointMismatch().EqualVT(mismatch) {
		return h
	}

	// Copy the snapshot with the new mismatch.
	next := h.edit()
	next.CheckpointMismatch = mismatch
	return next
}

// WithUnorderedCount returns a snapshot reporting n operations the sequencer
// has not placed yet.
func (h *SharedObjectHealth) WithUnorderedCount(n uint32) *SharedObjectHealth {
	// Keep the snapshot when the count is unchanged.
	if h.GetUnorderedCount() == n {
		return h
	}

	// Copy the snapshot with the new count.
	next := h.edit()
	next.UnorderedCount = n
	return next
}

// edit returns a copy of h to change, or a loading snapshot when h is nil.
func (h *SharedObjectHealth) edit() *SharedObjectHealth {
	if h == nil {
		return NewSharedObjectLoadingHealth(SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT)
	}
	return h.CloneVT()
}

// NewSharedObjectLoadingHealth constructs a loading SharedObjectHealth snapshot.
func NewSharedObjectLoadingHealth(layer SharedObjectHealthLayer) *SharedObjectHealth {
	return &SharedObjectHealth{
		Status:          SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_LOADING,
		Layer:           layer,
		RemediationHint: SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_NONE,
	}
}

// NewSharedObjectReadyHealth constructs a ready SharedObjectHealth snapshot.
func NewSharedObjectReadyHealth(layer SharedObjectHealthLayer) *SharedObjectHealth {
	return &SharedObjectHealth{
		Status:          SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_READY,
		Layer:           layer,
		RemediationHint: SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_NONE,
	}
}

// NewSharedObjectClosedHealth constructs a closed SharedObjectHealth snapshot.
func NewSharedObjectClosedHealth(
	layer SharedObjectHealthLayer,
	commonReason SharedObjectHealthCommonReason,
	remediationHint SharedObjectHealthRemediationHint,
	errText string,
) *SharedObjectHealth {
	return &SharedObjectHealth{
		Status:          SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_CLOSED,
		Layer:           layer,
		CommonReason:    commonReason,
		RemediationHint: remediationHint,
		Error:           errText,
	}
}

// WithStoreDamage shows err, a store refusal replay could not heal, as a
// degraded body whose block is missing, or, when err is nil, returns a body
// it showed degraded that way to ready. A loading or closed snapshot keeps its
// status: the mount's own state comes first.
func (h *SharedObjectHealth) WithStoreDamage(err error) *SharedObjectHealth {
	// Find whether the snapshot shows store damage now.
	shown := h.GetStatus() == SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_DEGRADED &&
		h.GetLayer() == SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY &&
		h.GetCommonReason() == SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BLOCK_NOT_FOUND
	ready := h.GetStatus() == SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_READY

	// Return a healed body to ready.
	if err == nil {
		if !shown {
			return h
		}
		next := h.edit()
		next.Status = SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_READY
		next.CommonReason = SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_UNKNOWN
		next.RemediationHint = SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_NONE
		next.Error = ""
		return next
	}

	// Show the damage on a ready body, or a changed one.
	if !ready && (!shown || h.GetError() == err.Error()) {
		return h
	}
	next := h.edit()
	next.Status = SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_DEGRADED
	next.Layer = SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY
	next.CommonReason = SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_BLOCK_NOT_FOUND
	next.RemediationHint = SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_REPAIR_SOURCE_DATA
	next.Error = err.Error()
	return next
}
