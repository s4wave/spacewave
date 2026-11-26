package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// The account's session client slot holds the one live SessionSigner, inside
// the client that signs with it. Its invariants, which SessionSigner.tla
// checks:
//
//   - At most one signer the slot bound is live, and it is the slot's.
//   - A locked, logged out or unmounted Session has no live signer.
//   - Each bound signer's generation is greater than every earlier one.
//
// sessionClient, sessionClientSessionID and sessionSignerGen are guarded by
// accountBcast. Every write that changes the slot broadcasts, so a waiter on
// the account broadcast sees each install and drop.

// configureSessionClient connects cli to the account's recovery keypair
// cache.
func (a *ProviderAccount) configureSessionClient(cli *SessionClient) *SessionClient {
	if cli == nil {
		return nil
	}
	cli.recoveryKeypairs = &a.recoveryKeypairs
	return cli
}

// currentSessionClient returns the slot's client, which may be nil or unable
// to sign.
func (a *ProviderAccount) currentSessionClient() *SessionClient {
	cli, _ := a.sessionClientSnapshot()
	return cli
}

// sessionClientSnapshot returns the slot's client and the Session that owns it.
func (a *ProviderAccount) sessionClientSnapshot() (*SessionClient, string) {
	var cli *SessionClient
	var sessionID string
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		cli = a.sessionClient
		sessionID = a.sessionClientSessionID
	})
	return cli, sessionID
}

// GetSessionClient returns the slot's client while it holds a live signer. In
// the gap after a lock, logout or unmount and before the next install, it
// returns ErrSigningUnavailable. The client stops signing once its signer is
// revoked, so a caller resolves it again for each operation.
func (a *ProviderAccount) GetSessionClient() (*SessionClient, error) {
	cli := a.currentSessionClient()
	signer := cli.GetSessionSigner()
	if !signer.CanSign() || signer.GetPeerID() == "" {
		return nil, ErrSigningUnavailable
	}
	return cli, nil
}

// getReadySessionClient returns the slot's client with its private key and
// peer. When the slot holds no live signer, it installs one from the owning
// Session, or else from any mounted unlocked Session.
func (a *ProviderAccount) getReadySessionClient(ctx context.Context) (*SessionClient, crypto.PrivKey, peer.ID, error) {
	// Use the slot's live signer.
	cli, sessionID := a.sessionClientSnapshot()
	signer := cli.GetSessionSigner()
	if priv := signer.PrivKey(); priv != nil && signer.GetPeerID() != "" {
		return cli, priv, signer.GetPeerID(), nil
	}

	// Otherwise install from the owning Session, then from any mounted one.
	entries := a.sessions.GetKeysWithData()
	if cli, priv, pid, ok := a.repairSessionClientFromSession(ctx, entries, sessionID); ok {
		return cli, priv, pid, nil
	}
	for _, entry := range entries {
		if cli, priv, pid, ok := a.repairSessionClientFromSession(ctx, entries, entry.Key); ok {
			return cli, priv, pid, nil
		}
	}
	return nil, nil, "", errors.Wrap(ErrSigningUnavailable, "session private key not available")
}

// repairSessionClientFromSession installs the signer of mounted Session
// sessionID when its key is unlocked, and returns the slot's live client.
func (a *ProviderAccount) repairSessionClientFromSession(
	ctx context.Context,
	entries []keyed.KeyWithData[string, *sessionTracker],
	sessionID string,
) (*SessionClient, crypto.PrivKey, peer.ID, bool) {
	// Find the mounted Session.
	if sessionID == "" {
		return nil, nil, "", false
	}
	for _, entry := range entries {
		if entry.Key != sessionID {
			continue
		}

		// Wait for the Session and take its unlocked key.
		prom, _ := entry.Data.sessionProm.GetPromise()
		if prom == nil {
			continue
		}
		sess, err := prom.Await(ctx)
		if err != nil || sess == nil {
			continue
		}
		priv := sess.GetPrivKey()
		if priv == nil {
			continue
		}

		// Install its signer and return whichever signer the slot then holds.
		cli := a.installSessionClient(sessionID, priv, sess.sessionPid)
		signer := cli.GetSessionSigner()
		if p := signer.PrivKey(); p != nil {
			return cli, p, signer.GetPeerID(), true
		}
	}
	return nil, nil, "", false
}

// maybeSetSessionClient installs the signer for sessionID's key unless another
// Session holds the slot.
func (a *ProviderAccount) maybeSetSessionClient(sessionID string, priv crypto.PrivKey, peerID peer.ID) {
	if sessionID == "" || priv == nil {
		return
	}
	a.installSessionClient(sessionID, priv, peerID)
}

// ReplaceSessionClient installs a signer for sessionID's freshly generated key,
// revoking the signer the slot held. The slot binds it under sessionID, so the
// Session's lock, logout and unmount revoke it. Used during reauthentication.
func (a *ProviderAccount) ReplaceSessionClient(sessionID string, priv crypto.PrivKey, peerID peer.ID) {
	// Bind the new signer.
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.bindSessionClientLocked(sessionID, priv, peerID)
		broadcast()
	})

	// Refresh the state derived from the session client.
	a.selfEnrollmentRefresh.Trigger()
}

// installSessionClient binds a signer for sessionID's key in the slot and
// returns the slot's client. A Session that already holds a live signer for
// the same peer keeps it, without a broadcast. A slot that another Session
// holds is left alone, since a slot with an owner always holds a live signer.
func (a *ProviderAccount) installSessionClient(sessionID string, priv crypto.PrivKey, peerID peer.ID) *SessionClient {
	// Bind under the slot's lock, keeping a live signer that already matches.
	var cli *SessionClient
	var rejoinState *selfRejoinSweepState
	var updated bool
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Keep, leave or replace the slot's signer by its owner and liveness.
		cur := a.sessionClient.GetSessionSigner()
		owner := a.sessionClientSessionID
		switch {
		case owner == sessionID && cur.CanSign() && cur.GetPeerID() == peerID:
			// Keep the Session's live signer.
		case owner != "" && owner != sessionID:
			// Leave the slot another Session holds.
		default:
			a.bindSessionClientLocked(sessionID, priv, peerID)
			broadcast()
		}
		cli = a.sessionClient
		if a.sessionClientSessionID == sessionID {
			rejoinState = a.buildSelfRejoinSweepStateLocked()
			updated = true
		}
	})
	if !updated {
		return cli
	}

	// Refresh the state derived from the session client.
	a.setSelfRejoinSweepState(rejoinState)
	a.selfEnrollmentRefresh.Trigger()
	return cli
}

// bindSessionClientLocked revokes the slot's signer and installs a client with
// a new signer for priv at the next generation. The caller holds the
// accountBcast lock and broadcasts.
func (a *ProviderAccount) bindSessionClientLocked(sessionID string, priv crypto.PrivKey, peerID peer.ID) {
	// Revoke the old signer and bind a new one at the next generation.
	a.sessionClient.GetSessionSigner().revoke()
	a.sessionSignerGen++
	signer := newSessionSigner(sessionID, peerID, a.sessionSignerGen, priv, nil)
	a.sessionClient = a.configureSessionClient(newSessionClientFromSigner(
		a.p.httpCli,
		a.p.endpoint,
		a.p.signingEnvPfx,
		signer,
	))
	a.sessionClientSessionID = sessionID
}

// dropSessionClientForSession revokes and clears the slot's signer when it
// belongs to sessionID, and refreshes the state derived from it. Lock, logout
// and the Session tracker's exit call it.
func (a *ProviderAccount) dropSessionClientForSession(sessionID string) {
	// Revoke and clear the signer only when sessionID owns it.
	if sessionID == "" {
		return
	}
	var rejoinState *selfRejoinSweepState
	var dropped bool
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Clear the slot only while sessionID still owns it.
		if a.sessionClientSessionID != sessionID {
			return
		}
		a.sessionClient.GetSessionSigner().revoke()
		a.sessionClient = nil
		a.sessionClientSessionID = ""
		rejoinState = a.buildSelfRejoinSweepStateLocked()
		dropped = true
		broadcast()
	})
	if !dropped {
		return
	}

	// Refresh the state derived from the session client.
	a.setSelfRejoinSweepState(rejoinState)
	a.selfEnrollmentRefresh.Trigger()
}
