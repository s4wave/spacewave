package auth_challenge_server

import (
	"context"
	"time"

	auth_challenge "github.com/aperturerobotics/auth/challenge"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/identity"
	"github.com/pkg/errors"
)

// remoteEntityLookupRequest contains info about a remote request.
type remoteEntityLookupRequest struct {
	// ctx is nil before the goroutine is started.
	// started by the execute loop in the controller.
	ctx       context.Context
	ctxCancel context.CancelFunc

	earliest time.Time
	peer     *authClientPeer
	rid      auth_challenge.RequestID
}

// newRemoteEntityLookupRequest constructs a new entity request.
// expects mtx to be locked on controller
func newRemoteEntityLookupRequest(peer *authClientPeer, rid auth_challenge.RequestID) *remoteEntityLookupRequest {
	return &remoteEntityLookupRequest{
		earliest: peer.backOffUntil,
		peer:     peer,
		rid:      rid,
	}
}

// executeEntityLookupRequest executes the request in a goroutine.
func (r *remoteEntityLookupRequest) executeEntityLookupRequest() {
	le := r.peer.c.le
	ctx := r.ctx
	defer r.ctxCancel()

	le.Info("execute entity lookup request starting")
	earliest := r.earliest
	now := time.Now()
	if now.Before(earliest) {
		waitDur := earliest.Sub(now)
		le.Infof("waiting %s until backoff finishes", waitDur)
		tmr := time.NewTimer(waitDur)
		select {
		case <-ctx.Done():
			tmr.Stop()
			return
		case <-tmr.C:
		}
	}

	// Issue the request on behalf of the remote peer.
	entity, err := r.executeEntityLookupRequestOnce(ctx)
	if err == context.Canceled {
		return
	}
	r.peer.c.mtx.Lock()
	resultErr := err
	if err != nil {
		if err != context.Canceled {
			nextBo := r.peer.bo.NextBackOff()
			le.
				WithError(err).
				WithField("backoff-time", nextBo.String()).
				Warn("error executing request, issuing backoff")
			now := time.Now()
			earliest = now.Add(nextBo)
			r.peer.backOffUntil = earliest
		}
	} else {
		le.WithField("domain-id", r.rid.GetDomainID()).
			WithField("entity-id", r.rid.GetEntityID()).
			Info("successfully returned entity lookup")
		// r.peer.bo.Reset()
	}
	if r.peer.requests[r.rid] == r {
		delete(r.peer.requests, r.rid)
	}
	// find write-back session
	for sess := range r.peer.sessions {
		if sess != nil {
			outErr := sess.sess.SendMsg(&auth_challenge.Msg{
				MsgType: auth_challenge.MsgType_MsgType_ENTITY_LOOKUP_FINISH,
				EntityLookupFinish: auth_challenge.NewEntityLookupFinish(
					r.rid.GetDomainID(),
					r.rid.GetEntityID(),
					resultErr,
					resultErr == nil && entity == nil,
					entity,
				),
			})
			if outErr == nil {
				break
			}
			le.WithError(outErr).Warn("unable to send response on session")
		}
	}
	r.peer.c.mtx.Unlock()
}

// executeEntityLookupRequestOnce attempts to execute the request
func (r *remoteEntityLookupRequest) executeEntityLookupRequestOnce(ctx context.Context) (*identity.Entity, error) {
	// check domain ID
	entityID := r.rid.GetEntityID()
	domainID := r.rid.GetDomainID()
	domainRestrict := r.peer.c.conf.GetDomains()
	if len(domainRestrict) != 0 {
		var found bool
		for _, d := range domainRestrict {
			if d == domainID {
				found = true
				break
			}
		}
		if !found {
			return nil, errors.Errorf("domain %s not in authorized set", domainID)
		}
	}

	// check entity
	resVal, resRef, err := bus.ExecOneOff(
		ctx,
		r.peer.c.bus,
		identity.NewIdentityLookupEntity(entityID, domainID),
		nil,
	)
	if err != nil {
		r.peer.c.le.
			WithError(err).
			WithField("peer-id", r.peer.id.Pretty()).
			WithField("entity-id", entityID).
			WithField("domain-id", domainID).
			Warn("error looking up entity")
		return nil, err
	}
	defer resRef.Release()

	val, valOk := resVal.GetValue().(identity.IdentityLookupEntityValue)
	if !valOk {
		return nil, errors.New("invalid value returned from directive")
	}
	if val.IsNotFound() {
		return nil, auth_challenge.ErrNotFound
	}
	if err := val.GetError(); err != nil {
		return nil, err
	}
	return val.GetEntity(), nil
}
