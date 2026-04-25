package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

func (a *ProviderAccount) configureSessionClient(cli *SessionClient) *SessionClient {
	if cli == nil {
		return nil
	}
	cli.executeWriteTicketAudience = a.ExecuteWriteTicketAudience
	return cli
}

// getReadySessionClient returns a signing-capable session client for this
// account. If the cached session client is missing its private key, it falls
// back to any mounted unlocked session and repairs the cached client.
func (a *ProviderAccount) getReadySessionClient(ctx context.Context) (*SessionClient, crypto.PrivKey, peer.ID, error) {
	if cli := a.sessionClient; cli != nil && cli.priv != nil && cli.peerID != "" && a.sessionClientSessionID == "" {
		return a.configureSessionClient(cli), cli.priv, cli.peerID, nil
	}

	entries := a.sessions.GetKeysWithData()
	if cli, priv, pid, ok := a.getReadySessionClientForSession(ctx, entries, a.sessionClientSessionID); ok {
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
		if err != nil || sess == nil || sess.sessionPriv == nil {
			continue
		}

		cli := NewSessionClient(
			a.p.httpCli,
			a.p.endpoint,
			a.p.signingEnvPfx,
			sess.sessionPriv,
			sess.sessionPid.String(),
		)
		cli = a.configureSessionClient(cli)
		a.sessionClient = cli
		a.sessionClientSessionID = entry.Key
		return cli, sess.sessionPriv, sess.sessionPid, true
	}
	return nil, nil, "", false
}

func (a *ProviderAccount) maybeSetSessionClient(sessionID string, cli *SessionClient) {
	if cli == nil || sessionID == "" {
		return
	}
	cli = a.configureSessionClient(cli)
	if a.sessionClientSessionID != "" && a.sessionClientSessionID != sessionID {
		return
	}
	var rejoinState *selfRejoinSweepState
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.sessionClient = cli
		a.sessionClientSessionID = sessionID
		rejoinState = a.buildSelfRejoinSweepStateLocked()
		broadcast()
	})
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
