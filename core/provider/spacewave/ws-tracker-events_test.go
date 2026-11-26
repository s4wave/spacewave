package provider_spacewave

import (
	"github.com/aperturerobotics/util/broadcast"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/sirupsen/logrus"
)

// nopWSEvents ignores every Session websocket event.
type nopWSEvents struct{}

// connected ignores the event.
func (nopWSEvents) connected() {}

// dormantChanged ignores the event.
func (nopWSEvents) dormantChanged(bool) {}

// sessionUnauthenticated ignores the event.
func (nopWSEvents) sessionUnauthenticated() {}

// accountWasDeleted ignores the event.
func (nopWSEvents) accountWasDeleted() {}

// accountChanged ignores the event.
func (nopWSEvents) accountChanged(uint64) {}

// orgChanged ignores the event.
func (nopWSEvents) orgChanged(string) {}

// pendingParticipant ignores the event.
func (nopWSEvents) pendingParticipant(string, string) {}

// memberSessionChanged ignores the event.
func (nopWSEvents) memberSessionChanged(string, string, string, bool) {}

// soNotify ignores the event.
func (nopWSEvents) soNotify(string, *api.SONotifyEventPayload) {}

// soListUpdate ignores the event.
func (nopWSEvents) soListUpdate(*sobject.SharedObjectList) {}

// inviteMailbox ignores the event.
func (nopWSEvents) inviteMailbox(string, *api.MailboxEntry, int64) {}

// inviteMailboxUpdate ignores the event.
func (nopWSEvents) inviteMailboxUpdate(string, *api.MailboxEntry, int64) {}

// targetedInvitationsChanged ignores the event.
func (nopWSEvents) targetedInvitationsChanged() {}

// updateAvailable ignores the event.
func (nopWSEvents) updateAvailable() {}

// cdnRootChanged ignores the event.
func (nopWSEvents) cdnRootChanged(string) {}

// newTestWSTracker builds a tracker that ignores account-level events and
// idles on its own broadcast.
func newTestWSTracker(le *logrus.Entry, getClient func() *SessionClient) *wsTracker {
	return newWSTracker(le, getClient, &broadcast.Broadcast{}, nopWSEvents{})
}
