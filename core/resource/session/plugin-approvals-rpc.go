package resource_session

import (
	"context"

	"github.com/pkg/errors"
	plugin_approval "github.com/s4wave/spacewave/core/plugin/approval"
	s4wave_plugin_approval "github.com/s4wave/spacewave/sdk/plugin/approval"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// SetPluginApproval records the session's decision about one plugin manifest
// root in device-local state. Only ALLOWED lets the root run.
func (r *SessionResource) SetPluginApproval(
	ctx context.Context,
	req *s4wave_session.SetPluginApprovalRequest,
) (*s4wave_session.SetPluginApprovalResponse, error) {
	// Look up the device-local approvals.
	approvals, ref, err := plugin_approval.ExLookupPluginApprovals(ctx, r.b)
	if err != nil {
		return nil, err
	}
	defer ref.Release()

	// Record the decision for the session's peer.
	err = approvals.Set(ctx, r.pluginApprovalPeer(), &s4wave_plugin_approval.PluginApproval{
		PluginId:     req.GetPluginId(),
		ManifestRoot: req.GetManifestRoot(),
		State:        req.GetState(),
		Name:         req.GetName(),
		Version:      req.GetVersion(),
	})
	if err != nil {
		return nil, err
	}
	return &s4wave_session.SetPluginApprovalResponse{}, nil
}

// WatchPluginApprovals streams the session's plugin decisions: every decision
// first with snapshot set, then only the decisions each write added or changed.
// Decisions are never removed.
func (r *SessionResource) WatchPluginApprovals(
	_ *s4wave_session.WatchPluginApprovalsRequest,
	strm s4wave_session.SRPCSessionResourceService_WatchPluginApprovalsStream,
) error {
	// Look up the device-local approvals.
	ctx := strm.Context()
	approvals, ref, err := plugin_approval.ExLookupPluginApprovals(ctx, r.b)
	if err != nil {
		return err
	}
	defer ref.Release()

	// Refuse a session with no peer, which owns no decisions.
	peerID := r.pluginApprovalPeer()
	if peerID == "" {
		return errors.New("session has no peer id")
	}

	// Rescan after each committed write, sending what differs from the last
	// scan.
	prev := make(map[string]*s4wave_plugin_approval.PluginApproval)
	for snapshot := true; ; snapshot = false {
		// Take the wait channel before the scan so a write during the scan
		// wakes the next one.
		waitCh := approvals.WaitCh()

		// Collect the decisions added or changed since the last scan.
		resp := &s4wave_session.WatchPluginApprovalsResponse{Snapshot: snapshot}
		err := approvals.Scan(ctx, peerID, func(approval *s4wave_plugin_approval.PluginApproval) error {
			// Skip a decision the last scan already sent.
			key := string(plugin_approval.Key(peerID, approval.GetPluginId(), approval.GetManifestRoot()))
			if old := prev[key]; old != nil && old.EqualVT(approval) {
				return nil
			}

			// Remember and send the new decision.
			prev[key] = approval
			resp.Approvals = append(resp.Approvals, approval)
			return nil
		})
		if err != nil {
			return err
		}

		// Send the snapshot, or the changes when there are any.
		if snapshot || len(resp.GetApprovals()) != 0 {
			if err := strm.Send(resp); err != nil {
				return err
			}
		}

		// Wait for the next committed write.
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-waitCh:
		}
	}
}

// pluginApprovalPeer returns the peer that owns the session's plugin
// decisions, or empty when the session has none, which refuses every root.
func (r *SessionResource) pluginApprovalPeer() string {
	peerID := r.session.GetPeerId()
	if len(peerID) == 0 {
		return ""
	}
	return peerID.String()
}
