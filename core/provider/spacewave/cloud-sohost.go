package provider_spacewave

import (
	"bytes"
	"context"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// maxWriteRetries is the maximum number of retries on 409 write conflicts.
const maxWriteRetries = 3

// errSOStateDeltaResponse indicates the HTTP SO state response carried a delta.
var errSOStateDeltaResponse = errors.New("so state response returned delta")

// errSOConfigChainChanged indicates the pulled state uses a newer config chain hash.
var errSOConfigChainChanged = errors.New("config chain hash changed")

// cloudSOHost implements the shared object state management via the wsTracker.
type cloudSOHost struct {
	// le is the logger
	le *logrus.Entry
	// client is the session client for API calls
	client *SessionClient
	// soID is the shared object ID
	soID string
	// selfEntityID is the stable entity id for the mounted account.
	selfEntityID string
	// privKey is the session private key for signing
	privKey crypto.PrivKey
	// peerID is the local peer ID derived from privKey
	peerID peer.ID
	// sfs is the step factory set for block transforms
	sfs *block_transform.StepFactorySet
	// tracker is the shared session WebSocket tracker
	tracker *wsTracker
	// soHost is the SOHost managing the refcount
	soHost *sobject.SOHost
	// stateCtr contains the current shared object state
	stateCtr *ccontainer.CContainer[*sobject.SOState]
	// snapCtr contains the derived SharedObjectStateSnapshot
	snapCtr *ccontainer.CContainer[sobject.SharedObjectStateSnapshot]
	// lastSeqno tracks the last applied change_log sequence number.
	lastSeqno uint64
	// lastConfigChainHash tracks the last known config chain hash
	lastConfigChainHash []byte
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
	// bcast guards lastSeqno and stateCtr updates
	bcast broadcast.Broadcast
	// writeMu serializes local writes to prevent self-nonce conflicts
	writeMu csync.Mutex
	// pullCh signals the pull goroutine to fetch fresh state
	pullCh chan struct{}
	// pullSeed coordinates concurrent pullState callers (cold seed in
	// Execute, lockFn cold fallback, gap recovery in runPuller, write
	// retries, op-queue cold fallback) so they share one in-flight HTTP
	// fetch and observe the same error. Guarded by bcast.
	pullSeed providerSeed
	// chainSeed coordinates concurrent syncConfigChain callers
	// (pullState inline recovery and runConfigChainVerifier handler)
	// so a single /config-chain fetch covers both verifier triggers.
	// Guarded by bcast.
	chainSeed providerSeed
	// configChangedCh signals that config_chain_hash changed and needs verification
	configChangedCh chan struct{}
	// ctxCancel cancels the host context (used for D11 teardown on participant removal)
	ctxCancel context.CancelFunc
	// onPeerRevoked is called when a peer is removed from the config chain with
	// RevocationInfo. Called with the revoked peer ID string.
	onPeerRevoked func(peerIDStr string)
	// forceBlockSync flushes pending block-store writes before publishing roots.
	forceBlockSync func(ctx context.Context) error
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
	forceBlockSync func(ctx context.Context) error,
) *cloudSOHost {
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
		pullCh:                    make(chan struct{}, 1),
		configChangedCh:           make(chan struct{}, 1),
		persistVerifiedStateCache: persistVerifiedStateCache,
		forceBlockSync:            forceBlockSync,
	}
	h.hydrateVerifiedStateCache(verifiedCache)

	// watchFn returns the stateCtr which is updated by pull-on-notify.
	watchFn := func(ctx context.Context, sharedObjectID string, released func()) (ccontainer.Watchable[*sobject.SOState], func(), error) {
		return h.stateCtr, func() {}, nil
	}

	// lockFn acquires the local write mutex and reads from the cached stateCtr.
	// WriteSOState does HTTP POST with 409 retry.
	lockFn := func(ctx context.Context, sharedObjectID string) (sobject.SOStateLock, error) {
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

		initialState := state.CloneVT()
		writeFn := func(ctx context.Context, state *sobject.SOState) error {
			return h.writeStateWithRetry(ctx, state)
		}

		return sobject.NewSOStateLock(initialState, writeFn, relLock), nil
	}

	// Pass nil context; SetContext called in Execute.
	h.soHost = sobject.NewSOHost(nil, watchFn, lockFn, soID)
	return h
}

// Execute runs the cloudSOHost lifecycle.
func (h *cloudSOHost) Execute(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	h.ctxCancel = cancel
	defer cancel()

	h.soHost.SetContext(ctx)

	h.tracker.RegisterNotifyCallback(h.soID, h.handleSONotify)
	defer h.tracker.UnregisterNotifyCallback(h.soID)

	// Pull goroutine: processes pull signals without blocking the WS read loop.
	go h.runPuller(ctx)

	// Snapshot derivation: watches stateCtr and updates snapCtr.
	go h.runSnapDeriver(ctx)

	// Config chain verifier: checks config chain when hash changes.
	go h.runConfigChainVerifier(ctx)

	// Seed the local SO state immediately so first mount does not depend on a
	// later websocket notification to populate the state containers.
	if err := h.ensureInitialState(ctx, SeedReasonColdSeed); err != nil {
		if ctx.Err() != nil {
			return context.Canceled
		}
		return errors.Wrap(err, "initial state pull")
	}

	<-ctx.Done()
	return context.Canceled
}

// ensureInitialState populates stateCtr once before read/write/watch paths run.
// If stateCtr already holds a value from an earlier mount-time seed, it reuses
// that snapshot instead of issuing another HTTP GET.
func (h *cloudSOHost) ensureInitialState(ctx context.Context, reason SeedReason) error {
	if h.stateCtr.GetValue() != nil {
		return nil
	}
	if err := h.pullStateSingleflight(ctx, reason); err != nil {
		return err
	}
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
func (h *cloudSOHost) runSnapDeriver(ctx context.Context) {
	var prev *sobject.SOState
	for {
		next, err := h.stateCtr.WaitValueChange(ctx, prev, nil)
		if err != nil {
			return
		}
		prev = next
		snap := sobject.NewSOStateParticipantHandle(
			h.le,
			h.sfs,
			h.soID,
			next,
			h.privKey,
			h.peerID,
		)
		h.snapCtr.SetValue(snap)
	}
}

// runPuller listens for pull signals and fetches fresh state via HTTP GET.
// With event-carried SO state deltas in place, the only callers that signal
// pullCh are gap-recovery paths (inline delta apply failed because the
// cache is behind by more than the cloud retains). Cold seed and write
// retry paths call pullState directly rather than queueing a signal.
func (h *cloudSOHost) runPuller(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.pullCh:
			if err := h.pullStateSingleflight(ctx, SeedReasonGapRecovery); err != nil {
				if ctx.Err() != nil {
					return
				}
				h.le.WithError(err).Warn("failed to pull state on notify")
			}
		}
	}
}

// pullStateSingleflight runs pullState behind providerSeed so concurrent
// callers (cold-seed in Execute, lockFn no-state fallback, gap recovery in
// runPuller, write conflict retry, op queue cold fallback) share one
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
	var since uint64
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		since = h.lastSeqno
	})
	stateData, err := h.client.GetSOState(ctx, h.soID, since, reason)
	if err != nil {
		return err
	}

	state, lastSeqno, err := decodeSOStateResponse(stateData)
	if errors.Is(err, errSOStateDeltaResponse) && since > 0 {
		h.le.WithField("since", since).Debug("received delta from state pull, retrying full snapshot")
		stateData, err = h.client.GetSOState(ctx, h.soID, 0, SeedReasonGapRecovery)
		if err != nil {
			return err
		}
		state, lastSeqno, err = decodeSOStateResponse(stateData)
	}
	if err != nil {
		return err
	}
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

	// D10: Client-side state verification before accepting.
	if err := h.verifyPulledState(state); err != nil {
		if errors.Is(err, errSOConfigChainChanged) {
			if syncErr := h.syncConfigChainSingleflight(ctx, state.GetConfig().GetConfigChainHash()); syncErr != nil {
				h.le.WithError(syncErr).Warn("failed to verify updated config chain for pulled state")
				return nil
			}
			if err := h.verifyPulledState(state); err != nil {
				h.noteInitialStateRejection(
					errors.Wrapf(
						errSharedObjectInitialStateRejected,
						"state verification after config sync: %v",
						err,
					),
				)
				h.le.WithError(err).Warn("pulled state failed verification after config sync, ignoring")
				return nil
			}
		} else {
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
	}

	var configHashChanged bool
	var prevState *sobject.SOState
	h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		prevState = h.stateCtr.GetValue()
		h.initialStateErr = nil
		if lastSeqno > h.lastSeqno {
			h.lastSeqno = lastSeqno
		}
		newHash := state.GetConfig().GetConfigChainHash()
		if !bytes.Equal(newHash, h.lastConfigChainHash) && len(newHash) > 0 {
			configHashChanged = true
		}
		h.stateCtr.SetValue(state)
		broadcast()
	})
	h.logNewOpRejections(prevState, state, "state-pull")

	// If the config chain hash changed, trigger verification.
	if configHashChanged {
		h.triggerConfigChanged()
	}

	return nil
}

// noteInitialStateRejection records a rejected cold-seed verification outcome
// so ensureInitialState can return a terminal mount error instead of falling
// back to a generic retryable "no state available after pull" error.
func (h *cloudSOHost) noteInitialStateRejection(err error) {
	if err == nil || h.stateCtr.GetValue() != nil {
		return
	}
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		h.initialStateErr = err
	})
}

// decodeSOStateResponse decodes an SOStateMessage snapshot or delta marker.
func decodeSOStateResponse(data []byte) (*sobject.SOState, uint64, error) {
	msg := &api.SOStateMessage{}
	if err := msg.UnmarshalVT(data); err != nil {
		return nil, 0, errors.Wrap(err, "unmarshal SOStateMessage")
	}
	if snap := msg.GetSnapshot(); snap != nil {
		return snap, msg.GetSeqno(), nil
	}
	if msg.GetDelta() != nil {
		return nil, 0, errSOStateDeltaResponse
	}
	return nil, 0, errors.New("missing snapshot or delta in SOStateMessage")
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
// Checks config chain hash continuity and root signature validity.
func (h *cloudSOHost) verifyPulledState(state *sobject.SOState) error {
	root := state.GetRoot()
	if root == nil {
		// No root yet (uninitialized state), allow it.
		return nil
	}
	var lastConfigHash []byte
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		lastConfigHash = h.lastConfigChainHash
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

	// Verify root has at least one validator signature.
	if len(root.GetValidatorSignatures()) == 0 && root.GetInnerSeqno() > 0 {
		return errors.New("root missing validator signatures")
	}

	// Verify root signatures are from VALIDATOR/OWNER participants.
	participants := state.GetConfig().GetParticipants()
	validSigs, err := root.ValidateSignatures(h.soID, participants)
	if err != nil {
		return errors.Wrap(err, "root signature validation")
	}

	// Check consensus acceptance based on the configured mode.
	if err := sobject.CheckConsensusAcceptance(state.GetConfig().GetConsensusMode(), validSigs); err != nil {
		return errors.Wrap(err, "consensus acceptance")
	}

	// Verify op signatures are from WRITER+ role participants.
	for i, op := range state.GetOps() {
		if err := op.ValidateSignature(h.soID, participants); err != nil {
			return errors.Wrapf(err, "op[%d] signature validation", i)
		}
	}

	return nil
}

// triggerPull sends a non-blocking signal to the pull goroutine. Use this
// only for gap-recovery cases where an inline state apply failed; cold-seed
// and write-retry callers should invoke pullState directly so the result is
// observable inline.
func (h *cloudSOHost) triggerPull() {
	select {
	case h.pullCh <- struct{}{}:
	default:
	}
}

// handleSONotify processes an SONotifyEventPayload delivered via so_notify.
// When the payload carries an inline SOStateMessage, the host applies it
// directly without issuing an HTTP /state pull. configChanged events trigger
// a config-chain re-verify only; the state pull fallback only fires for
// genuine gap or cold-cache cases.
func (h *cloudSOHost) handleSONotify(payload *api.SONotifyEventPayload) {
	if payload == nil {
		return
	}

	if msg := payload.GetStateMessage(); msg != nil {
		if err := h.handleStateDelta(msg); err != nil {
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

	switch payload.GetChangeType() {
	case "configChanged":
		h.triggerConfigChanged()
	case "metadata":
		// Account-level notification handling owns metadata cache updates.
	case "delete":
		// SO is being deleted; no inline state to apply.
	default:
		// Bare notify with no state payload. Cloud should always attach an
		// inline SOStateMessage for op/root mutations, so this indicates a
		// publisher bug rather than a missed update we should pull behind.
		h.le.WithField("change-type", payload.GetChangeType()).
			WithField("seqno", payload.GetSeqno()).
			Warn("so_notify arrived without inline state payload; ignoring")
	}
}

// handleStateDelta applies an inline SOStateMessage from a session WS event.
// Snapshots replace stateCtr after verifyPulledState succeeds; deltas are
// applied entry-by-entry against a clone of the cached state and committed
// atomically with the new lastSeqno. Returns an error when the delta cannot
// be applied (gap, decode failure, or verification failure); the caller may
// fall back to an HTTP pull.
func (h *cloudSOHost) handleStateDelta(msg *api.SOStateMessage) error {
	switch {
	case msg.GetSnapshot() != nil:
		snap := msg.GetSnapshot()
		if err := h.verifyChangeLogSeqno(msg.GetSeqno()); err != nil {
			return errors.Wrap(err, "verify inline snapshot seqno")
		}
		if err := h.verifyPulledState(snap); err != nil {
			return errors.Wrap(err, "verify inline snapshot")
		}
		var prevState *sobject.SOState
		h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			prevState = h.stateCtr.GetValue()
			if msg.GetSeqno() >= h.lastSeqno {
				h.stateCtr.SetValue(snap)
				h.lastSeqno = msg.GetSeqno()
				broadcast()
			}
		})
		h.logNewOpRejections(prevState, snap, "inline-snapshot")
		return nil

	case msg.GetDelta() != nil:
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
			cached = h.stateCtr.GetValue()
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

		if err := h.verifyPulledState(next); err != nil {
			return errors.Wrap(err, "verify state after delta apply")
		}

		newSeqno := entries[len(entries)-1].GetSeqno()
		var prevState *sobject.SOState
		h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			// Re-check under lock to avoid clobbering a concurrent update.
			if h.lastSeqno != lastSeqno {
				return
			}
			prevState = h.stateCtr.GetValue()
			h.stateCtr.SetValue(next)
			h.lastSeqno = newSeqno
			broadcast()
		})
		h.logNewOpRejections(prevState, next, "inline-delta")
		return nil

	case msg.GetConfigChanged() != nil:
		h.triggerConfigChanged()
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
// a full SOOperation envelope and 'root' carries a PostRootRequest. Entries are
// idempotent; replays for already-known (peer_id, nonce) ops are dropped.
func applyChangeLogEntry(
	sharedObjectID string,
	state *sobject.SOState,
	entry *api.SOStateDeltaEntry,
) error {
	switch entry.GetChangeType() {
	case "op":
		op := &sobject.SOOperation{}
		if err := op.UnmarshalVT(entry.GetChangeData()); err != nil {
			return errors.Wrap(err, "unmarshal op envelope")
		}
		inner, err := op.UnmarshalInner()
		if err != nil {
			return errors.Wrap(err, "unmarshal op inner")
		}
		peerID := inner.GetPeerId()
		nonce := inner.GetNonce()
		for _, existing := range state.GetOps() {
			existingInner, err := existing.UnmarshalInner()
			if err != nil {
				continue
			}
			if existingInner.GetPeerId() == peerID && existingInner.GetNonce() == nonce {
				return nil
			}
		}
		state.Ops = append(state.Ops, op)
		return nil

	case "root":
		req := &api.PostRootRequest{}
		if err := req.UnmarshalVT(entry.GetChangeData()); err != nil {
			return errors.Wrap(err, "unmarshal post root request")
		}
		if req.GetRoot() == nil {
			return errors.New("post root request missing root")
		}
		if currentRoot := state.GetRoot(); currentRoot != nil && currentRoot.EqualVT(req.GetRoot()) {
			return nil
		}

		accepted := make(map[string]uint64, len(req.GetRoot().GetAccountNonces()))
		for _, acc := range req.GetRoot().GetAccountNonces() {
			accepted[acc.GetPeerId()] = acc.GetNonce()
		}

		acceptedOps := make([]*sobject.SOOperation, 0, len(state.GetOps()))
		for _, existing := range state.GetOps() {
			existingInner, err := existing.UnmarshalInner()
			if err != nil {
				continue
			}
			limit, ok := accepted[existingInner.GetPeerId()]
			if ok && existingInner.GetNonce() <= limit {
				acceptedOps = append(acceptedOps, existing)
			}
		}

		if err := state.UpdateRootState(
			sharedObjectID,
			req.GetRoot(),
			"",
			req.GetRejectedOps(),
			acceptedOps,
		); err != nil {
			return errors.Wrap(err, "update root state")
		}
		return nil

	default:
		return errors.Errorf("unknown change_type %q", entry.GetChangeType())
	}
}

func diffSOOperationRejections(
	prevState *sobject.SOState,
	nextState *sobject.SOState,
) []*sobject.SOOperationRejection {
	if nextState == nil {
		return nil
	}

	prevKeys := make(map[string]struct{})
	addSOOperationRejectionKeys(prevKeys, prevState)

	var nextRejections []*sobject.SOOperationRejection
	for _, peerRejections := range nextState.GetOpRejections() {
		for _, rejection := range peerRejections.GetRejections() {
			key := buildSOOperationRejectionKey(rejection)
			if _, ok := prevKeys[key]; ok {
				continue
			}
			nextRejections = append(nextRejections, rejection)
		}
	}
	return nextRejections
}

func addSOOperationRejectionKeys(keys map[string]struct{}, state *sobject.SOState) {
	if state == nil {
		return
	}

	for _, peerRejections := range state.GetOpRejections() {
		for _, rejection := range peerRejections.GetRejections() {
			keys[buildSOOperationRejectionKey(rejection)] = struct{}{}
		}
	}
}

func buildSOOperationRejectionKey(rejection *sobject.SOOperationRejection) string {
	if rejection == nil {
		return ""
	}
	return string(rejection.GetInner()) + "\x00" + string(rejection.GetSignature().GetSigData())
}

func (h *cloudSOHost) logNewOpRejections(
	prevState *sobject.SOState,
	nextState *sobject.SOState,
	source string,
) {
	for _, rejection := range diffSOOperationRejections(prevState, nextState) {
		rejInner, err := rejection.UnmarshalInner()
		if err != nil {
			h.le.WithError(err).
				WithField("source", source).
				Warn("failed to decode shared object rejection")
			continue
		}

		le := h.le.WithFields(logrus.Fields{
			"source":               source,
			"rejected-op-local-id": rejInner.GetLocalId(),
			"rejected-op-nonce":    rejInner.GetOpNonce(),
			"rejected-peer-id":     rejInner.GetPeerId(),
		})

		if rejInner.GetPeerId() == h.peerID.String() {
			validatorPubKey, err := rejection.GetSignature().ParsePubKey()
			if err == nil {
				validatorPeerID, err := peer.IDFromPublicKey(validatorPubKey)
				if err == nil {
					errorDetails, err := rejInner.DecodeErrorDetails(
						h.privKey,
						h.soID,
						validatorPeerID,
					)
					if err == nil && errorDetails != nil && errorDetails.GetErrorMsg() != "" {
						le = le.WithField("error", errorDetails.GetErrorMsg())
					}
				}
			}
		}

		le.Warn("observed shared object operation rejection")
	}
}

// writeStateWithRetry posts the root state to the server.
// On 409 (seqno conflict): pull fresh state, rebuild, retry up to maxWriteRetries times.
func (h *cloudSOHost) writeStateWithRetry(ctx context.Context, state *sobject.SOState) error {
	for attempt := range maxWriteRetries {
		if h.forceBlockSync != nil {
			started := time.Now()
			h.le.WithField("attempt", attempt+1).Debug("flushing block store before root write")
			if err := h.forceBlockSync(ctx); err != nil {
				return errors.Wrap(err, "flush block store before root write")
			}
			h.le.WithField("attempt", attempt+1).
				WithField("duration", time.Since(started)).
				Debug("flushed block store before root write")
		}

		prevState := h.stateCtr.GetValue()
		rejectedOps := diffSOOperationRejections(prevState, state)

		started := time.Now()
		err := h.client.PostRoot(ctx, h.soID, state.GetRoot(), rejectedOps)
		if err == nil {
			h.le.WithField("attempt", attempt+1).
				WithField("duration", time.Since(started)).
				WithField("so-seqno", state.GetRoot().GetInnerSeqno()).
				Debug("posted root state")
			// Update cached state on successful write.
			h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				h.stateCtr.SetValue(state)
				broadcast()
			})
			h.logNewOpRejections(prevState, state, "root-write")
			return nil
		}

		var ce *cloudError
		if !errors.As(err, &ce) || ce.StatusCode != 409 {
			return err
		}

		h.le.WithField("attempt", attempt+1).Debug("seqno conflict, pulling fresh state for retry")
		if pullErr := h.pullStateSingleflight(ctx, SeedReasonGapRecovery); pullErr != nil {
			return errors.Wrap(pullErr, "pull state after 409")
		}
	}
	return errors.New("write failed after max retries due to seqno conflicts")
}

// applyQueuedOperation updates the cached state with a newly accepted queued op.
// It avoids an immediate read-after-write pull in the common success case.
func (h *cloudSOHost) applyQueuedOperation(op *sobject.SOOperation) {
	h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		st := h.stateCtr.GetValue()
		if st == nil {
			return
		}

		next := st.CloneVT()
		if err := next.QueueOperation(h.soID, op); err != nil {
			h.le.WithError(err).Debug("failed to optimistically apply queued operation")
			return
		}

		h.stateCtr.SetValue(next)
		broadcast()
	})
}

// applyKeyEpoch updates the cached key-epoch state after a successful write.
func (h *cloudSOHost) applyKeyEpoch(ctx context.Context, epoch *sobject.SOKeyEpoch) {
	if epoch == nil {
		return
	}

	h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		h.keyEpochs = mergeSOKeyEpochs(h.keyEpochs, epoch)

		st := h.stateCtr.GetValue()
		if st != nil {
			next := st.CloneVT()
			rootSeqno := next.GetRoot().GetInnerSeqno()
			if rootSeqno >= epoch.GetSeqnoStart() &&
				(epoch.GetSeqnoEnd() == 0 || rootSeqno <= epoch.GetSeqnoEnd()) {
				next.RootGrants = cloneSOGrants(epoch.GetGrants())
			}
			h.stateCtr.SetValue(next)
		}
		broadcast()
	})
	h.persistVerifiedStateCacheSnapshot(ctx)
}

// applyConfigMutation updates the cached state after a successful config-state write.
func (h *cloudSOHost) applyConfigMutation(
	ctx context.Context,
	entry *sobject.SOConfigChange,
	nextInvites []*sobject.SOInvite,
	epoch *sobject.SOKeyEpoch,
) error {
	newHash, err := sobject.HashSOConfigChange(entry)
	if err != nil {
		return errors.Wrap(err, "hash config change")
	}

	nextCfg := entry.GetConfig().CloneVT()
	nextCfg.ConfigChainHash = append([]byte(nil), newHash...)
	nextCfg.ConfigChainSeqno = entry.GetConfigSeqno()

	localPeerIDStr := h.peerID.String()
	var localFound bool
	for _, participant := range nextCfg.GetParticipants() {
		if participant.GetPeerId() == localPeerIDStr {
			localFound = true
			break
		}
	}

	h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		h.lastConfigChainHash = append([]byte(nil), newHash...)
		h.verifiedConfigChainSeqno = entry.GetConfigSeqno()
		if epoch != nil {
			h.keyEpochs = mergeSOKeyEpochs(h.keyEpochs, epoch)
		}

		st := h.stateCtr.GetValue()
		if st != nil {
			next := st.CloneVT()
			next.Config = nextCfg
			if nextInvites != nil {
				next.Invites = cloneSOInvites(nextInvites)
			}
			if epoch != nil {
				rootSeqno := next.GetRoot().GetInnerSeqno()
				if rootSeqno >= epoch.GetSeqnoStart() &&
					(epoch.GetSeqnoEnd() == 0 || rootSeqno <= epoch.GetSeqnoEnd()) {
					next.RootGrants = cloneSOGrants(epoch.GetGrants())
				}
			}
			h.stateCtr.SetValue(next)
		}
		broadcast()
	})

	if !localFound && h.ctxCancel != nil {
		h.ctxCancel()
	}
	h.persistVerifiedStateCacheSnapshot(ctx)
	return nil
}

// QueueOperation queues an operation to the cloud via the SOHost.
func (h *cloudSOHost) QueueOperation(ctx context.Context, peerID peer.ID, cb func(nonce uint64) (*sobject.SOOperation, error)) error {
	relLock, err := h.writeMu.Lock(ctx)
	if err != nil {
		return err
	}
	defer relLock()

	for attempt := range maxWriteRetries {
		if err := h.ensureInitialState(ctx, SeedReasonColdSeed); err != nil {
			return errors.Wrap(err, "initial state pull for op queue")
		}
		state := h.stateCtr.GetValue()
		if state == nil {
			return errors.New("no state available after pull")
		}

		op, err := cb(state.GetNextAccountNonce(peerID.String()))
		if err != nil {
			return err
		}
		if err := op.Validate(); err != nil {
			return err
		}

		opData, err := op.MarshalVT()
		if err != nil {
			return errors.Wrap(err, "marshal operation")
		}
		if err := h.client.PostOp(ctx, h.soID, opData); err != nil {
			var ce *cloudError
			if !errors.As(err, &ce) || ce.StatusCode != 409 || attempt+1 == maxWriteRetries {
				return err
			}
			h.le.WithFields(logrus.Fields{
				"attempt": attempt + 1,
				"code":    ce.Code,
			}).Debug("op conflict, pulling fresh state for retry")
			if pullErr := h.pullStateSingleflight(ctx, SeedReasonGapRecovery); pullErr != nil {
				return errors.Wrap(pullErr, "pull state after op conflict")
			}
			continue
		}

		h.applyQueuedOperation(op)
		return nil
	}
	return errors.New("queue operation failed after max retries due to write conflicts")
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
	select {
	case h.configChangedCh <- struct{}{}:
	default:
	}
}

// runConfigChainVerifier listens for config changed signals and verifies the config chain.
// If the local peer is removed from the participant list, it cancels the context to tear down the mount.
func (h *cloudSOHost) runConfigChainVerifier(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-h.configChangedCh:
			h.handleConfigChanged(ctx)
		}
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
	if !changed {
		return
	}

	if err := h.syncConfigChainSingleflight(ctx, newHash); err != nil {
		if ctx.Err() != nil {
			return
		}
		h.le.WithError(err).Warn("failed to sync config chain")
	}
}

// syncConfigChainSingleflight runs syncConfigChain behind chainSeed so the
// pullState inline recovery and the runConfigChainVerifier handler share one
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
	if len(newHash) == 0 {
		return nil
	}

	chainData, err := h.client.GetConfigChain(ctx, h.soID)
	if err != nil {
		return errors.Wrap(err, "fetch config chain")
	}

	resp := &sobject.SOConfigChainResponse{}
	if err := resp.UnmarshalVT(chainData); err != nil {
		return errors.Wrap(err, "parse config chain response")
	}

	entries := resp.GetConfigChanges()
	if err := sobject.VerifyConfigChain(entries); err != nil {
		return errors.Wrap(err, "verify config chain")
	}
	if len(entries) == 0 {
		return nil
	}

	// D5: Pin genesis hash on first chain fetch. On subsequent fetches,
	// verify the genesis entry has not been replaced (chain replacement attack).
	genesisEntryHash, err := sobject.HashSOConfigChange(entries[0])
	if err != nil {
		return errors.Wrap(err, "hash genesis config change")
	}
	if len(h.genesisHash) == 0 {
		h.genesisHash = genesisEntryHash
	} else if !bytes.Equal(genesisEntryHash, h.genesisHash) {
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

	latestConfig := entries[len(entries)-1].GetConfig()
	cache := &api.VerifiedSOStateCache{
		GenesisHash:              append([]byte(nil), h.genesisHash...),
		VerifiedConfigChainHash:  append([]byte(nil), newHash...),
		VerifiedConfigChainSeqno: entries[len(entries)-1].GetConfigSeqno(),
		KeyEpochs:                cloneSOKeyEpochs(resp.GetKeyEpochs()),
	}
	if latestConfig != nil {
		cache.CurrentConfig = latestConfig.CloneVT()
	}

	// Store epochs and update the last known config chain hash.
	h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		h.lastConfigChainHash = append([]byte(nil), newHash...)
		h.verifiedConfigChainSeqno = entries[len(entries)-1].GetConfigSeqno()
		h.keyEpochs = cloneSOKeyEpochs(resp.GetKeyEpochs())
		if latestConfig != nil {
			h.verifiedConfig = latestConfig.CloneVT()
		}
		broadcast()
	})
	if h.persistVerifiedStateCache != nil {
		if err := h.persistVerifiedStateCache(ctx, cache); err != nil {
			h.le.WithError(err).Warn("failed to write verified SO state cache")
		}
	}

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

	// If local peer is OWNER, check whether participants were removed by
	// comparing the previous and latest config entries. This is computed
	// locally rather than trusting the server-supplied changeType field.
	if localIsOwner && len(entries) >= 2 {
		prevParticipants := entries[len(entries)-2].GetConfig().GetParticipants()
		currParticipants := latestConfig.GetParticipants()
		if participantsRemoved(prevParticipants, currParticipants) {
			h.rotateKeyOnRevocation(ctx, currParticipants)
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
			prevIdx := int(entry.GetConfigSeqno()) - 1
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

// rotateKeyOnRevocation generates a new transform key and posts the epoch to the server.
// Note: old epoch grants remain on the server for historical decryption by remaining
// participants. This is by design -- forward secrecy means the revoked participant
// cannot decrypt NEW content, but historical content up to the rotation point remains
// accessible to anyone who had the old key. The server is trusted to serve epochs
// only to authorized participants (via rbac_role_bindings).
func (h *cloudSOHost) rotateKeyOnRevocation(ctx context.Context, participants []*sobject.SOParticipantConfig) {
	var currentEpoch uint64
	var currentSeqno uint64
	var currentCfg *sobject.SharedObjectConfig
	h.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		currentEpoch = sobject.CurrentEpochNumber(h.keyEpochs)
		if st := h.stateCtr.GetValue(); st != nil {
			if st.GetRoot() != nil {
				currentSeqno = st.GetRoot().GetInnerSeqno()
			}
			if st.GetConfig() != nil {
				currentCfg = st.GetConfig().CloneVT()
			}
		}
	})
	if currentCfg == nil {
		h.le.Warn("failed to rotate transform key: current config missing")
		return
	}

	transformConf, grants, epoch, err := sobject.RotateTransformKey(
		h.privKey,
		h.soID,
		participants,
		currentEpoch,
		currentSeqno,
	)
	if err != nil {
		h.le.WithError(err).Warn("failed to rotate transform key")
		return
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
			return
		}
		h.le.WithError(err).Warn("failed to build recovery envelopes for key rotation")
		return
	}

	// Post the new epoch to the server.
	if err := h.client.PostKeyEpoch(
		ctx,
		h.soID,
		epoch,
		recoveryEnvelopes,
	); err != nil {
		if ctx.Err() != nil {
			return
		}
		h.le.WithError(err).Warn("failed to post key epoch to server")
		return
	}

	// Update local state with the new grants as root_grants.
	h.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		h.keyEpochs = append(h.keyEpochs, epoch)
		st := h.stateCtr.GetValue()
		if st != nil {
			st = st.CloneVT()
			st.RootGrants = grants
			h.stateCtr.SetValue(st)
		}
		broadcast()
	})

	h.le.WithField("epoch", epoch.GetEpoch()).Info("key rotation complete after participant revocation")
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
			GenesisHash:              append([]byte(nil), h.genesisHash...),
			VerifiedConfigChainHash:  append([]byte(nil), h.lastConfigChainHash...),
			VerifiedConfigChainSeqno: h.verifiedConfigChainSeqno,
			KeyEpochs:                cloneSOKeyEpochs(h.keyEpochs),
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

// persistVerifiedStateCacheSnapshot writes the current trusted SO config cache.
func (h *cloudSOHost) persistVerifiedStateCacheSnapshot(ctx context.Context) {
	if h.persistVerifiedStateCache == nil {
		return
	}
	cache := h.buildVerifiedStateCache()
	if cache == nil {
		return
	}
	if err := h.persistVerifiedStateCache(ctx, cache); err != nil {
		h.le.WithError(err).Warn("failed to write verified SO state cache")
	}
}

// hydrateVerifiedStateCache loads persisted verified SO config state into memory.
func (h *cloudSOHost) hydrateVerifiedStateCache(cache *api.VerifiedSOStateCache) {
	if cache == nil {
		return
	}

	h.genesisHash = append([]byte(nil), cache.GetGenesisHash()...)
	h.lastConfigChainHash = append([]byte(nil), cache.GetVerifiedConfigChainHash()...)
	h.verifiedConfigChainSeqno = cache.GetVerifiedConfigChainSeqno()
	h.keyEpochs = cloneSOKeyEpochs(cache.GetKeyEpochs())
	if cache.GetCurrentConfig() != nil {
		h.verifiedConfig = cache.GetCurrentConfig().CloneVT()
	}
}
