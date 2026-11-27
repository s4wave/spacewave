package provider_spacewave

import (
	"bytes"
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
)

// peerImportLock serializes peer acceptance with cloud cache publications.
// Persistence completes before either the watched state or trusted head
// changes.
func (h *cloudSOHost) peerImportLock(ctx context.Context, _ string) (sobject.SOStateLock, error) {
	// Resolve initial authority before acquiring the publication boundary.
	if err := h.ensureInitialState(ctx, SeedReasonColdSeed); err != nil {
		return nil, err
	}
	release, err := h.acceptMu.Lock(ctx)
	if err != nil {
		return nil, err
	}
	initial := h.stateCtr.GetValue().Clone()
	return sobject.NewSOStateLock(initial, func(ctx context.Context, next *sobject.SOState, changes ...*sobject.SOConfigChange) error {
		// Peer imports require durable cache storage and never call PostRoot.
		if h.persistVerifiedStateCache == nil {

			return errors.New("peer import requires durable cloud cache")
		}

		// Build the verified cache and record the accepted configuration in it.
		cache := h.buildVerifiedStateCache()
		if cache == nil {
			return sobject.ErrConfigHistoryUnavailable
		}

		// Record the accepted config and chain lineage, and retain the new changes.
		cache.CurrentConfig = next.GetConfig().CloneVT()
		cache.VerifiedConfigChainHash = bytes.Clone(next.GetConfig().GetConfigChainHash())
		cache.VerifiedConfigChainSeqno = next.GetConfig().GetConfigChainSeqno()
		cache.PeerState = next.Clone()
		if err := h.persistVerifiedStateCache(ctx, cache, changes); err != nil {
			return err
		}

		// Publish the committed cache fields and state under the broadcast lock.
		h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			// Commit the accepted config and lineage into the host state.
			h.verifiedConfig = cache.CurrentConfig
			h.lastConfigChainHash = cache.VerifiedConfigChainHash
			h.verifiedConfigChainSeqno = cache.VerifiedConfigChainSeqno
			h.peerState = cache.PeerState
			h.stateCtr.SetValue(next)
			broadcast()
		})
		return nil
	}, release), nil
}

// readConfigHistory resolves a bounded suffix from retained verified cloud
// history. Retained entries are immutable and written with the state that
// accepted them, so the traversal reads them without the acceptance lock.
func (h *cloudSOHost) readConfigHistory(ctx context.Context, soID string, base, target []byte) ([]*sobject.SOConfigChange, error) {
	return sobject.ReadConfigSuffix(ctx, base, target, func(ctx context.Context, hash []byte) (*sobject.SOConfigChange, error) {
		return h.readConfigEntry(ctx, soID, hash)
	})
}

// readConfigEntry resolves one entry from retained verified cloud history,
// returning nil when the entry is not retained.
func (h *cloudSOHost) readConfigEntry(ctx context.Context, _ string, hash []byte) (*sobject.SOConfigChange, error) {
	if h.readRetainedConfigChange == nil {
		return nil, nil
	}
	return h.readRetainedConfigChange(ctx, hash)
}

// stateWithVerifiedConfig preserves the accepted checkpoint and operations
// under the verified configuration, keeping key grants only for participants
// that can still read.
func (h *cloudSOHost) stateWithVerifiedConfig(state *sobject.SOState, config *sobject.SharedObjectConfig) *sobject.SOState {
	// A nil state stays nil.
	if state == nil {
		return nil
	}

	// Clone the state and compute read access for each participant.
	next := state.Clone()
	next.Config = config.CloneVT()
	participants := config.GetParticipants()
	readable := make(map[string]bool, len(participants))
	for _, participant := range participants {
		readable[participant.GetPeerId()] = sobject.CanReadState(participant.GetRole())
	}

	// Local revocation fences all content capabilities before notification.
	if !readable[h.peerID.String()] {
		next.KeyEpochs = nil
		next.Ops = nil
		return next
	}

	// Retain only the grants of readers.
	for _, epoch := range next.GetKeyEpochs() {
		epoch.Grants = slices.DeleteFunc(epoch.Grants, func(grant *sobject.SOGrant) bool {
			return !readable[grant.GetPeerId()]
		})
	}
	return next
}
