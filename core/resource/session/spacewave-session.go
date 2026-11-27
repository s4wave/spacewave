package resource_session

import (
	"github.com/aperturerobotics/controllerbus/bus"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/core/session"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
)

// SpacewaveSessionResource implements SpacewaveSessionResourceService.
// It wraps a session-scoped spacewave ProviderAccount, resolving it directly
// from the session without scanning.
type SpacewaveSessionResource struct {
	// parent owns the shared Session RPC operations.
	parent *SessionResource
	// le records session-scoped cloud operations.
	le *logrus.Entry
	// b resolves provider and Session controllers.
	b bus.Bus
	// session identifies the caller and its mounted Session.
	session session.Session
	// swAcc owns the account state and cloud operations.
	swAcc *provider_spacewave.ProviderAccount
}

// NewSpacewaveSessionResource creates a new SpacewaveSessionResource.
func NewSpacewaveSessionResource(
	sr *SessionResource,
	le *logrus.Entry,
	b bus.Bus,
	sess session.Session,
	swAcc *provider_spacewave.ProviderAccount,
) *SpacewaveSessionResource {
	return &SpacewaveSessionResource{
		parent:  sr,
		le:      le,
		b:       b,
		session: sess,
		swAcc:   swAcc,
	}
}

// getSessionRef returns the session reference.
func (r *SpacewaveSessionResource) getSessionRef() *session.SessionRef {
	return r.session.GetSessionRef()
}

// getSessionID returns the session ID from the provider resource ref.
func (r *SpacewaveSessionResource) getSessionID() string {
	return r.getSessionRef().GetProviderResourceRef().GetId()
}

// _ verifies the Spacewave session resource RPC contract.
var _ s4wave_session.SRPCSpacewaveSessionResourceServiceServer = (*SpacewaveSessionResource)(nil)
