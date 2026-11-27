package provider_spacewave

import (
	"context"
	"time"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
	"github.com/s4wave/spacewave/net/peer"
)

// registerSession registers the Session identity with the cloud and records
// the accepted peer ID in objStore so later mounts skip registration.
func (t *sessionTracker) registerSession(
	ctx context.Context,
	objStore object.ObjectStore,
	sessionPeerID peer.ID,
) (*api.ObservedSessionMetadata, error) {
	// Register the session's peer with the cloud.
	resp, err := t.a.entityCli.RegisterSessionDirectWithResponse(ctx, sessionPeerID.String(), buildSessionDeviceInfo(), "", "")
	if err != nil {
		return nil, errors.Wrap(err, "register session with cloud")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	t.a.le.WithField("session-id", t.id).Debug("registered session with cloud")

	// Persist the registered session peer for future mounts.
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
func (s *Session) watchTransportAuthorization(ctx context.Context, credential *SessionCredential) error {
	// Follow transport authorization under this credential generation.
	a := s.tkr.a
	bo := providerBackoff.Construct()
	for {
		// Wait for the composition to report a rejected credential.
		snapshot, changed := a.GetTransportCompositionSnapshotWithWait(s.tkr.id)
		if !snapshot.Unauthorized {
			select {
			case <-ctx.Done():
				return context.Canceled
			case <-changed:
			}
			continue
		}

		// Bound registration attempts with the existing provider backoff.
		timer := time.NewTimer(bo.NextBackOff())
		select {
		case <-ctx.Done():
			timer.Stop()
			return context.Canceled
		case <-timer.C:
		}

		// Repair registration without installing another signer after the response.
		a.le.WithField("session-id", s.tkr.id).WithField("error", snapshot.LastError).Warn("cloud rejected session transport; re-registering")
		observed, err := s.tkr.registerSession(ctx, s.objStore, s.sessionPid)
		if err != nil {
			return err
		}

		// Keep repair and its replacement transport inside the same credential generation.
		err = a.ConfigureSessionTransport(ctx, s.tkr.id, credential, a.p.endpoint, s.GetDirectP2PEnabled())
		if err != nil && !sessionTransportUnauthorized(err) {
			return errors.Wrap(err, "restart session transport after re-registration")
		}

		// Mirror presentation only while the repaired generation remains active.
		if err := a.UpsertSessionPresentation(ctx, s.sessionPid.String(), observed); err != nil {
			a.le.WithError(err).Warn("failed to mirror session presentation metadata")
		}
	}
}

// watchSessionTransport serializes composition lifetimes while lock callers
// install and revoke credential generations independently of network readiness.
func (s *Session) watchSessionTransport(ctx context.Context) error {
	for {
		// Observe the current generation and its next change under one state lock.
		var credential *SessionCredential
		var changed <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			credential = s.sessionPriv
			changed = getWait()
		})

		// Wait for unlock when the Session holds no live generation.
		if credential == nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-changed:
				continue
			}
		}

		// Join this generation's mechanics before observing its successor.
		err := s.runSessionTransport(credential)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !credential.Revoked() {
			return err
		}
	}
}

// runSessionTransport owns setup, authorization repair and cleanup for one key.
func (s *Session) runSessionTransport(credential *SessionCredential) error {
	// Bind setup and every repair to revocation of this credential generation.
	a := s.tkr.a
	ctx := credential.Context()
	defer func() {
		if credential.Revoked() {
			a.StopSessionTransportComposition(s.tkr.id)
		}
	}()
	if credential.Revoked() {
		return context.Canceled
	}
	err := a.ConfigureSessionTransport(ctx, s.tkr.id, credential, a.p.endpoint, s.GetDirectP2PEnabled())
	if err != nil && !sessionTransportUnauthorized(err) {
		return err
	}

	// Keep authorization recovery in the composition's generation lifetime.
	return s.watchTransportAuthorization(ctx, credential)
}
