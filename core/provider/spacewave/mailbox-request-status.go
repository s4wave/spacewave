package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
)

// mailboxRequestKey identifies one cloud invite mailbox request.
type mailboxRequestKey struct {
	soID     string
	inviteID string
	peerID   string
}

// TrackMailboxRequest stores the current status for a cloud invite mailbox request.
func (a *ProviderAccount) TrackMailboxRequest(
	soID string,
	inviteID string,
	peerID string,
	status string,
) {
	if soID == "" || inviteID == "" || peerID == "" || status == "" {
		return
	}
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.state.mailboxRequestStatus == nil {
			a.state.mailboxRequestStatus = make(map[mailboxRequestKey]string)
		}
		key := mailboxRequestKey{
			soID:     soID,
			inviteID: inviteID,
			peerID:   peerID,
		}
		if a.state.mailboxRequestStatus[key] == status {
			return
		}
		a.state.mailboxRequestStatus[key] = status
		broadcast()
	})
}

// setMailboxRequestStatus stores a terminal mailbox status from a session event.
func (a *ProviderAccount) setMailboxRequestStatus(
	soID string,
	inviteID string,
	peerID string,
	status string,
) {
	a.TrackMailboxRequest(soID, inviteID, peerID, status)
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

	key := mailboxRequestKey{
		soID:     soID,
		inviteID: inviteID,
		peerID:   peerID,
	}
	for {
		var (
			ch     <-chan struct{}
			status string
		)
		a.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			if a.state.mailboxRequestStatus != nil {
				status = a.state.mailboxRequestStatus[key]
			}
		})
		if status != "" && status != "pending" {
			return status, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ch:
		}
	}
}
