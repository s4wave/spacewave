package provider_spacewave

import (
	"bytes"
	"context"
	"slices"
	"time"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
)

// cloudOpsBodyLimit is the largest operations request body the cloud accepts.
const cloudOpsBodyLimit = 256 << 10

// retainPublication commits accepted state and its cloud obligation atomically.
// The caller holds acceptMu. No watched local success precedes this transaction.
func (h *cloudSOHost) retainPublication(ctx context.Context, state *sobject.SOState, operations []*sobject.SOOperation, checkpoint bool) error {
	// Reject an operation that exceeds the cloud request limit.
	for _, op := range operations {
		if (&api.PostOpsRequest{Operations: []*sobject.SOOperation{op}}).SizeVT() > cloudOpsBodyLimit {
			return errors.New("operation exceeds the cloud request limit")
		}
	}

	// Fence the local block store before recording the obligation.
	if h.syncer != nil {
		fenced, err := h.syncer.upper.Sync(ctx)
		if err != nil {
			return err
		}
		if !fenced {
			return errors.New("local block store has no durability fence")
		}
	}
	cache := h.buildVerifiedStateCache()
	if cache == nil || h.persistVerifiedStateCache == nil {
		return errors.New("local publication requires durable verified state")
	}

	// Extend the pending publication with the operation or new checkpoint.
	pending := cache.GetPendingPublication()
	if pending == nil {
		pending = &api.PendingSOPublication{FirstPendingUnixMilli: time.Now().UnixMilli()}
	}
	pending.Operations = append(pending.Operations, cloneVTSlice(operations)...)
	if checkpoint && state.GetCheckpoint() != nil {
		pending.Checkpoint = state.GetCheckpoint().CloneVT()
		if err := dropCoveredOperations(h.soID, pending); err != nil {
			return err
		}
	}
	cache.PeerState = state.CloneVT()
	cache.PendingPublication = pending

	// Persist the accepted state and its obligation together.
	if err := h.persistVerifiedStateCache(ctx, cache); err != nil {
		return err
	}
	h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		h.peerState = cache.PeerState
		h.pending = pending
		h.stateCtr.SetValue(state)
		broadcast()
	})
	if h.syncer != nil {
		h.syncer.setPublication(h, pending)
	}
	return nil
}

// pendingPublication snapshots durable work before its block flush begins.
func (h *cloudSOHost) pendingPublication() *api.PendingSOPublication {
	var pending *api.PendingSOPublication
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { pending = h.pending.CloneVT() })
	return pending
}

// publishCheckpoint sends only work captured before the corresponding block
// fence. Newer local writes keep their own obligation through acknowledgment.
func (h *cloudSOHost) publishCheckpoint(ctx context.Context, sent *api.PendingSOPublication) error {
	// A nil publication has nothing to send.
	if sent == nil {
		return nil
	}

	// Publish an authorized checkpoint before the operation batch.
	var config *sobject.SharedObjectConfig
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { config = h.verifiedConfig.CloneVT() })
	if config == nil {
		return sobject.ErrConfigHistoryUnavailable
	}
	if sent.GetCheckpoint() != nil {
		if _, err := sent.GetCheckpoint().ValidateAuthority(h.soID, config.GetParticipants()); err != nil {
			return err
		}
		if err := h.client.PostCheckpoint(ctx, h.soID, sent.GetCheckpoint()); err != nil {
			return err
		}
	}

	// Post signed operations in bounded batches. Pending operations are held
	// in the order they were written, so each follows its author's previous
	// operation.
	var batch []*sobject.SOOperation
	for _, operation := range sent.GetOperations() {
		candidate := append(batch, operation)
		if len(candidate) > 50 || (&api.PostOpsRequest{Operations: candidate}).SizeVT() > cloudOpsBodyLimit {
			if err := h.client.PostOps(ctx, h.soID, batch); err != nil {
				return err
			}
			batch = nil
		}
		batch = append(batch, operation)
	}
	if len(batch) != 0 {
		if err := h.client.PostOps(ctx, h.soID, batch); err != nil {
			return err
		}
	}

	// A lost acknowledgment leaves the same signed request available for retry.
	release, err := h.acceptMu.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	cache := h.buildVerifiedStateCache()
	if cache == nil || h.persistVerifiedStateCache == nil {
		return sobject.ErrConfigHistoryUnavailable
	}

	// Remove acknowledged work from the durable pending publication.
	pending := cache.GetPendingPublication()
	if pending == nil {
		return nil
	}
	if pending.GetCheckpoint().EqualVT(sent.GetCheckpoint()) {
		pending.Checkpoint = nil
	}
	pending.Operations = slices.DeleteFunc(pending.Operations, func(op *sobject.SOOperation) bool {
		return slices.ContainsFunc(sent.Operations, func(other *sobject.SOOperation) bool { return op.EqualVT(other) })
	})
	if pending.GetCheckpoint() == nil && len(pending.Operations) == 0 {
		pending = nil
	}
	cache.PendingPublication = pending

	// Persist the reduced obligation and update the host state.
	if err := h.persistVerifiedStateCache(ctx, cache); err != nil {
		return err
	}
	h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		h.pending = pending
		broadcast()
	})
	if h.syncer != nil {
		h.syncer.setPublication(h, pending)
	}
	return nil
}

// setPublication projects durable obligations into the existing sync scheduler.
func (s *syncController) setPublication(h *cloudSOHost, pending *api.PendingSOPublication) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if pending == nil {
			delete(s.publications, h)
		} else {
			if s.publications == nil {
				s.publications = make(map[*cloudSOHost]time.Time)
			}
			s.publications[h] = time.UnixMilli(pending.GetFirstPendingUnixMilli())
		}
		if s.telemetry != nil {
			s.telemetry.syncTelemetry.SetPendingPublications(s.resourceID, len(s.publications))
		}
		broadcast()
	})
}

// flushCheckpoint runs under flushMtx. Snapshotting publications before blocks
// prevents a concurrent edit from publishing state whose blocks missed this
// flush.
func (s *syncController) flushCheckpoint(ctx context.Context, orderBlocks bool) (retErr error) {
	// Record sync errors in telemetry for the resource.
	if s.telemetry != nil {
		defer func() { s.telemetry.syncTelemetry.RecordError(s.resourceID, retErr) }()
	}

	// Snapshot each host's pending publication before flushing blocks.
	var hosts []*cloudSOHost
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for host := range s.publications {
			hosts = append(hosts, host)
		}
	})

	// Snapshot each host's durable obligation.
	pending := make([]*api.PendingSOPublication, len(hosts))
	for i, host := range hosts {
		pending[i] = host.pendingPublication()
	}
	if err := s.flush(ctx, orderBlocks); err != nil {
		return err
	}

	// Publish each host's checkpoint after its blocks are durable.
	for i, host := range hosts {
		if err := host.publishCheckpoint(ctx, pending[i]); err != nil {
			return err
		}
	}
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		s.compactDue = true
		broadcast()
	})
	return nil
}

// acceptCloudSnapshot keeps the cloud cursor paired with its authenticated
// base, while preserving a newer peer checkpoint and operations the cloud does
// not hold yet. The caller holds acceptMu and has verified the cloud state and
// changelog progression.
func (h *cloudSOHost) acceptCloudSnapshot(ctx context.Context, cloud *sobject.SOState, sequence uint64) error {
	// Select the higher checkpoint between the cloud snapshot and the accepted
	// state.
	previous := h.stateCtr.GetValue()
	cloudCheckpoint, err := cloud.GetCheckpointInner()
	if err != nil {
		return err
	}
	previousCheckpoint, err := previous.GetCheckpointInner()
	if err != nil {
		return err
	}
	next, other := cloud.CloneVT(), previous
	switch {
	case previousCheckpoint.GetHeight() > cloudCheckpoint.GetHeight():
		next, other = h.stateWithVerifiedConfig(previous, cloud.GetConfig()), cloud
	case previousCheckpoint != nil && previousCheckpoint.GetHeight() == cloudCheckpoint.GetHeight() &&
		!previous.GetCheckpoint().EqualVT(cloud.GetCheckpoint()):
		return errors.New("cloud checkpoint conflicts with accepted peer checkpoint")
	}

	// Union the operation sets. Operations the selected checkpoint covers drop
	// out.
	for _, op := range other.GetOps() {
		if _, err := next.AddOperation(h.soID, op); err != nil {
			return errors.Wrap(err, "merge operation")
		}
	}
	pending := h.pendingPublication()
	if pending != nil {
		if err := dropCoveredOperations(h.soID, &api.PendingSOPublication{Checkpoint: next.GetCheckpoint(), Operations: pending.GetOperations()}); err != nil {
			return err
		}
	}

	// Persist the accepted state and its obligation to the verified cache.
	cache := h.buildVerifiedStateCache()
	if cache != nil && h.persistVerifiedStateCache != nil {
		cache.PendingPublication = pending
		cache.PeerState = next.CloneVT()
		cache.CloudState = cloud.CloneVT()
		cache.CloudSequence = sequence
		if err := h.persistVerifiedStateCache(ctx, cache); err != nil {
			return err
		}
	}

	// Publish the accepted state and detect a config chain change.
	var configChanged bool
	h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Commit the accepted state, cursor, and obligation into the host.
		h.pending = pending
		h.peerState = next.CloneVT()
		h.cloudState = cloud.CloneVT()
		h.lastSeqno = sequence
		h.initialStateErr = nil

		// Detect a config chain change for downstream reactions.
		configHash := cloud.GetConfig().GetConfigChainHash()
		configChanged = len(configHash) != 0 && !bytes.Equal(configHash, h.lastConfigChainHash)
		h.stateCtr.SetValue(next)
		broadcast()
	})
	if configChanged {
		h.triggerConfigChanged()
	}
	return nil
}

// dropCoveredOperations removes the pending operations that the pending
// checkpoint covers.
func dropCoveredOperations(sharedObjectID string, pending *api.PendingSOPublication) error {
	// Drop the operations the checkpoint covers.
	checkpoint, err := pending.GetCheckpoint().UnmarshalInner()
	if err != nil || checkpoint == nil {
		return err
	}
	set := sobject.NewSOOperationSet(sharedObjectID, checkpoint)
	pending.Operations = slices.DeleteFunc(pending.Operations, func(op *sobject.SOOperation) bool {
		inner, err := op.UnmarshalInner()
		return err == nil && set.Covers(inner.GetPeerId(), inner.GetNonce())
	})
	return nil
}

// publishConfigChange posts a signed configuration change with the state it
// changes through the cloud config-state transaction, then accepts the result.
// prev is the state held under the write lock and next is that state after the
// change. The caller holds writeMu.
func (h *cloudSOHost) publishConfigChange(ctx context.Context, prev, next *sobject.SOState, entry *sobject.SOConfigChange) error {
	// Settle accepted local work so the cloud holds the locked state.
	if h.syncer != nil {
		if err := h.syncer.FlushNow(ctx); err != nil {
			return errors.Wrap(err, "flush pending publication")
		}
	}

	// Carry a changed current key epoch and changed invites.
	var epochs []*sobject.SOKeyEpoch
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { epochs = cloneVTSlice(h.keyEpochs) })
	current := currentEpochWithFallback(prev, epochs)
	var epoch *sobject.SOKeyEpoch
	if nextEpoch := next.CurrentKeyEpoch(); !nextEpoch.EqualVT(prev.CurrentKeyEpoch()) {
		epoch = nextEpoch.CloneVT()
	}
	var invites []*sobject.SOInvite
	if !slices.EqualFunc(prev.GetInvites(), next.GetInvites(), (*sobject.SOInvite).EqualVT) {
		invites = cloneVTSlice(next.GetInvites())
	}

	// Recover the read key from the local grant.
	grant := current.FindGrant(h.peerID.String())
	if grant == nil {
		return errors.New("local grant not found")
	}
	grantInner, err := grant.DecryptInnerData(h.privKey, h.soID)
	if err != nil {
		return errors.Wrap(err, "decrypt local grant")
	}

	// Seal the read key for each entity of the next configuration.
	cfg, err := configWithConfigChangeHash(entry)
	if err != nil {
		return err
	}
	keyEpoch := current.GetEpoch()
	if epoch != nil {
		keyEpoch = epoch.GetEpoch()
	}
	envelopes, err := buildSORecoveryEnvelopes(ctx, h.client, h.soID, cfg, keyEpoch, grantInner)
	if err != nil {
		return errors.Wrap(err, "build recovery envelopes")
	}

	// Commit the change, then accept it.
	entryData, err := entry.MarshalVT()
	if err != nil {
		return err
	}
	if err := h.client.PostConfigState(ctx, h.soID, entryData, invites, epoch, envelopes); err != nil {
		return err
	}
	return h.applyConfigMutation(ctx, entry, invites, epoch)
}
