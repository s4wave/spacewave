package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
)

// TrackMailboxRequest stores the current status for a cloud invite mailbox request.
func (a *ProviderAccount) TrackMailboxRequest(
	soID string,
	inviteID string,
	peerID string,
	status string,
) {
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if !a.state.mailboxRequests.Track(soID, inviteID, peerID, status) {
			return
		}
		broadcast()
	})
}

// WaitMailboxRequestDecision waits for a cloud invite mailbox request to leave
// the pending state.
func (a *ProviderAccount) WaitMailboxRequestDecision(
	ctx context.Context,
	soID string,
	inviteID string,
	peerID string,
) (string, error) {
	if soID == "" || inviteID == "" || peerID == "" {
		return "", errors.New("shared object id, invite id, and peer id are required")
	}
	for {
		var (
			ch     <-chan struct{}
			status string
			ready  bool
		)
		a.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			status, ready = a.state.mailboxRequests.Decision(soID, inviteID, peerID)
			if ready {
				return
			}
			ch = getWaitCh()
		})
		if ready {
			return status, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ch:
		}
	}
}
