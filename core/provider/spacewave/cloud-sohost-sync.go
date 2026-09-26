package provider_spacewave

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
)

// peerImportLock serializes peer acceptance with cloud cache publications.
// Persistence completes before either the watched state or trusted head changes.
func (h *cloudSOHost) peerImportLock(ctx context.Context, _ string) (sobject.SOStateLock, error) {
	// Resolve initial authority before acquiring the publication boundary.
	if err := h.ensureInitialState(ctx, SeedReasonColdSeed); err != nil {
		return nil, err
	}
	release, err := h.acceptMu.Lock(ctx)
	if err != nil {
		return nil, err
	}
	initial := h.stateCtr.GetValue().CloneVT()
	return sobject.NewSOStateLock(initial, func(ctx context.Context, next *sobject.SOState, changes ...*sobject.SOConfigChange) error {
		// Peer imports require durable cache storage and never call PostRoot.
		if h.persistVerifiedStateCache == nil {
			return errors.New("peer import requires durable cloud cache")
		}
		cache := h.buildVerifiedStateCache()
		if cache == nil {
			return sobject.ErrConfigHistoryUnavailable
		}
		cache.CurrentConfig = next.GetConfig().CloneVT()
		cache.VerifiedConfigChainHash = bytes.Clone(next.GetConfig().GetConfigChainHash())
		cache.VerifiedConfigChainSeqno = next.GetConfig().GetConfigChainSeqno()
		cache.PeerState = next.CloneVT()
		cache.ConfigHistory = append(cache.ConfigHistory, cloneVTSlice(changes)...)
		if err := h.persistVerifiedStateCache(ctx, cache); err != nil {
			return err
		}

		// Publish only the exact state and lineage already committed to storage.
		h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			h.verifiedConfig = cache.CurrentConfig
			h.lastConfigChainHash = cache.VerifiedConfigChainHash
			h.verifiedConfigChainSeqno = cache.VerifiedConfigChainSeqno
			h.configHistory = cache.ConfigHistory
			h.historyIndex = indexConfigHistory(cache.ConfigHistory)
			h.peerState = cache.PeerState
			h.stateCtr.SetValue(next)
			broadcast()
		})
		return nil
	}, release), nil
}

// readConfigHistory resolves a bounded suffix from retained verified cloud history.
func (h *cloudSOHost) readConfigHistory(ctx context.Context, _ string, base, target []byte) ([]*sobject.SOConfigChange, error) {
	// The acceptance lock pins the index; traversal touches only the bounded suffix.
	release, err := h.acceptMu.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return sobject.ReadConfigSuffix(ctx, base, target, func(_ context.Context, hash []byte) (*sobject.SOConfigChange, error) {
		return h.historyIndex[string(hash)].CloneVT(), nil
	})
}

// readConfigEntry resolves one entry from retained verified cloud history.
func (h *cloudSOHost) readConfigEntry(ctx context.Context, _ string, hash []byte) (*sobject.SOConfigChange, error) {
	release, err := h.acceptMu.Lock(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return h.historyIndex[string(hash)].CloneVT(), nil
}

// stateWithVerifiedConfig preserves the accepted root while fencing capabilities
// and queued work that no longer have authority in the verified configuration.
func (h *cloudSOHost) stateWithVerifiedConfig(state *sobject.SOState, config *sobject.SharedObjectConfig) *sobject.SOState {
	if state == nil {
		return nil
	}
	next := state.CloneVT()
	next.Config = config.CloneVT()
	participants := config.GetParticipants()
	readable := make(map[string]bool, len(participants))
	for _, participant := range participants {
		readable[participant.GetPeerId()] = sobject.CanReadState(participant.GetRole())
	}

	// Local revocation fences all content capabilities before notification.
	if !readable[h.peerID.String()] {
		next.RootGrants = nil
		next.Ops = nil
		next.OpRejections = nil
		next.QueuedAccountNonces = nil
		return next
	}

	// Retain only grants and outcomes still signed by allowed participants.
	grants := next.RootGrants
	next.RootGrants = nil
	for _, grant := range grants {
		if readable[grant.GetPeerId()] && grant.ValidateSignature(h.soID, participants) == nil {
			next.RootGrants = append(next.RootGrants, grant)
		}
	}
	rejections := next.OpRejections
	next.OpRejections = nil
	for _, group := range rejections {
		pending := group.Rejections
		group.Rejections = nil
		for _, rejection := range pending {
			if _, err := rejection.ValidateSignature(h.soID, participants); err == nil {
				group.Rejections = append(group.Rejections, rejection)
			}
		}
		if len(group.Rejections) != 0 {
			next.OpRejections = append(next.OpRejections, group)
		}
	}

	// Rebuild derived nonce state from the remaining signed, admissible operations.
	operations := next.Ops
	next.Ops = nil
	next.QueuedAccountNonces = nil
	for _, operation := range operations {
		_ = next.QueueOperation(h.soID, operation)
	}
	return next
}

// indexConfigHistory builds a lookup at acceptance time, avoiding full-history scans
// on peer requests. Corrupt retained entries remain unavailable for catch-up.
func indexConfigHistory(entries []*sobject.SOConfigChange) map[string]*sobject.SOConfigChange {
	index := make(map[string]*sobject.SOConfigChange, len(entries))
	for _, entry := range entries {
		hash, err := sobject.HashSOConfigChange(entry)
		if err == nil {
			index[string(hash)] = entry
		}
	}
	return index
}
