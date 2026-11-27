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
	// No denial, or no participants to own one, revokes nothing.
	denied := health.GetSyncDeniedPeerIds()
	if config == nil || len(denied) == 0 {
		return false
	}

	// Index the owners once, then look for a denial among them.
	owners := make(map[string]struct{})
	for _, participant := range config.GetParticipants() {
		if IsOwner(participant.GetRole()) {
			owners[participant.GetPeerId()] = struct{}{}
		}
	}
	return slices.ContainsFunc(denied, func(id string) bool {
		_, ok := owners[id]
		return ok
	})
}

// WithSyncPeerAdmission returns a health snapshot reflecting a verified peer's
// admission response without changing local mount readiness or authority.
// The caller must have verified that peerID is readable under local authority.
func (h *SharedObjectHealth) WithSyncPeerAdmission(peerID string, accepted bool) *SharedObjectHealth {
	// Keep the snapshot when it records this decision already.
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
// denial. A newly installed authenticated checkpoint replaces the authority
// those denials answered. Subsequent sync rounds record fresh decisions.
func (h *SharedObjectHealth) WithoutSyncDenials() *SharedObjectHealth {
	if len(h.GetSyncDeniedPeerIds()) == 0 {
		return h
	}
	next := h.edit()
	next.SyncDeniedPeerIds = nil
	return next
}

// WithSyncPeerRecovery records a recovery requirement without closing local
// access. The caller supplies an authenticated participant. Only verified
// convergence clears the requirement.
func (h *SharedObjectHealth) WithSyncPeerRecovery(peerID string, required bool) *SharedObjectHealth {
	// Keep the snapshot when it records this requirement already.
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

// WithReplay returns a snapshot showing what the body driver reports of this
// device's replay: its revoked edits, a checkpoint whose state differs from
// the replayed one, the edits the sequencer has not placed, where replay
// stands, and the roots its state keeps that this device holds no copy of.
func (h *SharedObjectHealth) WithReplay(r *BodyReport) *SharedObjectHealth {
	// Keep the snapshot when the report shows nothing new.
	if slices.EqualFunc(h.GetRejectedEdits(), r.RejectedEdits, (*SORejectedEdit).EqualVT) &&
		h.GetCheckpointMismatch().EqualVT(r.CheckpointMismatch) &&
		h.GetUnorderedCount() == r.Unordered &&
		h.GetReplay().EqualVT(r.Replay) &&
		slices.EqualFunc(h.GetRootWaits(), r.RootWaits, (*SORootWait).EqualVT) {
		return h
	}

	// Copy the snapshot with the report.
	next := h.edit()
	next.RejectedEdits = r.RejectedEdits
	next.CheckpointMismatch = r.CheckpointMismatch
	next.UnorderedCount = r.Unordered
	next.Replay = r.Replay
	next.RootWaits = r.RootWaits
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
