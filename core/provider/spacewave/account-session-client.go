package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// configureSessionClient connects cli to the account's recovery keypair
// cache.
func (a *ProviderAccount) configureSessionClient(cli *SessionClient) *SessionClient {
	if cli == nil {
		return nil
	}
	cli.recoveryKeypairs = &a.recoveryKeypairs
	return cli
}

func (a *ProviderAccount) currentSessionClient() *SessionClient {
	cli, _ := a.sessionClientSnapshot()
	return cli
}

func (a *ProviderAccount) sessionClientSnapshot() (*SessionClient, string) {
	var cli *SessionClient
	var sessionID string
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		cli = a.sessionClient
		sessionID = a.sessionClientSessionID
	})
	return cli, sessionID
}

// getReadySessionClient returns a signing-capable session client for this
// account. If the cached session client is missing its private key, it falls
// back to any mounted unlocked session and repairs the cached client.
func (a *ProviderAccount) getReadySessionClient(ctx context.Context) (*SessionClient, crypto.PrivKey, peer.ID, error) {
	cli, sessionID := a.sessionClientSnapshot()
	if cli != nil && cli.priv != nil && cli.peerID != "" && sessionID == "" {
		return cli, cli.priv, cli.peerID, nil
	}

	entries := a.sessions.GetKeysWithData()
	if cli, priv, pid, ok := a.getReadySessionClientForSession(ctx, entries, sessionID); ok {
		return cli, priv, pid, nil
	}
	for _, entry := range entries {
		cli, priv, pid, ok := a.getReadySessionClientForSession(ctx, entries, entry.Key)
		if ok {
			return cli, priv, pid, nil
		}
	}

	return nil, nil, "", errors.New("session private key not available")
}

func (a *ProviderAccount) getReadySessionClientForSession(
	ctx context.Context,
	entries []keyed.KeyWithData[string, *sessionTracker],
	sessionID string,
) (*SessionClient, crypto.PrivKey, peer.ID, bool) {
	if sessionID == "" {
		return nil, nil, "", false
	}

	for _, entry := range entries {
		if entry.Key != sessionID {
			continue
		}
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

		// Build the session's client and install it.
		cli := NewSessionClient(
			a.p.httpCli,
			a.p.endpoint,
			a.p.signingEnvPfx,
			priv,
			sess.sessionPid.String(),
		)
		cli = a.configureSessionClient(cli)
		a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			a.sessionClient = cli
			a.sessionClientSessionID = entry.Key
			broadcast()
		})
		return cli, priv, sess.sessionPid, true
	}
	return nil, nil, "", false
}

// maybeSetSessionClient installs cli as the account's session client for
// sessionID unless another session's client is already installed.
func (a *ProviderAccount) maybeSetSessionClient(sessionID string, cli *SessionClient) {
	// A client without a session cannot hold the slot.
	if cli == nil || sessionID == "" {
		return
	}

	// Install the client unless another session holds the slot.
	cli = a.configureSessionClient(cli)
	var rejoinState *selfRejoinSweepState
	var updated bool
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Leave a slot another session holds.
		if a.sessionClientSessionID != "" && a.sessionClientSessionID != sessionID {
			return
		}

		// Install the client and announce it.
		a.sessionClient = cli
		a.sessionClientSessionID = sessionID
		rejoinState = a.buildSelfRejoinSweepStateLocked()
		updated = true
		broadcast()
	})
	if !updated {
		return
	}

	// Refresh the state derived from the session client.
	a.setSelfRejoinSweepState(rejoinState)
	a.refreshSelfEnrollmentSummary(context.Background())
}

func (a *ProviderAccount) dropSessionClientForSession(sessionID string) {
	if sessionID == "" {
		return
	}
	var rejoinState *selfRejoinSweepState
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.sessionClientSessionID != sessionID {
			return
		}
		a.sessionClient = nil
		a.sessionClientSessionID = ""
		rejoinState = a.buildSelfRejoinSweepStateLocked()
		broadcast()
	})
	a.setSelfRejoinSweepState(rejoinState)
	a.refreshSelfEnrollmentSummary(context.Background())
}
