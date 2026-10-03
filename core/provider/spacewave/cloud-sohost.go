package provider_spacewave

import (
	"bytes"
	"context"
	"slices"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/seedflight"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// maxWriteRetries is the maximum number of retries on 409 write conflicts.
const maxWriteRetries = 3

// cloudSOHost implements the shared object state management via the wsTracker.
type cloudSOHost struct {
	// le is the logger.
	le *logrus.Entry
	// client is the session client for API calls.
	client *SessionClient
	// soID is the shared object ID.
	soID string
	// selfEntityID is the stable entity id for the mounted account.
	selfEntityID string
	// privKey is the session private key for signing.
	privKey crypto.PrivKey
	// peerID is the local peer ID derived from privKey.
	peerID peer.ID
	// sfs is the step factory set for block transforms.
	sfs *block_transform.StepFactorySet
	// tracker is the shared session WebSocket tracker.
	tracker *wsTracker
	// soHost is the SOHost managing the refcount.
	soHost *sobject.SOHost
	// stateCtr contains the current shared object state.
	stateCtr *ccontainer.CContainer[*sobject.SOState]
	// snapCtr contains the derived SharedObjectStateSnapshot.
	snapCtr *ccontainer.CContainer[sobject.SharedObjectStateSnapshot]
	// lastSeqno tracks the last applied change_log sequence number.
	lastSeqno uint64
	// lastConfigChainHash tracks the last known config chain hash.
	lastConfigChainHash []byte
	// rotatedConfigChainHash is the head whose participant removal this peer
	// last rotated the key for.
	rotatedConfigChainHash []byte
	// verifiedConfigChainSeqno tracks the seqno of the last verified config chain head.
	verifiedConfigChainSeqno uint64
	// keyEpochs stores the key epochs fetched from the config chain.
	keyEpochs []*sobject.SOKeyEpoch
	// verifiedConfig stores the latest trusted config from the verified chain.
	verifiedConfig *sobject.SharedObjectConfig
	// genesisHash is the hash of the genesis config change entry, pinned on first chain fetch.
	genesisHash []byte
	// persistVerifiedStateCache stores verified SO config state for restart hydration.
	persistVerifiedStateCache func(context.Context, *api.VerifiedSOStateCache) error
	// stateObserved projects accepted checkpoint mechanics without owning state.
	stateObserved func(*sobject.SOState)
	// bcast guards state, trusted configuration, and notification snapshots.
	bcast broadcast.Broadcast
	// acceptMu serializes cache acceptance and persistence, never HTTP requests.
	acceptMu csync.Mutex
	// configHistory retains immutable verified transitions, guarded by bcast.
	configHistory []*sobject.SOConfigChange
	// historyIndex serves bounded hash traversal; acceptMu guards updates and reads.
	historyIndex map[string]*sobject.SOConfigChange
	// peerState is the durable peer snapshot retained across cache-only updates.
	peerState *sobject.SOState
	// pending retains only work accepted locally for cloud publication.
	pending *api.PendingSOPublication
	// cloudState is the authenticated cloud-delta base, independent of live peers.
	cloudState *sobject.SOState
	// syncer owns this host's block sync and checkpoint lifetime.
	syncer *syncController
	// writeMu serializes local writes to prevent self-nonce conflicts.
	writeMu csync.Mutex
	// pullRoutine runs coalesced gap-recovery state pulls.
	pullRoutine *coalescedTriggerRoutine
	// pullSeed coordinates concurrent pullState callers (cold seed in
	// Execute, lockFn cold fallback, gap recovery in pullRoutine, write
	// retries, op-queue cold fallback) so they share one in-flight HTTP
	// fetch and observe the same error. Guarded by bcast.
	pullSeed seedflight.Seed
	// chainSeed coordinates concurrent syncConfigChain callers
	// (pullState inline recovery and configChangedRoutine handler)
	// so a single /config-chain fetch covers both verifier triggers.
	// Guarded by bcast.
	chainSeed seedflight.Seed
	// snapDeriver derives SharedObjectStateSnapshot values from stateCtr.
	snapDeriver *routine.RoutineContainer
	// configChangedRoutine runs coalesced config-chain verification.
	configChangedRoutine *coalescedTriggerRoutine
	// ctxCancel cancels the host context on participant removal.
	ctxCancel context.CancelFunc
	// onPeerRevoked is called when a peer is removed from the config chain with
	// RevocationInfo. Called with the revoked peer ID string.
	onPeerRevoked func(peerIDStr string)
	// refreshBlockManifest pulls remote packfile metadata before publishing
	// remote SO state that may reference newly-pushed blocks.
	refreshBlockManifest func(ctx context.Context) error
	// blockManifestSequence returns the local pull cursor for the backing block
	// store manifest.
	blockManifestSequence func(ctx context.Context) (uint64, error)
	// initialStateErr stores the last verification rejection seen while no
	// accepted state snapshot had been cached yet.
	initialStateErr error
}

// newCloudSOHost constructs a new cloudSOHost.
func newCloudSOHost(
	le *logrus.Entry,
	client *SessionClient,
	soID string,
	selfEntityID string,
	tracker *wsTracker,
	privKey crypto.PrivKey,
	peerID peer.ID,
	sfs *block_transform.StepFactorySet,
	verifiedCache *api.VerifiedSOStateCache,
	persistVerifiedStateCache func(context.Context, *api.VerifiedSOStateCache) error,
	syncer *syncController,
) *cloudSOHost {
	// Construct state containers and background routines before exposing the host.
	h := &cloudSOHost{
		le:                        le,
		client:                    client,
		soID:                      soID,
		selfEntityID:              selfEntityID,
		privKey:                   privKey,
		peerID:                    peerID,
		sfs:                       sfs,
		tracker:                   tracker,
		stateCtr:                  ccontainer.NewCContainer[*sobject.SOState](nil),
		snapCtr:                   ccontainer.NewCContainer[sobject.SharedObjectStateSnapshot](nil),
		persistVerifiedStateCache: persistVerifiedStateCache,
		syncer:                    syncer,
	}
	h.pullRoutine = newCoalescedTriggerRoutine(le, "so-state-pull", h.pullOnTrigger)
	h.snapDeriver = newNamedRoutineContainer(le, "so-snapshot-deriver")
	h.snapDeriver.SetRoutine(h.runSnapDeriver)
	h.configChangedRoutine = newCoalescedTriggerRoutine(le, "so-config-chain-verifier", h.handleConfigChanged)
	h.hydrateVerifiedStateCache(verifiedCache)

	// watchFn returns the stateCtr which is updated by pull-on-notify.
	watchFn := func(ctx context.Context, sharedObjectID string, released func()) (ccontainer.Watchable[*sobject.SOState], func(), error) {
		return h.stateCtr, func() {}, nil
	}

	// lockFn acquires the local write mutex and reads from the cached stateCtr.
	// WriteSOState persists accepted local work before peer notification.
	lockFn := func(ctx context.Context, sharedObjectID string) (sobject.SOStateLock, error) {
		// Serialize local writers until their returned state lock is released.
		relLock, err := h.writeMu.Lock(ctx)
		if err != nil {
			return nil, err
		}

		// Read current state from cached stateCtr (kept fresh by pull-on-notify).
		// Fall back to HTTP GET if no cached state yet.
		if err := h.ensureInitialState(ctx, SeedReasonColdSeed); err != nil {
			relLock()
			return nil, errors.Wrap(err, "initial state pull for lock")
		}
		state := h.stateCtr.GetValue()
		if state == nil {
			relLock()
			return nil, errors.New("no state available after pull")
		}

		// Bind the local snapshot to durable acceptance and the acquired write lock.
		initialState := state.CloneVT()
		writeFn := func(ctx context.Context, state *sobject.SOState, changes ...*sobject.SOConfigChange) error {
			if len(changes) == 0 {
				return h.acceptLocalWrite(ctx, initialState, state)
			}
			if len(changes) != 1 {
				return errors.New("cloud publishes one configuration change per write")
			}
			return h.publishConfigChange(ctx, initialState, state, changes[0])
		}

		return sobject.NewSOStateLock(initialState, writeFn, relLock), nil
	}

	// Pass nil context; SetContext called in Execute.
	h.soHost = sobject.NewSOHost(nil, watchFn, lockFn, soID, &sobject.SOHostSyncFuncs{Lock: h.peerImportLock, History: h.readConfigHistory, Entry: h.readConfigEntry})
	return h
}

// Execute runs the cloudSOHost lifecycle.
func (h *cloudSOHost) Execute(ctx context.Context) error {
	return h.execute(ctx, nil)
}

// execute subscribes before seeding state and publishes readiness only after
// the caller's initial enrollment has completed under that same subscription.
func (h *cloudSOHost) execute(ctx context.Context, ready func(context.Context) error) error {
	// Keep the host and its notification callback inside one cancellation lifetime.
	ctx, cancel := context.WithCancel(ctx)
	h.ctxCancel = cancel
	defer cancel()

	// Receive notifications for the host's lifetime.
	h.soHost.SetContext(ctx)
	h.tracker.RegisterNotifyCallback(h.soID, func(payload *api.SONotifyEventPayload) {
		h.handleSONotifyWithContext(ctx, payload)
	})
	defer h.tracker.UnregisterNotifyCallback(h.soID)

	// Attach background work before seeding the first observable snapshot.
	h.pullRoutine.SetContext(ctx)
	defer h.pullRoutine.ClearContext()
	h.snapDeriver.SetContext(ctx, true)
	defer h.snapDeriver.ClearContext()
	h.configChangedRoutine.SetContext(ctx)
	defer h.configChangedRoutine.ClearContext()

	// Seed the local SO state immediately so first mount does not depend on a
	// later websocket notification to populate the state containers. A state
	// restored from the verified cache serves the mount at once, and a
	// background pull catches up on changes made while this host was away.
	cached := h.stateCtr.GetValue() != nil
	if err := h.ensureInitialState(ctx, SeedReasonColdSeed); err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return errors.Wrap(err, "initial state pull")
	}
	if cached {
		h.triggerPull()
	}
	h.triggerConfigChanged()
	if ready != nil {
		if err := ready(ctx); err != nil {
			return err
		}
	}

	// Retain the registered callback until host cancellation.
	<-ctx.Done()
	return context.Canceled
}

// ensureInitialState populates stateCtr once before read/write/watch paths run.
// If stateCtr already holds a value from an earlier mount-time seed, it reuses
// that snapshot instead of issuing another HTTP GET.
func (h *cloudSOHost) ensureInitialState(ctx context.Context, reason SeedReason) error {
	// Reuse an accepted snapshot or join the existing cold-seed request.
	if h.stateCtr.GetValue() != nil {
		return nil
	}
	if err := h.pullStateSingleflight(ctx, reason); err != nil {
		return err
	}

	// Preserve the verification failure instead of reporting an empty cache.
	if h.stateCtr.GetValue() == nil {
		var initialStateErr error
		h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			initialStateErr = h.initialStateErr
		})
		if initialStateErr != nil {
			return initialStateErr
		}
		return errors.New("no state available after pull")
	}
	return nil
}

// runSnapDeriver watches stateCtr and derives SharedObjectStateSnapshot values into snapCtr.
func (h *cloudSOHost) runSnapDeriver(ctx context.Context) error {
	var prev *sobject.SOState
	for {
		// Wait on the state container's atomic change notification.
		next, err := h.stateCtr.WaitValueChange(ctx, prev, nil)
		if err != nil {
			return err
		}
		prev = next

		// Project each accepted state through the existing participant handle.
		if h.stateObserved != nil {
			h.stateObserved(next)
		}
		h.snapCtr.SetValue(h.newSnapshot(next))
	}
}

// newSnapshot returns a participant handle for state that resolves the config
// of an operation written under an earlier config from the verified history.
func (h *cloudSOHost) newSnapshot(state *sobject.SOState) *sobject.SOStateParticipantHandle {
	snap := sobject.NewSOStateParticipantHandle(h.le, h.sfs, h.soID, state, h.privKey, h.peerID)
	return snap.WithConfigHistory(func(ctx context.Context, hash []byte) (*sobject.SOConfigChange, error) {
		return h.readConfigEntry(ctx, h.soID, hash)
	})
}

// pullOnTrigger fetches fresh state via HTTP GET after a pull signal.
// With event-carried SO state deltas in place, the only callers that signal
// pullRoutine are gap-recovery paths (inline delta apply failed because the
// cache is behind by more than the cloud retains). Cold seed and write
// retry paths call pullState directly rather than queueing a signal.
func (h *cloudSOHost) pullOnTrigger(ctx context.Context) {
	if err := h.pullStateSingleflight(ctx, SeedReasonGapRecovery); err != nil {
		if ctx.Err() != nil {
			return
		}
		h.le.WithError(err).Warn("failed to pull state on notify")
	}
}

// pullStateSingleflight runs pullState behind seedflight.Seed so concurrent
// callers (cold-seed in Execute, lockFn no-state fallback, gap recovery in
// pullRoutine, write conflict retry, op queue cold fallback) share one
// in-flight HTTP fetch and observe the same outcome. reason tags the fan-out
// origin for the resulting HTTP GET.
func (h *cloudSOHost) pullStateSingleflight(ctx context.Context, reason SeedReason) error {
	return h.pullSeed.Run(ctx, &h.bcast, func(ctx context.Context) error {
		return h.pullState(ctx, reason)
	})
}

// pullState fetches the current state via HTTP GET and updates stateCtr.
// reason tags the fan-out origin on the underlying HTTP request.
func (h *cloudSOHost) pullState(ctx context.Context, reason SeedReason) error {
	// Fetch from the held changelog cursor, falling back when a full snapshot is needed.
	var since uint64
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		since = h.lastSeqno
	})
	stateData, err := h.client.GetSOState(ctx, h.soID, since, reason)
	if err != nil {
		return err
	}

	// Decode a snapshot, retrying without a cursor when the server returned a delta.
	state, lastSeqno, configChain, err := decodeSOStateResponse(stateData)
	if errors.Is(err, errSOStateDeltaResponse) && since > 0 {
		h.le.WithField("since", since).Debug("received delta from state pull, retrying full snapshot")
		stateData, err = h.client.GetSOState(ctx, h.soID, 0, SeedReasonGapRecovery)
		if err != nil {
			return err
		}
		state, lastSeqno, configChain, err = decodeSOStateResponse(stateData)
	}
	if err != nil {
		return err
	}

	// Reject rollback before synchronizing the response's embedded authority.
	if err := h.verifyChangeLogSeqno(lastSeqno); err != nil {
		h.noteInitialStateRejection(
			errors.Wrapf(
				errSharedObjectInitialStateRejected,
				"changelog verification: %v",
				err,
			),
		)
		h.le.WithError(err).Warn("pulled state failed changelog verification, ignoring")
		return nil
	}

	// Verify the response under its authenticated configuration history.
	embeddedChainSynced := h.syncEmbeddedConfigChain(ctx, state, configChain)
	if err := h.verifyPulledState(state); err != nil {
		// A matching head cannot recover an invalid snapshot by fetching more history.
		if !errors.Is(err, errSOConfigChainChanged) {
			h.noteInitialStateRejection(
				errors.Wrapf(
					errSharedObjectInitialStateRejected,
					"state verification: %v",
					err,
				),
			)
			h.le.WithError(err).Warn("pulled state failed verification, ignoring")
			return nil
		}

		// Resolve a newer head once, then repeat the complete snapshot verification.
		if !embeddedChainSynced {
			if syncErr := h.syncConfigChainSingleflight(ctx, state.GetConfig().GetConfigChainHash()); syncErr != nil {
				h.le.WithError(syncErr).Warn("failed to verify updated config chain for pulled state")
				return nil
			}
		}
		if err := h.verifyPulledState(state); err != nil {
			h.noteInitialStateRejection(errors.Wrapf(errSharedObjectInitialStateRejected, "state verification after config sync: %v", err))
			h.le.WithError(err).Warn("pulled state failed verification after config sync, ignoring")
			return nil
		}
	}

	// Recheck after network work while excluding concurrent peer imports.
	release, err := h.acceptMu.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// An inline update or local configuration write can advance state while
	// this HTTP response is in flight; the host then already holds newer state.
	if err := h.verifyChangeLogSeqno(lastSeqno); err != nil {
		return nil
	}
	if err := h.verifyPulledState(state); err != nil {
		// A config chain sync can advance the held head meanwhile. The
		// verifier already signaled the sync, which carries state forward.
		if errors.Is(err, errSOConfigChainChanged) {
			return nil
		}
		return err
	}

	return h.acceptCloudSnapshot(ctx, state, lastSeqno)
}

// noteInitialStateRejection records a rejected cold-seed verification outcome
// so ensureInitialState can return a terminal mount error instead of falling
// back to a generic retryable "no state available after pull" error.
func (h *cloudSOHost) noteInitialStateRejection(err error) {
	// Record failures only until the first state is accepted.
	if err == nil || h.stateCtr.GetValue() != nil {
		return
	}
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		h.initialStateErr = err
	})
}

// DecodeSOStateSnapshot decodes a full-snapshot SOStateMessage response.
//
// Returns an error if the response carries a delta instead of a snapshot.
func DecodeSOStateSnapshot(data []byte) (*sobject.SOState, error) {
	state, _, _, err := decodeSOStateResponse(data)
	return state, err
}

// decodeSOStateResponse decodes an SOStateMessage snapshot or delta marker.
func decodeSOStateResponse(data []byte) (*sobject.SOState, uint64, *sobject.SOConfigChainResponse, error) {
	msg := &api.SOStateMessage{}
	if err := msg.UnmarshalVT(data); err != nil {
		return nil, 0, nil, errors.Wrap(err, "unmarshal SOStateMessage")
	}
	if snap := msg.GetSnapshot(); snap != nil {
		return snap, msg.GetSeqno(), msg.GetConfigChain(), nil
	}
	if msg.GetDelta() != nil {
		return nil, 0, nil, errSOStateDeltaResponse
	}
	return nil, 0, nil, errors.New("missing snapshot or delta in SOStateMessage")
}

// syncEmbeddedConfigChain verifies a response's embedded chain through the shared fetch coordinator.
func (h *cloudSOHost) syncEmbeddedConfigChain(
	ctx context.Context,
	state *sobject.SOState,
	chain *sobject.SOConfigChainResponse,
) bool {
	// A response without an embedded chain has nothing to verify.
	if state == nil || chain == nil {
		return false
	}

	// Note the head before this sync to detect an advance.
	var prevHash []byte
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		prevHash = h.lastConfigChainHash
	})

	// Verify the chain up to the response's head.
	newHash := state.GetConfig().GetConfigChainHash()
	if err := h.chainSeed.Run(ctx, &h.bcast, func(ctx context.Context) error {
		return h.syncConfigChainResponse(ctx, chain, newHash)
	}); err != nil {
		h.le.WithError(err).Warn("failed to verify embedded config chain from state response")
		return false
	}

	// Check that the verified head matches the response.
	var synced bool
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		synced = bytes.Equal(h.lastConfigChainHash, newHash)
	})

	// The verifier never sees a head advanced here, so wake the config
	// reaction for changes such as a departure handed to this peer.
	if synced && len(prevHash) != 0 && !bytes.Equal(prevHash, newHash) {
		h.triggerConfigChanged()
	}

	return synced
}

// verifyChangeLogSeqno rejects snapshots whose changelog counter goes backwards.
func (h *cloudSOHost) verifyChangeLogSeqno(snapshotSeqno uint64) error {
	var lastSeqno uint64
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		lastSeqno = h.lastSeqno
	})
	if snapshotSeqno < lastSeqno {
		return errors.Errorf("seqno rollback: got %d, last was %d", snapshotSeqno, lastSeqno)
	}
	return nil
}

// verifyPulledState performs client-side verification on a pulled SOState.
// Checks config chain hash continuity and checkpoint authority.
func (h *cloudSOHost) verifyPulledState(state *sobject.SOState) error {
	var held *sobject.SOState
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		held = h.cloudState
		if held == nil && h.pending == nil && h.peerState == nil {
			held = h.stateCtr.GetValue()
		}
	})
	return h.verifyStateAgainst(state, held)
}

// verifyStateAgainst binds signatures and checkpoint progression to the given
// origin.
func (h *cloudSOHost) verifyStateAgainst(state, held *sobject.SOState) error {
	// Snapshot the trusted head before comparing the response.
	var lastConfigHash []byte
	var trustedConfig *sobject.SharedObjectConfig
	var trustedSeqno uint64
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		lastConfigHash = h.lastConfigChainHash
		trustedConfig = h.verifiedConfig
		trustedSeqno = h.verifiedConfigChainSeqno
	})

	// D3/D4: Reject state if its config chain hash differs from the last
	// verified hash. This prevents a malicious server from injecting a
	// participant list that was never verified through the config chain.
	// On first mount (lastConfigChainHash unset), skip this check and let
	// the config chain verifier establish trust.
	configHash := state.GetConfig().GetConfigChainHash()
	if len(lastConfigHash) > 0 && !bytes.Equal(configHash, lastConfigHash) {
		h.triggerConfigChanged()
		return errSOConfigChainChanged
	}

	// Bind authority to the verified configuration, including all participant roles.
	if trustedConfig != nil {
		trustedConfig = trustedConfig.CloneVT()
		trustedConfig.ConfigChainHash = bytes.Clone(lastConfigHash)
		trustedConfig.ConfigChainSeqno = trustedSeqno
		if !sobject.EqualSOConfigs(trustedConfig, state.GetConfig()) {
			return errors.New("cloud state differs from verified configuration")
		}
	}

	// An uninitialized cloud object is legal only before a checkpoint has
	// been accepted.
	if state.GetCheckpoint() == nil {
		if held.GetCheckpoint() != nil {
			return errors.New("cloud checkpoint rollback")
		}
		return nil
	}

	// Every signature verifies, and the verified participants authorize each
	// checkpoint and grant this host has not adopted yet. Replay authorizes
	// operations under the config each names.
	if err := state.Validate(h.soID); err != nil {
		return err
	}
	participants := state.GetConfig().GetParticipants()
	for _, epoch := range state.GetKeyEpochs() {
		heldEpoch := held.GetKeyEpoch(epoch.GetEpoch())
		for _, grant := range epoch.GetGrants() {
			if slices.ContainsFunc(heldEpoch.GetGrants(), grant.EqualVT) {
				continue
			}
			if err := grant.ValidateSignature(h.soID, participants); err != nil {
				return errors.Wrapf(err, "key epoch %d grant", epoch.GetEpoch())
			}
		}
	}
	if state.GetCheckpoint().EqualVT(held.GetCheckpoint()) {
		return nil
	}
	next, err := state.GetCheckpoint().ValidateAuthority(h.soID, participants)
	if err != nil {
		return errors.Wrap(err, "checkpoint authority")
	}

	// Never roll back past the accepted checkpoint or replace it at its height.
	prev, err := held.GetCheckpointInner()
	if err != nil {
		return err
	}
	if prev != nil && next.GetHeight() <= prev.GetHeight() {
		return errors.New("cloud checkpoint conflicts with accepted checkpoint")
	}
	return nil
}

// triggerPull sends a non-blocking signal to the pull routine. Use this
// only for gap-recovery cases where an inline state apply failed; cold-seed
// and write-retry callers should invoke pullState directly so the result is
// observable inline.
func (h *cloudSOHost) triggerPull() {
	if h.pullRoutine != nil {
		h.pullRoutine.Trigger()
	}
}

// handleSONotify processes an SONotifyEventPayload delivered via so_notify.
func (h *cloudSOHost) handleSONotify(payload *api.SONotifyEventPayload) {
	h.handleSONotifyWithContext(context.Background(), payload)
}

// handleSONotifyWithContext processes an SONotifyEventPayload delivered via so_notify.
// When the payload carries an inline SOStateMessage, the host applies it
// directly without issuing an HTTP /state pull. A configChanged event pulls
// the state holding the new configuration, which another peer's change does
// not otherwise deliver.
func (h *cloudSOHost) handleSONotifyWithContext(ctx context.Context, payload *api.SONotifyEventPayload) {
	// Ignore absent notifications before inspecting their state payload.
	if payload == nil {
		return
	}

	// Refresh referenced blocks before applying an inline snapshot or delta.
	if msg := payload.GetStateMessage(); msg != nil {
		if err := h.refreshBlockManifestForNonce(ctx, payload.GetBlockStoreNonce()); err != nil {
			h.le.WithError(err).
				WithField("change-type", payload.GetChangeType()).
				WithField("seqno", payload.GetSeqno()).
				WithField("blockstore-nonce", payload.GetBlockStoreNonce()).
				Warn("failed to refresh block manifest before inline state apply")
			h.triggerPull()
			return
		}
		if err := h.handleStateDelta(ctx, msg); err != nil {
			// Config chain mismatch: verifyPulledState already signaled the
			// config chain verifier, which fetches /config-chain and refreshes
			// the trusted hash. Firing /state here would just re-read the same
			// inline state and fail verification the same way until the chain
			// catches up; let the next inline event (or gap recovery) carry
			// the state forward instead.
			if errors.Is(err, errSOConfigChainChanged) {
				h.le.WithError(err).
					WithField("change-type", payload.GetChangeType()).
					WithField("seqno", payload.GetSeqno()).
					Debug("inline state deferred to config chain sync")
				return
			}
			h.le.WithError(err).
				WithField("change-type", payload.GetChangeType()).
				WithField("seqno", payload.GetSeqno()).
				Debug("inline state apply failed; falling back to pull")
			h.triggerPull()
		}
		return
	}

	// A configuration notification carries no state, so pull the state that
	// holds the new configuration; applying it wakes the chain verifier.
	switch payload.GetChangeType() {
	case "configChanged":
		h.triggerPull()
	case "metadata", "ownershipChanged", "access_changed", "delete":
		// Account-level notification routing handles these.
	default:
		// Bare notify with no state payload. Cloud should always attach an
		// inline SOStateMessage for op and checkpoint mutations, so this indicates a
		// publisher bug rather than a missed update we should pull behind.
		h.le.WithField("change-type", payload.GetChangeType()).
			WithField("seqno", payload.GetSeqno()).
			Warn("so_notify arrived without inline state payload; ignoring")
	}
}

// refreshBlockManifestForNonce makes referenced blocks available before publishing newer state.
func (h *cloudSOHost) refreshBlockManifestForNonce(ctx context.Context, nonce uint64) error {
	// Skip absent notifications and already-pulled manifest versions.
	if nonce == 0 || h.refreshBlockManifest == nil {
		return nil
	}
	if h.blockManifestSequence != nil {
		lastSeq, err := h.blockManifestSequence(ctx)
		if err != nil {
			return errors.Wrap(err, "read local block manifest sequence")
		}
		if nonce <= lastSeq {
			return nil
		}
	}

	// Fetch the required manifest through the backing store's existing refresh.
	return h.refreshBlockManifest(ctx)
}

// handleStateDelta applies an inline SOStateMessage from a session WS event.
// Snapshots replace stateCtr after verifyPulledState succeeds; deltas are
// applied entry-by-entry against a clone of the cached state and committed
// atomically with the new lastSeqno. Returns an error when the delta cannot
// be applied (gap, decode failure, or verification failure); the caller may
// fall back to an HTTP pull.
func (h *cloudSOHost) handleStateDelta(ctx context.Context, msg *api.SOStateMessage) error {
	// Serialize the complete inline acceptance with cloud and peer publications.
	release, err := h.acceptMu.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Apply only the response variant supplied by the cloud.
	switch {
	case msg.GetSnapshot() != nil:
		// Verify a complete snapshot before retaining or exposing it.
		snap := msg.GetSnapshot()
		if err := h.verifyChangeLogSeqno(msg.GetSeqno()); err != nil {
			return errors.Wrap(err, "verify inline snapshot seqno")
		}
		if err := h.verifyPulledState(snap); err != nil {
			return errors.Wrap(err, "verify inline snapshot")
		}
		return h.acceptCloudSnapshot(ctx, snap, msg.GetSeqno())

	case msg.GetDelta() != nil:
		// Read the delta and the held base needed to apply it.
		delta := msg.GetDelta()
		entries := delta.GetEntries()
		if len(entries) == 0 {
			return nil
		}

		var (
			cached    *sobject.SOState
			lastSeqno uint64
		)
		h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			cached = h.cloudState
			if cached == nil && h.pending == nil {
				cached = h.stateCtr.GetValue()
			}
			lastSeqno = h.lastSeqno
		})

		// Cold cache: cannot apply a delta without a base snapshot.
		if cached == nil {
			return errors.New("delta arrived before initial snapshot")
		}

		// Duplicate: cloud may resend deltas the cache already includes.
		// Drop silently rather than treating as a gap.
		since := delta.GetSince()
		if since < lastSeqno {
			return nil
		}

		// Gap: events between lastSeqno+1 and since are missing. Reset
		// lastSeqno=0 under the broadcast lock so the gap-recovery pull
		// forces since=0 and ingests a fresh full snapshot instead of
		// looping through deltas the cloud may no longer retain. The
		// caller in handleSONotify dispatches that pull via triggerPull.
		if since > lastSeqno {
			h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				if h.lastSeqno == lastSeqno {
					h.lastSeqno = 0
					broadcast()
				}
			})
			return errors.Errorf("delta gap: since=%d, last=%d", since, lastSeqno)
		}

		// Apply the contiguous delta to an independent snapshot.
		next := cached.CloneVT()
		expected := since + 1
		for _, entry := range entries {
			if entry.GetSeqno() != expected {
				return errors.Errorf("delta entry out of order: got=%d, want=%d", entry.GetSeqno(), expected)
			}
			if err := applyChangeLogEntry(h.soID, next, entry); err != nil {
				return errors.Wrapf(err, "apply entry seqno=%d", entry.GetSeqno())
			}
			expected++
		}

		// Retain only a fully verified result before publishing it.
		if err := h.verifyPulledState(next); err != nil {
			return errors.Wrap(err, "verify state after delta apply")
		}

		return h.acceptCloudSnapshot(ctx, next, entries[len(entries)-1].GetSeqno())

	case msg.GetConfigChanged() != nil:
		// The new configuration arrives with the next pulled state.
		h.triggerPull()
		return nil

	case msg.GetError() != nil:
		h.le.WithField("code", msg.GetError().GetCode()).
			WithField("message", msg.GetError().GetMessage()).
			Warn("received error from SO DO via inline state")
		return nil
	}
	return nil
}

// applyChangeLogEntry applies a single change_log entry to the cached state.
// The change_data wire format mirrors the cloud's sharedobject DO: 'op' carries
// an SOOperation, 'ops' a PostOpsRequest, 'checkpoint' a PostCheckpointRequest
// and 'sequence' an SOSequenceBatch. Entries are idempotent.
func applyChangeLogEntry(
	sharedObjectID string,
	state *sobject.SOState,
	entry *api.SOStateDeltaEntry,
) error {
	switch entry.GetChangeType() {
	case "ops":
		batch := &api.PostOpsRequest{}
		if err := batch.UnmarshalVT(entry.GetChangeData()); err != nil {
			return err
		}
		for _, op := range batch.GetOperations() {
			if _, err := state.AddOperation(sharedObjectID, op); err != nil {
				return errors.Wrap(err, "add operation")
			}
		}
		return nil

	case "op":
		op := &sobject.SOOperation{}
		if err := op.UnmarshalVT(entry.GetChangeData()); err != nil {
			return errors.Wrap(err, "unmarshal op envelope")
		}
		if _, err := state.AddOperation(sharedObjectID, op); err != nil {
			return errors.Wrap(err, "add operation")
		}
		return nil

	case "checkpoint":
		req := &api.PostCheckpointRequest{}
		if err := req.UnmarshalVT(entry.GetChangeData()); err != nil {
			return errors.Wrap(err, "unmarshal post checkpoint request")
		}
		if req.GetCheckpoint() == nil {
			return errors.New("post checkpoint request missing checkpoint")
		}
		return errors.Wrap(state.AdoptCheckpoint(sharedObjectID, req.GetCheckpoint()), "adopt checkpoint")

	case "sequence":
		batch := &api.SOSequenceBatch{}
		if err := batch.UnmarshalVT(entry.GetChangeData()); err != nil {
			return errors.Wrap(err, "unmarshal sequence batch")
		}
		for _, record := range batch.GetSequence() {
			if _, err := state.AddSequence(sharedObjectID, record); err != nil {
				return errors.Wrap(err, "add sequence position")
			}
		}
		return nil

	default:
		return errors.Errorf("unknown change_type %q", entry.GetChangeType())
	}
}

// acceptLocalWrite durably accepts a local write of base to written for
// publication. Peer imports and cloud snapshots may have moved the accepted
// state since base was read, so the write is rebased onto the current state.
func (h *cloudSOHost) acceptLocalWrite(ctx context.Context, base, written *sobject.SOState) error {
	// Serialize acceptance.
	release, err := h.acceptMu.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Rebase the write onto the current state and verify it.
	current := h.stateCtr.GetValue()
	next, ops, err := rebaseLocalWrite(h.soID, base, written, current)
	if err != nil {
		return err
	}
	if err := h.verifyStateAgainst(next, current); err != nil {
		return err
	}

	// Retain the result for publication.
	checkpoint := !next.GetCheckpoint().EqualVT(current.GetCheckpoint())
	return h.retainPublication(ctx, next, ops, checkpoint)
}

// rebaseLocalWrite applies the changes a local write made from base to
// written onto current. It returns the rebased state and the operations the
// write added.
func rebaseLocalWrite(sharedObjectID string, base, written, current *sobject.SOState) (*sobject.SOState, []*sobject.SOOperation, error) {
	// Collect the operations the write added.
	var ops []*sobject.SOOperation
	for _, op := range written.GetOps() {
		if !slices.ContainsFunc(base.GetOps(), op.EqualVT) {
			ops = append(ops, op)
		}
	}
	if current == nil || current.EqualVT(base) {
		return written, ops, nil
	}

	// Apply each changed field to the current state.
	next := current.CloneVT()
	if !written.GetCheckpoint().EqualVT(base.GetCheckpoint()) {
		if err := next.AdoptCheckpoint(sharedObjectID, written.GetCheckpoint()); err != nil {
			return nil, nil, err
		}
	}
	for _, op := range ops {
		if _, err := next.AddOperation(sharedObjectID, op); err != nil {
			return nil, nil, err
		}
	}
	for _, epoch := range written.GetKeyEpochs() {
		if !epoch.EqualVT(base.GetKeyEpoch(epoch.GetEpoch())) {
			next.SetKeyEpoch(epoch.CloneVT())
		}
	}
	if !slices.EqualFunc(written.GetInvites(), base.GetInvites(), (*sobject.SOInvite).EqualVT) {
		next.Invites = cloneVTSlice(written.GetInvites())
	}
	return next, ops, nil
}

// applyKeyEpoch updates the cached key-epoch state after a successful write.
func (h *cloudSOHost) applyKeyEpoch(ctx context.Context, epoch *sobject.SOKeyEpoch) {
	// Serialize epoch acceptance with state and authority publications.
	if epoch == nil {
		return
	}
	release, err := h.acceptMu.Lock(ctx)
	if err != nil {
		return
	}
	defer release()

	// Prepare epoch and grant changes against current authority after the HTTP wait.
	cache := h.buildVerifiedStateCache()
	if cache == nil {
		return
	}
	cache.KeyEpochs = mergeSOKeyEpochs(cache.KeyEpochs, epoch)
	next := h.stateCtr.GetValue().CloneVT()
	if next != nil {
		next.SetKeyEpoch(epoch.CloneVT())
		next = h.stateWithVerifiedConfig(next, next.GetConfig())
		if h.peerState != nil {
			cache.PeerState = next.CloneVT()
		}
	}
	if h.persistVerifiedStateCache != nil {
		if err := h.persistVerifiedStateCache(ctx, cache); err != nil {
			h.le.WithError(err).Warn("failed to retain accepted key epoch")
			return
		}
	}

	// Publish the durable epoch and the state holding it together.
	h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		h.keyEpochs = cache.KeyEpochs
		h.peerState = cache.PeerState
		if next != nil {
			h.stateCtr.SetValue(next)
		}
		broadcast()
	})
}

// applyConfigMutation updates the cached state after a successful config-state
// write.
func (h *cloudSOHost) applyConfigMutation(
	ctx context.Context,
	entry *sobject.SOConfigChange,
	nextInvites []*sobject.SOInvite,
	epoch *sobject.SOKeyEpoch,
) error {
	// Serialize with peer imports.
	release, err := h.acceptMu.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()

	// Identify the signed transition before comparing it with held authority.
	newHash, err := sobject.HashSOConfigChange(entry)
	if err != nil {
		return errors.Wrap(err, "hash config change")
	}

	// A completed server request may race a peer import or the config chain
	// verifier, which may already hold this entry and its successors. Never
	// regress held authority.
	if _, ok := h.historyIndex[string(newHash)]; ok {
		return nil
	}
	if current := h.stateCtr.GetValue(); current != nil {
		if bytes.Equal(current.GetConfig().GetConfigChainHash(), newHash) {
			return nil
		}
		if _, err := sobject.VerifyConfigChange(h.soID, current.GetConfig(), entry); err != nil {
			return err
		}
	}

	// Derive the next effective head and this peer's remaining membership.
	nextCfg := entry.GetConfig().CloneVT()
	nextCfg.ConfigChainHash = bytes.Clone(newHash)
	nextCfg.ConfigChainSeqno = entry.GetConfigSeqno()
	localPeerIDStr := h.peerID.String()
	var localFound bool
	for _, participant := range nextCfg.GetParticipants() {
		if participant.GetPeerId() == localPeerIDStr {
			localFound = true
			break
		}
	}

	// Construct the complete cache update before changing held state.
	next := h.stateWithVerifiedConfig(h.stateCtr.GetValue(), nextCfg)
	cache := h.buildVerifiedStateCache()
	if cache == nil {
		cache = &api.VerifiedSOStateCache{}
	}
	cache.CurrentConfig = nextCfg.CloneVT()
	cache.VerifiedConfigChainHash = bytes.Clone(newHash)
	cache.VerifiedConfigChainSeqno = entry.GetConfigSeqno()
	cache.ConfigHistory = append(cache.ConfigHistory, entry.CloneVT())
	if epoch != nil {
		cache.KeyEpochs = mergeSOKeyEpochs(cache.KeyEpochs, epoch)
	}

	// Carry the new invites and the epoch into the next state.
	if next != nil {
		if nextInvites != nil {
			next.Invites = cloneVTSlice(nextInvites)
		}
		if epoch != nil {
			next.SetKeyEpoch(epoch.CloneVT())
		}
		if h.peerState != nil {
			cache.PeerState = next.CloneVT()
		}
	}

	// Persist the cache before publishing it.
	if h.persistVerifiedStateCache != nil {
		if err := h.persistVerifiedStateCache(ctx, cache); err != nil {
			return err
		}
	}

	// State, history and trusted metadata become visible together after persistence.
	h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		h.verifiedConfig = cache.CurrentConfig
		h.configHistory = cache.ConfigHistory
		h.historyIndex = indexConfigHistory(cache.ConfigHistory)
		h.lastConfigChainHash = cache.VerifiedConfigChainHash
		h.verifiedConfigChainSeqno = cache.VerifiedConfigChainSeqno
		h.keyEpochs = cache.KeyEpochs
		h.peerState = cache.PeerState
		if next != nil {
			h.stateCtr.SetValue(next)
		}
		broadcast()
	})

	// A peer removed from the object stops hosting it.
	if !localFound && h.ctxCancel != nil {
		h.ctxCancel()
	}
	return nil
}

// AccessSharedObjectState returns the raw SOState container.
func (h *cloudSOHost) AccessSharedObjectState(ctx context.Context, released func()) (ccontainer.Watchable[*sobject.SOState], func(), error) {
	return h.soHost.GetSOStateCtr(ctx, released)
}

// AccessSharedObjectSnapshot returns the derived SharedObjectStateSnapshot container.
func (h *cloudSOHost) AccessSharedObjectSnapshot() ccontainer.Watchable[sobject.SharedObjectStateSnapshot] {
	return h.snapCtr
}

// triggerConfigChanged sends a non-blocking signal to the config chain verifier.
func (h *cloudSOHost) triggerConfigChanged() {
	if h.configChangedRoutine != nil {
		h.configChangedRoutine.Trigger()
	}
}

// handleConfigChanged fetches and verifies the config chain after a config change notification.
func (h *cloudSOHost) handleConfigChanged(ctx context.Context) {
	// Read state and last hash atomically to avoid split-state-read.
	var newHash []byte
	var newSeqno uint64
	var changed bool
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		state := h.stateCtr.GetValue()
		if state != nil {
			newHash = state.GetConfig().GetConfigChainHash()
			newSeqno = state.GetConfig().GetConfigChainSeqno()
		}
		changed = shouldSyncVerifiedConfigChain(
			newHash,
			newSeqno,
			h.lastConfigChainHash,
			h.verifiedConfigChainSeqno,
		)
	})

	// Join the existing verifier request when the observed head is newer.
	if changed {
		if err := h.syncConfigChainSingleflight(ctx, newHash); err != nil {
			if ctx.Err() != nil {
				return
			}
			h.le.WithError(err).Warn("failed to sync config chain")
			return
		}
	}

	// Commit a departure the verified head hands to this peer, promoting this
	// account's other peers so any of its sessions can sign afterward.
	var promote []string
	for _, p := range h.stateCtr.GetValue().GetConfig().GetParticipants() {
		if p.GetEntityId() == h.selfEntityID && p.GetPeerId() != h.peerID.String() {
			promote = append(promote, p.GetPeerId())
		}
	}
	if _, err := sobject.CompleteSOOwnershipTransfer(ctx, h.soHost, h.privKey, promote...); err != nil && ctx.Err() == nil {
		h.le.WithError(err).Warn("failed to complete ownership transfer")
	}
}

// syncConfigChainSingleflight runs syncConfigChain behind chainSeed so the
// pullState inline recovery and the configChangedRoutine handler share one
// in-flight /config-chain fetch and observe the same outcome. Both callers
// drive verification toward the latest hash on the cached SOState; collisions
// across slightly different hashes are benign because the next config_changed
// signal will trigger another sync if the verifier is still behind.
func (h *cloudSOHost) syncConfigChainSingleflight(ctx context.Context, newHash []byte) error {
	return h.chainSeed.Run(ctx, &h.bcast, func(ctx context.Context) error {
		return h.syncConfigChain(ctx, newHash)
	})
}

// syncConfigChain fetches and verifies the latest config chain for the given hash.
func (h *cloudSOHost) syncConfigChain(ctx context.Context, newHash []byte) error {
	// Fetch history only when the response advertises a configuration head.
	if len(newHash) == 0 {
		return nil
	}

	chainData, err := h.client.GetConfigChain(ctx, h.soID)
	if err != nil {
		return errors.Wrap(err, "fetch config chain")
	}

	// Verify the decoded history through the shared response acceptance path.
	resp := &sobject.SOConfigChainResponse{}
	if err := resp.UnmarshalVT(chainData); err != nil {
		return errors.Wrap(err, "parse config chain response")
	}
	return h.syncConfigChainResponse(ctx, resp, newHash)
}

// syncConfigChainResponse verifies pinned chain continuity and records the resulting authority.
func (h *cloudSOHost) syncConfigChainResponse(
	ctx context.Context,
	resp *sobject.SOConfigChainResponse,
	newHash []byte,
) error {
	// Verify the chain from genesis.
	entries := resp.GetConfigChanges()
	if err := sobject.VerifyConfigChain(h.soID, entries); err != nil {
		return errors.Wrap(err, "verify config chain")
	}
	if len(entries) == 0 {
		return nil
	}

	// Serialize trusted head and durable lineage updates with peer imports.
	release, err := h.acceptMu.Lock(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if release != nil {
			release()
		}
	}()

	// D5: Pin genesis hash on first chain fetch. On subsequent fetches,
	// verify the genesis entry has not been replaced (chain replacement attack).
	genesisEntryHash, err := sobject.HashSOConfigChange(entries[0])
	if err != nil {
		return errors.Wrap(err, "hash genesis config change")
	}
	var genesisHash []byte
	var genesisMismatch bool
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		genesisHash = bytes.Clone(h.genesisHash)
		if len(genesisHash) == 0 {
			genesisHash = bytes.Clone(genesisEntryHash)
		}
		genesisMismatch = !bytes.Equal(genesisEntryHash, genesisHash)
	})
	if genesisMismatch {
		if h.ctxCancel != nil {
			h.ctxCancel()
		}
		return errors.New("genesis config change hash mismatch, possible chain replacement attack")
	}

	// D6: Verify the state's config_chain_hash matches the hash of the
	// last chain entry. Prevents the server from acknowledging a chain
	// but serving state derived from a different (forked) chain.
	lastEntryHash, err := sobject.HashSOConfigChange(entries[len(entries)-1])
	if err != nil {
		return errors.Wrap(err, "hash last config chain entry")
	}
	if !bytes.Equal(lastEntryHash, newHash) {
		return errors.New("config chain hash mismatch: state hash does not match last chain entry")
	}

	// Refuse stale cloud history after a newer peer head has been accepted.
	lastEntry := entries[len(entries)-1]
	if lastEntry.GetConfigSeqno() < h.verifiedConfigChainSeqno {
		return nil
	}
	if lastEntry.GetConfigSeqno() == h.verifiedConfigChainSeqno && len(h.lastConfigChainHash) != 0 && !bytes.Equal(newHash, h.lastConfigChainHash) {
		return errors.New("config chain conflicts with accepted head")
	}

	// Pin the accepted intermediate head as well as genesis to reject later forks.
	if len(h.lastConfigChainHash) != 0 {
		var extendsHeld bool
		for _, entry := range entries {
			if entry.GetConfigSeqno() != h.verifiedConfigChainSeqno {
				continue
			}
			hash, err := sobject.HashSOConfigChange(entry)
			if err != nil {
				return err
			}
			extendsHeld = bytes.Equal(hash, h.lastConfigChainHash)
			break
		}
		if !extendsHeld {
			return errors.New("cloud configuration history does not extend held checkpoint")
		}
	}

	// Prepare the complete durable configuration checkpoint.
	latestConfig := lastEntry.GetConfig().CloneVT()
	latestConfig.ConfigChainHash = bytes.Clone(newHash)
	latestConfig.ConfigChainSeqno = lastEntry.GetConfigSeqno()
	cache := &api.VerifiedSOStateCache{
		GenesisHash:              genesisHash,
		VerifiedConfigChainHash:  bytes.Clone(newHash),
		VerifiedConfigChainSeqno: entries[len(entries)-1].GetConfigSeqno(),
		KeyEpochs:                cloneVTSlice(resp.GetKeyEpochs()),
		PeerState:                h.peerState.CloneVT(),
		ConfigHistory:            cloneVTSlice(entries),
	}
	if latestConfig != nil {
		cache.CurrentConfig = latestConfig.CloneVT()
	}

	// Project configuration-only changes immediately, including local revocation.
	nextState := h.stateWithVerifiedConfig(h.stateCtr.GetValue(), latestConfig)
	if h.peerState != nil {
		cache.PeerState = nextState.CloneVT()
	}

	// Commit verified lineage before making the new authority observable.
	if h.persistVerifiedStateCache != nil {
		if err := h.persistVerifiedStateCache(ctx, cache); err != nil {
			return errors.Wrap(err, "persist verified config chain")
		}
	}

	// Store epochs and update the last known config chain hash.
	var rotate bool
	h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Rotate the key at most once per configuration head.
		rotate = !bytes.Equal(h.rotatedConfigChainHash, newHash)

		// Adopt the verified peer state and projected state.
		h.genesisHash = cache.GenesisHash
		h.peerState = cache.PeerState
		if nextState != nil {
			h.stateCtr.SetValue(nextState)
		}

		// Record the verified chain head and its history.
		h.configHistory = cache.ConfigHistory
		h.historyIndex = indexConfigHistory(cache.ConfigHistory)
		h.lastConfigChainHash = bytes.Clone(newHash)
		h.verifiedConfigChainSeqno = entries[len(entries)-1].GetConfigSeqno()

		// Publish the epochs and latest configuration to waiters.
		h.keyEpochs = cloneVTSlice(resp.GetKeyEpochs())
		if latestConfig != nil {
			h.verifiedConfig = latestConfig.CloneVT()
		}
		broadcast()
	})
	release()
	release = nil

	// Check if local peer is still in the participant list.
	localPeerIDStr := h.peerID.String()
	var localFound bool
	var localIsOwner bool
	for _, p := range latestConfig.GetParticipants() {
		if p.GetPeerId() == localPeerIDStr {
			localFound = true
			localIsOwner = sobject.IsOwner(p.GetRole())
			break
		}
	}
	if !localFound {
		if sobject.CanReadState(
			readableParticipantRoleForEntity(latestConfig, h.selfEntityID),
		) {
			return nil
		}
		if h.ctxCancel != nil {
			h.ctxCancel()
		}
		return sobject.ErrNotParticipant
	}

	// If local peer is OWNER, check whether the head removed participants by
	// comparing the previous and latest config entries. This is computed
	// locally rather than trusting the server-supplied changeType field. Every
	// state pull re-verifies the same head, which rotates the key only once.
	if rotate && localIsOwner && len(entries) >= 2 {
		prevParticipants := entries[len(entries)-2].GetConfig().GetParticipants()
		currParticipants := latestConfig.GetParticipants()
		if participantsRemoved(prevParticipants, currParticipants) && h.rotateKeyOnRevocation(ctx, currParticipants) {
			h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				h.rotatedConfigChainHash = bytes.Clone(newHash)
			})
		}
	}

	// Client-side revocation enforcement: scan recent config chain entries for
	// REMOVE_PARTICIPANT changes with RevocationInfo and notify via callback.
	if h.onPeerRevoked != nil {
		for _, entry := range entries {
			if entry.GetChangeType() != sobject.SOConfigChangeType_SO_CONFIG_CHANGE_TYPE_REMOVE_PARTICIPANT {
				continue
			}
			revInfo := entry.GetRevocationInfo()
			if revInfo == nil {
				continue
			}

			// Determine which peer was removed by diffing this entry's config
			// against the previous entry's config.
			if entry.GetConfigSeqno() == 0 {
				continue
			}
			if entry.GetConfigSeqno() > uint64(len(entries)) {
				continue
			}
			prevIdx := int(entry.GetConfigSeqno()) - 1 //nolint:gosec // the preceding sequence bound makes this a valid slice-index conversion.
			if prevIdx < 0 || prevIdx >= len(entries) {
				continue
			}
			prevPeers := entries[prevIdx].GetConfig().GetParticipants()
			currPeers := entry.GetConfig().GetParticipants()
			currSet := make(map[string]struct{}, len(currPeers))
			for _, p := range currPeers {
				currSet[p.GetPeerId()] = struct{}{}
			}
			for _, p := range prevPeers {
				if _, ok := currSet[p.GetPeerId()]; !ok {
					h.le.WithField("revoked-peer", p.GetPeerId()).
						WithField("reason", revInfo.GetReason().String()).
						Info("peer revoked via config chain")
					h.onPeerRevoked(p.GetPeerId())
				}
			}
		}
	}
	return nil
}

// participantsRemoved returns true if any participant peer IDs present
// in prev are absent in curr (i.e., a participant was removed).
func participantsRemoved(prev, curr []*sobject.SOParticipantConfig) bool {
	// Index current membership before checking for removed peers.
	currIDs := make(map[string]struct{}, len(curr))
	for _, p := range curr {
		currIDs[p.GetPeerId()] = struct{}{}
	}
	for _, p := range prev {
		if _, ok := currIDs[p.GetPeerId()]; !ok {
			return true
		}
	}
	return false
}

// rotateKeyOnRevocation generates a new transform key and posts the epoch to
// the server, reporting whether the rotation completed.
// Note: old epoch grants remain on the server for historical decryption by remaining
// participants. This is by design -- forward secrecy means the revoked participant
// cannot decrypt NEW content, but historical content up to the rotation point remains
// accessible to anyone who had the old key. The server is trusted to serve epochs
// only to authorized participants (via rbac_role_bindings).
func (h *cloudSOHost) rotateKeyOnRevocation(ctx context.Context, participants []*sobject.SOParticipantConfig) bool {
	// Snapshot the current epoch and configuration together.
	var currentEpoch uint64
	var currentCfg *sobject.SharedObjectConfig
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if len(h.keyEpochs) != 0 {
			currentEpoch = h.keyEpochs[len(h.keyEpochs)-1].GetEpoch()
		}
		if st := h.stateCtr.GetValue(); st.GetConfig() != nil {
			currentCfg = st.GetConfig().CloneVT()
		}
	})
	if currentCfg == nil {
		h.le.Warn("failed to rotate transform key: current config missing")
		return false
	}

	// Build the new encryption epoch and authorized recovery envelopes.
	transformConf, epoch, err := sobject.RotateTransformKey(
		h.privKey,
		h.soID,
		participants,
		currentEpoch,
	)
	if err != nil {
		h.le.WithError(err).Warn("failed to rotate transform key")
		return false
	}
	recoveryEnvelopes, err := buildSORecoveryEnvelopes(
		ctx,
		h.client,
		h.soID,
		currentCfg,
		epoch.GetEpoch(),
		&sobject.SOGrantInner{TransformConf: transformConf},
	)
	if err != nil {
		if ctx.Err() != nil {
			return false
		}
		h.le.WithError(err).Warn("failed to build recovery envelopes for key rotation")
		return false
	}

	// Post the new epoch to the server.
	if err := h.client.PostKeyEpoch(
		ctx,
		h.soID,
		epoch,
		recoveryEnvelopes,
	); err != nil {
		if ctx.Err() != nil {
			return false
		}
		h.le.WithError(err).Warn("failed to post key epoch to server")
		return false
	}

	// Project the successful rotation into the accepted local state.
	h.applyKeyEpoch(ctx, epoch)
	h.le.WithField("epoch", epoch.GetEpoch()).Info("key rotation complete after participant revocation")
	return true
}

// GetKeyEpochs returns the current key epochs.
func (h *cloudSOHost) GetKeyEpochs() []*sobject.SOKeyEpoch {
	var epochs []*sobject.SOKeyEpoch
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		epochs = h.keyEpochs
	})
	return epochs
}

// GetSOHost returns the underlying SOHost.
func (h *cloudSOHost) GetSOHost() *sobject.SOHost {
	return h.soHost
}

// buildVerifiedStateCache snapshots the trusted SO config cache for persistence.
func (h *cloudSOHost) buildVerifiedStateCache() *api.VerifiedSOStateCache {
	var cache *api.VerifiedSOStateCache
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if len(h.lastConfigChainHash) == 0 {
			return
		}
		cache = &api.VerifiedSOStateCache{
			GenesisHash:              bytes.Clone(h.genesisHash),
			VerifiedConfigChainHash:  bytes.Clone(h.lastConfigChainHash),
			VerifiedConfigChainSeqno: h.verifiedConfigChainSeqno,
			KeyEpochs:                cloneVTSlice(h.keyEpochs),
			PeerState:                h.peerState.CloneVT(),
			ConfigHistory:            cloneVTSlice(h.configHistory),
			PendingPublication:       h.pending.CloneVT(),
			CloudState:               h.cloudState.CloneVT(),
			CloudSequence:            h.lastSeqno,
		}
		config := h.verifiedConfig
		if config == nil {
			if st := h.stateCtr.GetValue(); st != nil {
				config = st.GetConfig()
			}
		}
		if config != nil {
			cache.CurrentConfig = config.CloneVT()
		}
	})
	return cache
}

// shouldSyncVerifiedConfigChain returns true when the verified local chain is
// missing or behind the current SO state snapshot.
func shouldSyncVerifiedConfigChain(
	currentHash []byte,
	currentSeqno uint64,
	verifiedHash []byte,
	verifiedSeqno uint64,
) bool {
	if len(currentHash) == 0 {
		return false
	}
	if len(verifiedHash) == 0 {
		return true
	}
	if currentSeqno > verifiedSeqno {
		return true
	}
	if currentSeqno < verifiedSeqno {
		return false
	}
	return !bytes.Equal(currentHash, verifiedHash)
}

// hydrateVerifiedStateCache loads persisted verified SO config state into memory.
func (h *cloudSOHost) hydrateVerifiedStateCache(cache *api.VerifiedSOStateCache) {
	// Preserve an empty host when no durable checkpoint exists.
	if cache == nil {
		return
	}

	// Restore a valid snapshot even when its participant projection is reordered.
	h.configHistory = cloneVTSlice(cache.GetConfigHistory())
	h.historyIndex = indexConfigHistory(h.configHistory)
	h.peerState = cache.GetPeerState().CloneVT()
	h.pending = cache.GetPendingPublication().CloneVT()
	h.cloudState = cache.GetCloudState().CloneVT()
	h.lastSeqno = cache.GetCloudSequence()
	if h.peerState != nil && sobject.EqualSOConfigs(h.peerState.GetConfig(), cache.GetCurrentConfig()) && h.peerState.Validate(h.soID) == nil {
		h.stateCtr.SetValue(h.peerState.CloneVT())
	}

	// Restore the exact signed lineage and encryption epochs independently of order.
	h.genesisHash = bytes.Clone(cache.GetGenesisHash())
	h.lastConfigChainHash = bytes.Clone(cache.GetVerifiedConfigChainHash())
	h.verifiedConfigChainSeqno = cache.GetVerifiedConfigChainSeqno()
	h.keyEpochs = cloneVTSlice(cache.GetKeyEpochs())
	if cache.GetCurrentConfig() != nil {
		h.verifiedConfig = cache.GetCurrentConfig().CloneVT()
	}
}
