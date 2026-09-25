package provider_spacewave

import (
	"context"
	"time"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// registerSession registers the Session identity with the cloud, installs its
// signer for account requests, and records the accepted peer ID in objStore so
// later mounts skip registration.
func (t *sessionTracker) registerSession(
	ctx context.Context,
	objStore object.ObjectStore,
	sessionPriv crypto.PrivKey,
	sessionPeerID peer.ID,
) (*api.ObservedSessionMetadata, error) {
	resp, err := t.a.entityCli.RegisterSessionDirectWithResponse(ctx, sessionPeerID.String(), buildSessionDeviceInfo(), "", "")
	if err != nil {
		return nil, errors.Wrap(err, "register session with cloud")
	}
	t.a.le.WithField("session-id", t.id).Debug("registered session with cloud")
	t.a.maybeSetSessionClient(t.id, NewSessionClient(
		t.a.p.httpCli,
		t.a.p.endpoint,
		t.a.p.signingEnvPfx,
		sessionPriv,
		sessionPeerID.String(),
	))

	regKey := []byte(t.id + "/registered")
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, regKey, []byte(sessionPeerID.String()))
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "record session registration")
	}
	return resp.GetObservedMetadata(), nil
}

// watchTransportAuthorization registers the Session again whenever the cloud
// rejects its transport credential, then restarts the transport. Each repair
// first waits out providerBackoff so a persistent rejection cannot spin.
func (s *Session) watchTransportAuthorization(ctx context.Context) error {
	a := s.tkr.a
	bo := providerBackoff.Construct()
	for {
		snapshot, changed := a.GetTransportCompositionSnapshotWithWait(s.tkr.id)
		if !snapshot.Unauthorized {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-changed:
			}
			continue
		}

		timer := time.NewTimer(bo.NextBackOff())
		select {
		case <-ctx.Done():
			timer.Stop()
			return context.Canceled
		case <-timer.C:
		}

		a.le.WithField("session-id", s.tkr.id).WithField("error", snapshot.LastError).Warn("cloud rejected session transport; re-registering")
		priv := s.GetPrivKey()
		observed, err := s.tkr.registerSession(ctx, s.objStore, priv, s.sessionPid)
		if err != nil {
			return err
		}
		// The transport outlives this routine, so it binds to the Session lifetime.
		err = a.ConfigureSessionTransport(s.lifecycleCtx, s.tkr.id, priv, a.p.endpoint, s.GetDirectP2PEnabled())
		if err != nil && !sessionTransportUnauthorized(err) {
			return errors.Wrap(err, "restart session transport after re-registration")
		}
		if err := a.UpsertSessionPresentation(ctx, s.sessionPid.String(), observed); err != nil {
			a.le.WithError(err).Warn("failed to mirror session presentation metadata")
		}
	}
}
