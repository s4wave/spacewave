package dexaccess

import (
	"context"
	"encoding/hex"
	"slices"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// NewSharedObjectRequestAccess retains the SharedObject authority watch for
// one authenticated DEX requester. sourceCtx ends when that authority source is
// released; nil leaves the retained watch as the source. releaseObject releases
// the caller's SharedObject mount. le receives denial context only in the
// source process; nil disables that diagnostic.
func NewSharedObjectRequestAccess(
	ctx context.Context,
	le *logrus.Entry,
	sourceCtx context.Context,
	host *sobject.SOHost,
	requester peer.ID,
	releaseObject func(),
) (dex_solicit.RequestAccess, error) {
	// Keep the existing SOHost authority watch until this request finishes.
	watchCtx, cancel := context.WithCancel(ctx)
	states, releaseStates, err := host.GetSOStateCtr(watchCtx, cancel)
	if err != nil {
		cancel()
		releaseObject()
		return nil, err
	}
	return &sharedObjectDEXRequestAccess{
		le: le, objectID: host.GetSharedObjectID(),
		sourceCtx: sourceCtx, watchCtx: watchCtx, cancel: cancel,
		states: states, requester: requester.String(),
		releaseStates: releaseStates, releaseObject: releaseObject,
	}, nil
}

// sharedObjectDEXRequestAccess reads participant roles from the retained SOHost watch.
type sharedObjectDEXRequestAccess struct {
	// le receives server-local denial context, never the wire response.
	le *logrus.Entry
	// objectID identifies the retained source of authority.
	objectID string
	// sourceCtx ends when the SharedObject's authority source is released.
	sourceCtx context.Context
	// watchCtx ends when the request releases its retained authority watch.
	watchCtx context.Context
	// cancel releases the watch context.
	cancel context.CancelFunc
	// states is the SharedObject's current accepted state.
	states ccontainer.Watchable[*sobject.SOState]
	// requester is the authenticated peer requesting the block.
	requester string
	// releaseStates releases the SOHost authority watch.
	releaseStates func()
	// releaseObject releases the mounted SharedObject.
	releaseObject func()
}

// Check rejects requesters absent from the current readable participant roles.
func (a *sharedObjectDEXRequestAccess) Check() error {
	// A released source cannot authorize a request from its cached value.
	if a.sourceCtx != nil {
		if err := a.sourceCtx.Err(); err != nil {
			return err
		}
	}
	if err := a.watchCtx.Err(); err != nil {
		return err
	}

	// Read the participant role from the retained authority source.
	return a.checkState(a.states.GetValue())
}

// WaitRevoked waits on the same authority watch Check reads.
func (a *sharedObjectDEXRequestAccess) WaitRevoked(ctx context.Context) error {
	// End the wait when the request, watch, or source ends.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopWatch := context.AfterFunc(a.watchCtx, cancel)
	defer stopWatch()
	if a.sourceCtx != nil {
		stopSource := context.AfterFunc(a.sourceCtx, cancel)
		defer stopSource()
	}

	// Wait on the same state Check reads without mirroring membership.
	_, err := a.states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		return false, a.checkState(state)
	}, nil)
	return err
}

// checkState rejects absent participants and roles without read permission.
func (a *sharedObjectDEXRequestAccess) checkState(state *sobject.SOState) error {
	// Authorize only the requester's current readable participant record.
	if slices.ContainsFunc(state.GetConfig().GetParticipants(), func(participant *sobject.SOParticipantConfig) bool {
		return participant.GetPeerId() == a.requester && sobject.CanReadState(participant.GetRole())
	}) {
		return nil
	}

	// Record only public identities from the same snapshot that denied access.
	// Keep this server-local: the unauthorized peer receives no participant list.
	err := errors.New("shared object block access denied")
	if a.le != nil {
		// Summarize public roles without logging grants or private keys.
		participants := make([]string, 0, len(state.GetConfig().GetParticipants()))
		for _, participant := range state.GetConfig().GetParticipants() {
			participants = append(participants, participant.GetPeerId()+"="+participant.GetRole().String())
		}

		// Keep the diagnostic in the caller's server log.
		a.le.WithError(err).WithFields(logrus.Fields{
			"requester-peer":    a.requester,
			"shared-object-id":  a.objectID,
			"config-chain-hash": hex.EncodeToString(state.GetConfig().GetConfigChainHash()),
			"participants":      participants,
		}).Warn("DEX request denied by current SharedObject authority")
	}
	return err
}

// Release releases the state watch before the mounted SharedObject.
func (a *sharedObjectDEXRequestAccess) Release() {
	a.cancel()
	a.releaseStates()
	a.releaseObject()
}

// _ is a type assertion.
var _ dex_solicit.RequestAccess = (*sharedObjectDEXRequestAccess)(nil)
