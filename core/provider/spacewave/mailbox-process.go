package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_invite "github.com/s4wave/spacewave/core/sobject/invite"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

type mailboxAutoProcessKey struct {
	soID    string
	entryID int64
}

type invalidMailboxEntryError struct {
	err error
}

func (e *invalidMailboxEntryError) Error() string {
	return e.err.Error()
}

func (e *invalidMailboxEntryError) Unwrap() error {
	return e.err
}

// ProcessMailboxEntry accepts or rejects a mailbox entry for a cloud space.
func (a *ProviderAccount) ProcessMailboxEntry(
	ctx context.Context,
	soID string,
	entryID int64,
	accept bool,
) error {
	if soID == "" {
		return errors.New("shared object id is required")
	}
	if entryID == 0 {
		return errors.New("entry id is required")
	}

	cli := a.GetSessionClient()
	if cli == nil {
		return errors.New("session client not available")
	}

	resp, err := a.getPendingMailboxResponseCached(ctx, soID)
	if err != nil {
		return err
	}

	var entry *api.MailboxEntry
	for _, candidate := range resp.GetEntries() {
		if candidate.GetId() == entryID {
			entry = candidate
			break
		}
	}
	if entry == nil {
		return errors.New("mailbox entry not found")
	}

	return a.processMailboxEntry(ctx, cli, soID, entry, accept)
}

// processPendingMailboxEntries processes the current pending mailbox queue for
// an owned cloud space.
func (a *ProviderAccount) processPendingMailboxEntries(
	ctx context.Context,
	soID string,
) error {
	if soID == "" {
		return errors.New("shared object id is required")
	}
	if !a.canAccessOwnerMailbox() {
		return nil
	}

	cli := a.GetSessionClient()
	if cli == nil {
		return errors.New("session client not available")
	}

	resp, err := a.getPendingMailboxResponseCached(ctx, soID)
	if err != nil {
		return err
	}

	for _, entry := range resp.GetEntries() {
		if err := a.processMailboxEntryWithReject(ctx, cli, soID, entry); err != nil {
			return err
		}
	}

	return nil
}

func (a *ProviderAccount) setMailboxAutoProcessEntry(key mailboxAutoProcessKey, entry *api.MailboxEntry) {
	if entry == nil {
		return
	}
	a.mailboxAutoEntriesMtx.Lock()
	if a.mailboxAutoEntries == nil {
		a.mailboxAutoEntries = make(map[mailboxAutoProcessKey]*api.MailboxEntry)
	}
	a.mailboxAutoEntries[key] = entry.CloneVT()
	a.mailboxAutoEntriesMtx.Unlock()
}

func (a *ProviderAccount) getMailboxAutoProcessEntry(key mailboxAutoProcessKey) *api.MailboxEntry {
	a.mailboxAutoEntriesMtx.Lock()
	defer a.mailboxAutoEntriesMtx.Unlock()
	if entry := a.mailboxAutoEntries[key]; entry != nil {
		return entry.CloneVT()
	}
	return nil
}

func (a *ProviderAccount) clearMailboxAutoProcessEntry(key mailboxAutoProcessKey) {
	a.mailboxAutoEntriesMtx.Lock()
	delete(a.mailboxAutoEntries, key)
	a.mailboxAutoEntriesMtx.Unlock()
}

// buildMailboxAutoProcessRoutine builds the keyed owner-side mailbox processor.
func (a *ProviderAccount) buildMailboxAutoProcessRoutine(key mailboxAutoProcessKey) (keyed.Routine, struct{}) {
	return func(ctx context.Context) error {
		if key.soID == "" || key.entryID == 0 {
			return nil
		}
		if !a.canAccessOwnerMailbox() {
			return nil
		}
		entry := a.getMailboxAutoProcessEntry(key)
		if entry == nil {
			return nil
		}
		cli := a.GetSessionClient()
		if cli == nil {
			return errors.New("session client not available")
		}
		if err := a.processMailboxEntryWithReject(ctx, cli, key.soID, entry); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			a.le.WithError(err).
				WithField("sobject-id", key.soID).
				WithField("entry-id", key.entryID).
				Debug("live mailbox auto-process failed")
			return err
		}
		return nil
	}, struct{}{}
}

// triggerMailboxEntryAutoProcess queues an owner-side auto-accept for a
// newly-observed pending mailbox entry received via ws notify. Non-pending
// events are ignored; non-owner sessions that receive an event will no-op.
func (a *ProviderAccount) triggerMailboxEntryAutoProcess(
	_ context.Context,
	soID string,
	entry *api.MailboxEntry,
) {
	if entry == nil || soID == "" || entry.GetStatus() != "pending" {
		return
	}
	if !a.canAccessOwnerMailbox() {
		return
	}
	key := mailboxAutoProcessKey{
		soID:    soID,
		entryID: entry.GetId(),
	}
	a.setMailboxAutoProcessEntry(key, entry)
	a.mailboxAutoProcessors.SetKey(key, false)
}

// processMailboxEntryWithReject accepts a pending mailbox entry, rejecting it
// via a follow-up RPC when validation fails so the queue drains.
func (a *ProviderAccount) processMailboxEntryWithReject(
	ctx context.Context,
	cli *SessionClient,
	soID string,
	entry *api.MailboxEntry,
) error {
	err := a.processMailboxEntry(ctx, cli, soID, entry, true)
	if err == nil {
		return nil
	}
	var invalidErr *invalidMailboxEntryError
	if !errors.As(err, &invalidErr) {
		return err
	}
	if _, rejectErr := cli.ProcessMailboxEntry(ctx, soID, &api.ProcessMailboxEntryRequest{
		Id:     entry.GetId(),
		Accept: false,
	}); rejectErr != nil {
		return rejectErr
	}
	a.RemovePendingMailboxEntry(soID, entry.GetId())
	return nil
}

func (a *ProviderAccount) processMailboxEntry(
	ctx context.Context,
	cli *SessionClient,
	soID string,
	entry *api.MailboxEntry,
	accept bool,
) error {
	if entry == nil {
		return errors.New("mailbox entry is required")
	}

	if !accept {
		if _, err := cli.ProcessMailboxEntry(ctx, soID, &api.ProcessMailboxEntryRequest{
			Id:     entry.GetId(),
			Accept: false,
		}); err != nil {
			return err
		}
		a.RemovePendingMailboxEntry(soID, entry.GetId())
		return nil
	}

	swSO, rel, err := a.mountSpaceSO(ctx, soID)
	if err != nil {
		return err
	}
	defer rel()

	invite, responderPeerID, responderPub, err := validateMailboxEntryForAccept(ctx, swSO, entry)
	if err != nil {
		return err
	}

	grant, err := swSO.AddParticipant(
		ctx,
		responderPeerID.String(),
		responderPub,
		invite.GetRole(),
		entry.GetAccountId(),
	)
	if err != nil {
		return err
	}
	if grant != nil {
		if err := swSO.IncrementInviteUses(ctx, swSO.privKey, invite.GetInviteId()); err != nil {
			return errors.Wrap(err, "increment invite uses")
		}
	}

	if _, err := cli.ProcessMailboxEntry(ctx, soID, &api.ProcessMailboxEntryRequest{
		Id:     entry.GetId(),
		Accept: true,
	}); err != nil {
		return err
	}
	a.RemovePendingMailboxEntry(soID, entry.GetId())
	return nil
}

func validateMailboxEntryForAccept(
	ctx context.Context,
	swSO *SharedObject,
	entry *api.MailboxEntry,
) (*sobject.SOInvite, peer.ID, crypto.PubKey, error) {
	if entry == nil {
		return nil, "", nil, errors.New("mailbox entry is required")
	}
	if entry.GetAccountId() == "" {
		return nil, "", nil, &invalidMailboxEntryError{err: errors.New("mailbox entry account_id is required")}
	}

	joinResp := entry.GetJoinResponse()
	responderPeerID, responderPub, err := sobject_invite.ValidateJoinResponse(joinResp)
	if err != nil {
		return nil, "", nil, &invalidMailboxEntryError{err: err}
	}
	if responderPeerID.String() != entry.GetPeerId() {
		return nil, "", nil, &invalidMailboxEntryError{err: errors.New("mailbox entry peer_id does not match join response")}
	}
	if joinResp.GetInviteId() != entry.GetInviteId() {
		return nil, "", nil, &invalidMailboxEntryError{err: errors.New("mailbox entry invite_id does not match join response")}
	}

	state, err := swSO.GetSOHost().GetHostState(ctx)
	if err != nil {
		return nil, "", nil, errors.Wrap(err, "get current shared object state")
	}

	var invite *sobject.SOInvite
	for _, candidate := range state.GetInvites() {
		if candidate.GetInviteId() == entry.GetInviteId() {
			invite = candidate
			break
		}
	}
	if invite == nil {
		return nil, "", nil, &invalidMailboxEntryError{err: errors.New("invite not found")}
	}
	if err := sobject.ValidateInviteUsable(invite); err != nil {
		return nil, "", nil, &invalidMailboxEntryError{err: err}
	}
	if targetPeerID := invite.GetTargetPeerId(); targetPeerID != "" && targetPeerID != responderPeerID.String() {
		return nil, "", nil, &invalidMailboxEntryError{err: errors.New("invite is targeted to a different peer")}
	}

	return invite, responderPeerID, responderPub, nil
}
