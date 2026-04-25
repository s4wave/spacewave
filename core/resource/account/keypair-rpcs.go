package resource_account

import (
	"context"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
)

// WatchEntityKeypairs streams entity keypairs with their lock state.
//
// Account state and tracker unlock state are folded into one local broadcast
// so the watch loop reads both inputs under the same HoldLock that obtains
// the wait channel. This eliminates the missed-wakeup race that the previous
// dual-channel select had to defend against and coalesces near-simultaneous
// changes (e.g. an account-state update landing alongside a tracker unlock)
// into a single emission instead of one per source.
func (r *AccountResource) WatchEntityKeypairs(
	req *s4wave_account.WatchEntityKeypairsRequest,
	strm s4wave_account.SRPCAccountResourceService_WatchEntityKeypairsStream,
) error {
	ctx := strm.Context()
	tracker := r.account.GetEntityKeypairTracker()
	state := &entityKeypairsWatchState{
		keypairs:      r.account.KeypairsSnapshot(),
		valid:         r.account.AccountStateSnapshot() != nil,
		unlockedPeers: tracker.GetUnlockedPeerIDs(),
	}

	bridgeCtx, cancelBridges := context.WithCancel(ctx)
	defer cancelBridges()
	go state.bridgeAccount(bridgeCtx, r.account)
	go state.bridgeTracker(bridgeCtx, tracker)

	return state.runWatchLoop(ctx, strm.Send)
}

// entityKeypairsWatchState carries every input snapshot the keypairs watch
// reads per emission, guarded by a single broadcast so the watch loop reads
// all of them under the same HoldLock that obtains the wait channel. Both
// bridge goroutines update fields under HoldLock and broadcast on change;
// the watch never observes a stale wait channel paired with a fresh source
// update.
type entityKeypairsWatchState struct {
	keypairs      []*session.EntityKeypair
	valid         bool
	unlockedPeers map[peer.ID]bool
	bcast         broadcast.Broadcast
}

// bridgeAccount forwards account broadcast wakeups into the local broadcast,
// re-snapshotting the keypair list and account validity on each update.
func (s *entityKeypairsWatchState) bridgeAccount(
	ctx context.Context,
	acc *provider_spacewave.ProviderAccount,
) {
	accountBcast := acc.GetAccountBroadcast()
	for {
		var waitCh <-chan struct{}
		accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
		})
		select {
		case <-ctx.Done():
			return
		case <-waitCh:
		}
		keypairs := acc.KeypairsSnapshot()
		valid := acc.AccountStateSnapshot() != nil
		s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			s.keypairs = keypairs
			s.valid = valid
			broadcast()
		})
	}
}

// bridgeTracker forwards tracker broadcast wakeups into the local broadcast,
// re-snapshotting the unlocked peer set on each update.
func (s *entityKeypairsWatchState) bridgeTracker(
	ctx context.Context,
	tracker *provider_spacewave.EntityKeyStore,
) {
	trackerBcast := tracker.GetBroadcast()
	for {
		var waitCh <-chan struct{}
		trackerBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
		})
		select {
		case <-ctx.Done():
			return
		case <-waitCh:
		}
		unlockedPeers := tracker.GetUnlockedPeerIDs()
		s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			s.unlockedPeers = unlockedPeers
			broadcast()
		})
	}
}

// runWatchLoop emits a fresh WatchEntityKeypairsResponse whenever any folded
// source changes. The state and wait channel are read in the same HoldLock
// so a source update that lands between reading state and selecting on the
// wait channel cannot be missed: the broadcast in the source bridge replaces
// the wait channel before the watch loop blocks on it.
func (s *entityKeypairsWatchState) runWatchLoop(
	ctx context.Context,
	send func(*s4wave_account.WatchEntityKeypairsResponse) error,
) error {
	var prev *s4wave_account.WatchEntityKeypairsResponse
	for {
		var (
			keypairs      []*session.EntityKeypair
			valid         bool
			unlockedPeers map[peer.ID]bool
			waitCh        <-chan struct{}
		)
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			keypairs = s.keypairs
			valid = s.valid
			unlockedPeers = s.unlockedPeers
			waitCh = getWaitCh()
		})

		if valid {
			states := make([]*s4wave_account.EntityKeypairState, 0, len(keypairs))
			for _, kp := range keypairs {
				pid, pidErr := peer.IDB58Decode(kp.GetPeerId())
				unlocked := pidErr == nil && unlockedPeers[pid]
				states = append(states, &s4wave_account.EntityKeypairState{
					Keypair:  kp,
					Unlocked: unlocked,
				})
			}
			resp := &s4wave_account.WatchEntityKeypairsResponse{
				Keypairs:      states,
				UnlockedCount: uint32(len(unlockedPeers)),
			}
			if prev == nil || !resp.EqualVT(prev) {
				if err := send(resp); err != nil {
					return err
				}
				prev = resp
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

// UnlockEntityKeypair derives the entity private key from a credential
// and holds it in memory for signing operations.
func (r *AccountResource) UnlockEntityKeypair(
	ctx context.Context,
	req *s4wave_account.UnlockEntityKeypairRequest,
) (*s4wave_account.UnlockEntityKeypairResponse, error) {
	privKey, resolvedPeerID, err := r.ResolveEntityKey(ctx, req.GetCredential())
	if err != nil {
		return nil, err
	}
	requestedPeerID := req.GetPeerId()
	if requestedPeerID != "" && resolvedPeerID.String() != requestedPeerID {
		return nil, errors.Errorf("resolved peer ID %s does not match requested %s", resolvedPeerID.String(), requestedPeerID)
	}
	r.account.GetEntityKeypairTracker().Unlock(resolvedPeerID, privKey)
	return &s4wave_account.UnlockEntityKeypairResponse{}, nil
}

// LockEntityKeypair drops a previously unlocked entity private key.
func (r *AccountResource) LockEntityKeypair(
	ctx context.Context,
	req *s4wave_account.LockEntityKeypairRequest,
) (*s4wave_account.LockEntityKeypairResponse, error) {
	pidStr := req.GetPeerId()
	if pidStr == "" {
		return nil, errors.New("peer_id is required")
	}
	pid, err := peer.IDB58Decode(pidStr)
	if err != nil {
		return nil, errors.Wrap(err, "decode peer ID")
	}
	r.account.GetEntityKeypairTracker().Lock(pid)
	return &s4wave_account.LockEntityKeypairResponse{}, nil
}

// LockAllEntityKeypairs drops all unlocked entity private keys.
func (r *AccountResource) LockAllEntityKeypairs(
	ctx context.Context,
	_ *s4wave_account.LockAllEntityKeypairsRequest,
) (*s4wave_account.LockAllEntityKeypairsResponse, error) {
	r.account.GetEntityKeypairTracker().LockAll()
	return &s4wave_account.LockAllEntityKeypairsResponse{}, nil
}

// resolveOrSignWithTracker builds the MultiSigActionEnvelope and produces
// signatures over the envelope bytes using either a caller-supplied credential
// or any keys currently unlocked in the keypair tracker. Returns the envelope
// bytes plus the resolved signatures.
//
// The precedence is:
//  1. credential != nil: resolve entity key, sign envelope
//  2. tracker has unlocked keys: sign envelope with all unlocked keys
//  3. error: no credentials and no unlocked keys
func (r *AccountResource) resolveOrSignWithTracker(
	ctx context.Context,
	cred *session.EntityCredential,
	kind api.MultiSigActionKind,
	method, reqPath string,
	actionBody []byte,
) ([]byte, []*api.EntitySignature, error) {
	envelope, err := r.buildMultiSigEnvelope(kind, method, reqPath, actionBody)
	if err != nil {
		return nil, nil, err
	}
	if cred != nil {
		entityPriv, entityPeerID, err := r.ResolveEntityKey(ctx, cred)
		if err != nil {
			return nil, nil, err
		}
		now := timestamppb.New(time.Now().Truncate(time.Millisecond))
		payload := provider_spacewave.BuildMultiSigPayload(now, envelope)
		sig, err := entityPriv.Sign(payload)
		if err != nil {
			return nil, nil, errors.Wrap(err, "sign envelope")
		}
		return envelope, []*api.EntitySignature{{
			PeerId:    entityPeerID.String(),
			Signature: sig,
			SignedAt:  now,
		}}, nil
	}
	trackerSigs, err := r.account.GetEntityKeypairTracker().SignAll(envelope)
	if err != nil {
		return nil, nil, errors.Wrap(err, "tracker sign")
	}
	if len(trackerSigs) == 0 {
		return nil, nil, errors.New("no credentials provided and no keypairs unlocked")
	}
	return envelope, trackerSigs, nil
}
