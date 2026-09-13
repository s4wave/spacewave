package provider_spacewave

import (
	"bytes"
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
)

// retainPublication commits accepted state and its cloud obligation atomically.
// The caller holds acceptMu. No watched local success precedes this transaction.
func (h *cloudSOHost) retainPublication(ctx context.Context, state *sobject.SOState, operation *sobject.SOOperation, root bool) error {
	if operation != nil && (&api.PostOpsRequest{Operations: []*sobject.SOOperation{operation}}).SizeVT() > 1<<20 {
		return errors.New("operation exceeds the cloud checkpoint limit")
	}
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
	pending := cache.GetPendingPublication()
	if pending == nil {
		pending = &api.PendingSOPublication{FirstPendingUnixMilli: time.Now().UnixMilli()}
	}
	if operation != nil {
		pending.Operations = append(pending.Operations, operation.CloneVT())
	}
	if root {
		pending.Root = state.GetRoot().CloneVT()
		pending.Rejections = append(pending.Rejections, diffSOOperationRejections(h.stateCtr.GetValue(), state)...)
		pending.Operations = sobject.FilterResolvedOperations(pending.Operations, pending.Root.GetAccountNonces(), nil, state.GetOpRejections())
	}
	cache.PeerState = state.CloneVT()
	cache.PendingPublication = pending
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
	if sent == nil {
		return nil
	}
	var config *sobject.SharedObjectConfig
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { config = h.verifiedConfig.CloneVT() })
	if config == nil {
		return sobject.ErrConfigHistoryUnavailable
	}
	if sent.GetRoot() != nil {
		valid, err := sent.Root.ValidateSignatures(h.soID, config.GetParticipants())
		if err != nil {
			return err
		}
		if err := sobject.CheckConsensusAcceptance(config.GetConsensusMode(), valid); err != nil {
			return err
		}
		if err := h.client.PostRoot(ctx, h.soID, sent.Root, sent.Rejections); err != nil {
			return err
		}
	}
	var batch []*sobject.SOOperation
	for _, operation := range sent.GetOperations() {
		if err := operation.ValidateSignature(h.soID, config.GetParticipants()); err != nil {
			return err
		}
		candidate := append(batch, operation)
		if len(candidate) > 50 || (&api.PostOpsRequest{Operations: candidate}).SizeVT() > 1<<20 {
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
	pending := cache.GetPendingPublication()
	if pending == nil {
		return nil
	}
	if pending.GetRoot().EqualVT(sent.GetRoot()) {
		pending.Root = nil
	}
	pending.Operations = slices.DeleteFunc(pending.Operations, func(op *sobject.SOOperation) bool {
		return slices.ContainsFunc(sent.Operations, func(other *sobject.SOOperation) bool { return op.EqualVT(other) })
	})
	pending.Rejections = slices.DeleteFunc(pending.Rejections, func(rejection *sobject.SOOperationRejection) bool {
		return slices.ContainsFunc(sent.Rejections, func(other *sobject.SOOperationRejection) bool { return rejection.EqualVT(other) })
	})
	if pending.GetRoot() == nil && len(pending.Operations) == 0 && len(pending.Rejections) == 0 {
		pending = nil
	}
	cache.PendingPublication = pending
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

// flushCheckpoint runs under flushMtx. Snapshotting roots before blocks prevents
// a concurrent edit from publishing a root whose dependencies missed this flush.
func (s *syncController) flushCheckpoint(ctx context.Context, orderBlocks bool) (retErr error) {
	if s.telemetry != nil {
		defer func() { s.telemetry.syncTelemetry.RecordError(s.resourceID, retErr) }()
	}
	var hosts []*cloudSOHost
	s.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for host := range s.publications {
			hosts = append(hosts, host)
		}
	})
	pending := make([]*api.PendingSOPublication, len(hosts))
	for i, host := range hosts {
		pending[i] = host.pendingPublication()
	}
	if err := s.flush(ctx, orderBlocks); err != nil {
		return err
	}
	for i, host := range hosts {
		if err := host.publishCheckpoint(ctx, pending[i]); err != nil {
			return err
		}
	}
	return nil
}

// acceptCloudSnapshot keeps the cloud cursor paired with its authenticated base,
// while preserving newer peer roots and unresolved local operations. The caller
// holds acceptMu and has verified both cloud signatures and changelog progression.
func (h *cloudSOHost) acceptCloudSnapshot(ctx context.Context, cloud *sobject.SOState, sequence uint64) error {
	previous := h.stateCtr.GetValue()
	next := cloud.CloneVT()
	other := previous
	if previous.GetRoot().GetInnerSeqno() > cloud.GetRoot().GetInnerSeqno() {
		next = h.stateWithVerifiedConfig(previous, cloud.GetConfig())
		other = cloud
	} else if previous.GetRoot().GetInnerSeqno() == cloud.GetRoot().GetInnerSeqno() && previous.GetRoot() != nil && !previous.GetRoot().EqualVT(cloud.GetRoot()) {
		return errors.New("cloud root conflicts with accepted peer root")
	}

	// Rebuild admissible work against the selected root. Its committed nonces
	// discard resolved operations; signed rejection identities remain available.
	for _, rejection := range diffSOOperationRejections(next, other) {
		inner, err := rejection.ValidateSignature(h.soID, next.GetConfig().GetParticipants())
		if err != nil {
			continue
		}
		group := slices.IndexFunc(next.OpRejections, func(group *sobject.SOPeerOpRejections) bool { return group.GetPeerId() == inner.GetPeerId() })
		if group == -1 {
			next.OpRejections = append(next.OpRejections, &sobject.SOPeerOpRejections{PeerId: inner.GetPeerId(), Rejections: []*sobject.SOOperationRejection{rejection.CloneVT()}})
		} else {
			next.OpRejections[group].Rejections = append(next.OpRejections[group].Rejections, rejection.CloneVT())
		}
	}
	slices.SortFunc(next.OpRejections, func(a, b *sobject.SOPeerOpRejections) int { return strings.Compare(a.GetPeerId(), b.GetPeerId()) })
	operations := append(next.Ops, cloneVTSlice(other.GetOps())...)
	operations = sobject.FilterResolvedOperations(operations, next.GetRoot().GetAccountNonces(), nil, next.GetOpRejections())
	slices.SortStableFunc(operations, func(a, b *sobject.SOOperation) int {
		aa, _ := a.UnmarshalInner()
		bb, _ := b.UnmarshalInner()
		if cmp := strings.Compare(aa.GetPeerId(), bb.GetPeerId()); cmp != 0 {
			return cmp
		}
		return cmp.Compare(aa.GetNonce(), bb.GetNonce())
	})
	next.Ops = nil
	next.QueuedAccountNonces = nil
	for _, operation := range operations {
		// Duplicate, committed, or revoked work is not admissible in this state.
		_ = next.QueueOperation(h.soID, operation)
	}

	// A signed rejection consumes its nonce even when an older cache reused it
	// for different local work. Re-sign only that provably unpublished collision;
	// retain the operation ID, encrypted contents, and original flush deadline.
	pending := h.pendingPublication()
	for i, operation := range pending.GetOperations() {
		inner, err := operation.UnmarshalInner()
		if err != nil {
			return err
		}
		if inner.GetPeerId() != h.peerID.String() {
			continue
		}
		collision := false
		for _, group := range next.GetOpRejections() {
			if group.GetPeerId() != inner.GetPeerId() {
				continue
			}
			for _, rejection := range group.GetRejections() {
				rejected, err := rejection.UnmarshalInner()
				if err != nil {
					return err
				}
				if rejected.GetOpNonce() == inner.GetNonce() && rejected.GetLocalId() != inner.GetLocalId() {
					collision = true
				}
			}
		}
		if !collision {
			continue
		}
		if err := operation.ValidateSignature(h.soID, next.GetConfig().GetParticipants()); err != nil {
			return err
		}
		operation, err = sobject.BuildSOOperation(h.soID, h.privKey, inner.GetOpData(), next.GetNextAccountNonce(inner.GetPeerId()), inner.GetLocalId())
		if err != nil {
			return err
		}
		if err := next.QueueOperation(h.soID, operation); err != nil {
			return err
		}
		pending.Operations[i] = operation
	}

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
	var configChanged bool
	h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		h.pending = pending
		h.peerState = next.CloneVT()
		h.cloudState = cloud.CloneVT()
		h.lastSeqno = sequence
		h.initialStateErr = nil
		configHash := cloud.GetConfig().GetConfigChainHash()
		configChanged = len(configHash) != 0 && !bytes.Equal(configHash, h.lastConfigChainHash)
		h.stateCtr.SetValue(next)
		broadcast()
	})
	if configChanged {
		h.triggerConfigChanged()
	}
	h.logNewOpRejections(previous, next, "cloud-checkpoint")
	return nil
}
