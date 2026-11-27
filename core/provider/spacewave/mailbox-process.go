package provider_spacewave

import (
	"bytes"
	"context"
	"crypto/sha256"
	"slices"
	"time"

	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_invite "github.com/s4wave/spacewave/core/sobject/invite"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// mailboxAutoProcessKey identifies one mailbox entry of one Space.
type mailboxAutoProcessKey struct {
	soID    string
	entryID int64
}

// invalidMailboxEntryError marks a mailbox entry that can never be accepted,
// which the owner session rejects so the queue drains.
type invalidMailboxEntryError struct {
	err error
}

// Error returns the reason the entry is invalid.
func (e *invalidMailboxEntryError) Error() string {
	return e.err.Error()
}

// Unwrap returns the reason the entry is invalid.
func (e *invalidMailboxEntryError) Unwrap() error {
	return e.err
}

// ProcessMailboxEntry accepts or rejects a mailbox entry for a cloud space.
//
// Accepting or rejecting is the owner's decision: an accept admits the
// redeemer of any usable invite, whatever conditions the invite carries.
func (a *ProviderAccount) ProcessMailboxEntry(
	ctx context.Context,
	soID string,
	entryID int64,
	accept bool,
) error {
	// Validate the mailbox target and authenticated client.
	if soID == "" {
		return errors.New("shared object id is required")
	}
	if entryID == 0 {
		return errors.New("entry id is required")
	}
	cli, err := a.GetSessionClient()
	if err != nil {
		return err
	}

	// Load the pending mailbox snapshot.
	resp, err := a.getPendingMailboxResponseCached(ctx, soID)
	if err != nil {
		return err
	}

	// Resolve the requested mailbox entry.
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

	// Apply the requested accept or reject action.
	if !accept {
		return a.rejectMailboxEntry(ctx, cli, soID, entry)
	}
	return a.acceptMailboxEntry(ctx, cli, soID, entry, false)
}

// processPendingMailboxEntries processes the current pending mailbox queue for
// an owned cloud space.
func (a *ProviderAccount) processPendingMailboxEntries(
	ctx context.Context,
	soID string,
) error {
	// Validate mailbox processing authority.
	if soID == "" {
		return errors.New("shared object id is required")
	}
	if !a.canAccessOwnerMailbox() {
		return nil
	}
	cli, err := a.GetSessionClient()
	if err != nil {
		return err
	}

	// Load pending entries and process them in order.
	resp, err := a.getPendingMailboxResponseCached(ctx, soID)
	if err != nil {
		return err
	}

	// Decide each pending entry under its invite's conditions.
	for _, entry := range resp.GetEntries() {
		if err := a.autoProcessMailboxEntry(ctx, cli, soID, entry); err != nil {
			return err
		}
	}
	return nil
}

// setMailboxAutoProcessEntry stores the entry an auto-process routine decides.
func (a *ProviderAccount) setMailboxAutoProcessEntry(key mailboxAutoProcessKey, entry *api.MailboxEntry) {
	a.mailboxAutoEntriesMtx.Lock()
	if a.mailboxAutoEntries == nil {
		a.mailboxAutoEntries = make(map[mailboxAutoProcessKey]*api.MailboxEntry)
	}
	a.mailboxAutoEntries[key] = entry.CloneVT()
	a.mailboxAutoEntriesMtx.Unlock()
}

// getMailboxAutoProcessEntry returns a copy of the stored entry, or nil.
func (a *ProviderAccount) getMailboxAutoProcessEntry(key mailboxAutoProcessKey) *api.MailboxEntry {
	// Copy the entry under the lock.
	a.mailboxAutoEntriesMtx.Lock()
	defer a.mailboxAutoEntriesMtx.Unlock()
	if entry := a.mailboxAutoEntries[key]; entry != nil {
		return entry.CloneVT()
	}
	return nil
}

// clearMailboxAutoProcessEntry forgets the stored entry.
func (a *ProviderAccount) clearMailboxAutoProcessEntry(key mailboxAutoProcessKey) {
	a.mailboxAutoEntriesMtx.Lock()
	delete(a.mailboxAutoEntries, key)
	a.mailboxAutoEntriesMtx.Unlock()
}

// buildMailboxAutoProcessRoutine builds the keyed owner-side mailbox processor.
func (a *ProviderAccount) buildMailboxAutoProcessRoutine(key mailboxAutoProcessKey) (keyed.Routine, struct{}) {
	return func(ctx context.Context) error {
		// Validate the keyed auto-process request.
		if key.soID == "" || key.entryID == 0 {
			return nil
		}
		if !a.canAccessOwnerMailbox() {
			return nil
		}

		// Load the queued mailbox entry and authenticated client.
		entry := a.getMailboxAutoProcessEntry(key)
		if entry == nil {
			return nil
		}
		cli, err := a.GetSessionClient()
		if err != nil {
			return err
		}

		// Process the queued entry and preserve cancellation errors.
		if err := a.autoProcessMailboxEntry(ctx, cli, key.soID, entry); err != nil {
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

// triggerMailboxEntryAutoProcess queues the owner-side decision of a
// newly observed pending mailbox entry received via ws notify. Non-pending
// events are ignored. Non-owner sessions that receive an event will no-op.
func (a *ProviderAccount) triggerMailboxEntryAutoProcess(
	_ context.Context,
	soID string,
	entry *api.MailboxEntry,
) {
	// Only an owner session decides pending entries.
	if entry == nil || soID == "" || entry.GetStatus() != "pending" {
		return
	}
	if !a.canAccessOwnerMailbox() {
		return
	}

	// Store the entry and start its routine.
	key := mailboxAutoProcessKey{
		soID:    soID,
		entryID: entry.GetId(),
	}
	a.setMailboxAutoProcessEntry(key, entry)
	a.mailboxAutoProcessors.SetKey(key, false)
}

// autoProcessMailboxEntry decides a pending mailbox entry under its invite's
// conditions. It accepts an entry the invite admits, rejects an invalid or
// refused entry so the queue drains, and leaves a join request pending for
// the owner.
func (a *ProviderAccount) autoProcessMailboxEntry(
	ctx context.Context,
	cli *SessionClient,
	soID string,
	entry *api.MailboxEntry,
) error {
	// Try the conditional accept before rejecting invalid entries.
	err := a.acceptMailboxEntry(ctx, cli, soID, entry, true)
	var invalidErr *invalidMailboxEntryError
	if !errors.As(err, &invalidErr) {
		return err
	}
	return a.rejectMailboxEntry(ctx, cli, soID, entry)
}

// rejectMailboxEntry rejects a mailbox entry and removes it from the local queue.
func (a *ProviderAccount) rejectMailboxEntry(
	ctx context.Context,
	cli *SessionClient,
	soID string,
	entry *api.MailboxEntry,
) error {
	if _, err := cli.ProcessMailboxEntry(ctx, soID, &api.ProcessMailboxEntryRequest{
		Id:     entry.GetId(),
		Accept: false,
	}); err != nil {
		return err
	}
	a.RemovePendingMailboxEntry(soID, entry.GetId())
	return nil
}

// acceptMailboxEntry admits the redeemer of a valid mailbox entry.
//
// With conditional set, the invite's conditions decide first: a refused
// redeemer returns an invalidMailboxEntryError, and a join request awaiting
// approval stays pending without a decision.
func (a *ProviderAccount) acceptMailboxEntry(
	ctx context.Context,
	cli *SessionClient,
	soID string,
	entry *api.MailboxEntry,
	conditional bool,
) error {
	// Mount the shared object for accepted-entry validation.
	swSO, rel, err := a.mountSpaceSO(ctx, soID)
	if err != nil {
		return err
	}
	defer rel()

	// Resolve the owner account for targeted invitations.
	ownerAccountID := ""
	if entry.GetTargetedEnvelope() != nil {
		account, err := cli.GetAccountInfo(ctx)
		if err != nil {
			return errors.Wrap(err, "get owner account info")
		}
		ownerAccountID = account.GetAccountId()
	}

	// Validate the invitation and responder proof.
	redemption, err := validateMailboxEntryForAccept(ctx, swSO, soID, entry, ownerAccountID)
	if err != nil {
		return err
	}
	invite, responderPeerID := redemption.invite, redemption.peerID

	// Admit, queue or refuse the redeemer under the invite's conditions.
	if conditional && !redemption.enrolled {
		held, err := a.heldSpaceConfigs(ctx, invite)
		if err != nil {
			return err
		}
		switch sobject.RedeemInvite(invite, responderPeerID.String(), held) {
		case sobject.InviteRedemptionRefuse:
			return &invalidMailboxEntryError{err: errors.New("invite does not admit this peer")}
		case sobject.InviteRedemptionQueue:
			return nil
		}
	}

	// Add the responder participant. A new grant consumes an invite use.
	grant, _, _, err := swSO.AddParticipant(
		ctx,
		responderPeerID.String(),
		redemption.pubKey,
		invite.GetRole(),
		entry.GetAccountId(),
		entry.GetEntityId(),
	)
	if err != nil {
		return err
	}
	if grant != nil {
		if err := swSO.IncrementInviteUses(ctx, swSO.privKey, invite.GetInviteId()); err != nil {
			return errors.Wrap(err, "increment invite uses")
		}
	}

	// Commit the accepted mailbox decision to the cloud.
	if _, err := cli.ProcessMailboxEntry(ctx, soID, &api.ProcessMailboxEntryRequest{
		Id:     entry.GetId(),
		Accept: true,
	}); err != nil {
		return err
	}
	a.RemovePendingMailboxEntry(soID, entry.GetId())
	return nil
}

// heldSpaceConfigs reads this account's configuration of each Space a
// participant_of invite lists, skipping Spaces the account does not list.
func (a *ProviderAccount) heldSpaceConfigs(ctx context.Context, invite *sobject.SOInvite) (map[string]*sobject.SharedObjectConfig, error) {
	// Without named Spaces there is nothing to read.
	ids := invite.GetParticipantOf().GetSharedObjectIds()
	if len(ids) == 0 {
		return nil, nil
	}

	// Read each named Space the account lists from its own copy.
	if err := a.EnsureSharedObjectListLoaded(ctx); err != nil {
		return nil, err
	}
	named := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		named[id] = struct{}{}
	}
	held := make(map[string]*sobject.SharedObjectConfig, len(ids))
	for _, listed := range a.soListCtr.GetValue().GetSharedObjects() {
		id := listed.GetRef().GetProviderResourceRef().GetId()
		if _, ok := named[id]; !ok {
			continue
		}
		config, err := a.readSpaceConfig(ctx, id)
		if err != nil {
			return nil, errors.Wrapf(err, "read held space %s", id)
		}
		held[id] = config
	}
	return held, nil
}

// readSpaceConfig reads the current configuration of a Space this account holds.
func (a *ProviderAccount) readSpaceConfig(ctx context.Context, soID string) (*sobject.SharedObjectConfig, error) {
	// Mount the Space.
	swSO, rel, err := a.mountSpaceSO(ctx, soID)
	if err != nil {
		return nil, err
	}
	defer rel()

	// Read its current configuration from the host state.
	state, err := swSO.host.getHostState(ctx)
	if err != nil {
		return nil, err
	}
	return state.GetConfig(), nil
}

// mailboxRedemption is a mailbox entry that validated against the Space.
type mailboxRedemption struct {
	// invite is the invite the entry redeems.
	invite *sobject.SOInvite
	// peerID is the responder's peer ID.
	peerID peer.ID
	// pubKey is the responder's public key.
	pubKey crypto.PubKey
	// enrolled is set when the responder already holds the invite's role, so
	// the invite's own checks and conditions do not apply.
	enrolled bool
}

// validateMailboxEntryForAccept checks a mailbox entry against the current
// Space state and returns the redemption it describes. An entry that can never
// be accepted returns an invalidMailboxEntryError.
func validateMailboxEntryForAccept(
	ctx context.Context,
	swSO *SharedObject,
	soID string,
	entry *api.MailboxEntry,
	ownerAccountID string,
) (*mailboxRedemption, error) {
	// Check the entry against its signed join response.
	if entry == nil {
		return nil, errors.New("mailbox entry is required")
	}
	if entry.GetAccountId() == "" {
		return nil, &invalidMailboxEntryError{err: errors.New("mailbox entry account_id is required")}
	}
	joinResp := entry.GetJoinResponse()
	responderPeerID, responderPub, err := sobject_invite.ValidateJoinResponse(joinResp)
	if err != nil {
		return nil, &invalidMailboxEntryError{err: err}
	}
	if responderPeerID.String() != entry.GetPeerId() {
		return nil, &invalidMailboxEntryError{err: errors.New("mailbox entry peer_id does not match join response")}
	}
	if joinResp.GetInviteId() != entry.GetInviteId() {
		return nil, &invalidMailboxEntryError{err: errors.New("mailbox entry invite_id does not match join response")}
	}

	// Find the redeemed invite in the current state.
	state, err := swSO.host.getHostState(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get current shared object state")
	}
	invite := sobject.FindInvite(state, entry.GetInviteId())
	if invite == nil {
		return nil, &invalidMailboxEntryError{err: errors.New("invite not found")}
	}

	// A responder the Space already enrolled at the invite's role is accepted
	// however the invite changed since, because the repeat changes nothing.
	redemption := &mailboxRedemption{invite: invite, peerID: responderPeerID, pubKey: responderPub}
	if sobject.InviteEnrolled(state.GetConfig(), invite, responderPeerID.String()) {
		redemption.enrolled = true
		return redemption, nil
	}

	// Check that the invite still admits this responder.
	if err := sobject.ValidateInviteRedeemable(invite, responderPeerID.String()); err != nil {
		return nil, &invalidMailboxEntryError{err: err}
	}
	if err := validateTargetedMailboxProof(entry, invite, state, soID, ownerAccountID); err != nil {
		return nil, &invalidMailboxEntryError{err: err}
	}
	return redemption, nil
}

// validateTargetedMailboxProof checks the owner-signed envelope that proves an
// account-targeted invitation was issued to the entry's account.
func validateTargetedMailboxProof(
	entry *api.MailboxEntry,
	invite *sobject.SOInvite,
	state *sobject.SOState,
	soID string,
	ownerAccountID string,
) error {
	// An untargeted invite needs no envelope.
	envelope := entry.GetTargetedEnvelope()
	if envelope == nil {
		if invite.GetTargetAccountId() != "" {
			return errors.New("targeted invitation proof is required")
		}
		return nil
	}
	if ownerAccountID == "" {
		return errors.New("owner account id is required for targeted mailbox proof")
	}

	// Check the envelope's signature, scope and expiry.
	if err := VerifyTargetedInvitationEnvelope(envelope); err != nil {
		return err
	}
	if envelope.GetSchemaVersion() != 1 {
		return errors.New("unsupported targeted invitation envelope version")
	}
	if envelope.GetPurpose() != api.TargetedInvitePurpose_TARGETED_INVITE_PURPOSE_SPACE {
		return errors.New("targeted invitation purpose is not space")
	}
	if envelope.GetExpiresAt() > 0 && envelope.GetExpiresAt() <= time.Now().UnixMilli() {
		return errors.New("targeted invitation is expired")
	}

	// Check that a current owner addressed this Space to the entry's account.
	if envelope.GetActorAccountId() != ownerAccountID {
		return errors.New("targeted invitation actor is not the owner account")
	}
	if envelope.GetContextId() == "" || envelope.GetContextId() != soID {
		return errors.New("targeted invitation context mismatch")
	}
	if envelope.GetTargetAccountId() != entry.GetAccountId() {
		return errors.New("targeted invitation target account mismatch")
	}
	if invite.GetTargetAccountId() != "" &&
		envelope.GetTargetAccountId() != invite.GetTargetAccountId() {
		return errors.New("targeted invitation invite account mismatch")
	}
	if !isCurrentTargetedInviteSigner(state.GetConfig(), envelope.GetSignerPeerId()) {
		return errors.New("targeted invitation signer is not a current owner")
	}

	// Check that the payload is the redeemed invite, signed by the envelope signer.
	inviteMsg := &sobject.SOInviteMessage{}
	if err := inviteMsg.UnmarshalVT(envelope.GetPayload()); err != nil {
		return errors.Wrap(err, "unmarshal targeted space invite payload")
	}
	if inviteMsg.GetInviteId() != entry.GetInviteId() || inviteMsg.GetInviteId() != invite.GetInviteId() {
		return errors.New("targeted invitation payload invite mismatch")
	}
	if inviteMsg.GetSharedObjectId() != envelope.GetContextId() {
		return errors.New("targeted invitation payload context mismatch")
	}
	if inviteMsg.GetOwnerPeerId() != envelope.GetSignerPeerId() {
		return errors.New("targeted invitation payload signer mismatch")
	}
	if inviteMsg.GetRole() != invite.GetRole() {
		return errors.New("targeted invitation payload role mismatch")
	}
	tokenHash := sha256.Sum256(inviteMsg.GetToken())
	if !bytes.Equal(tokenHash[:], invite.GetTokenHash()) {
		return errors.New("targeted invitation payload token mismatch")
	}

	// Check the envelope's role against the invite.
	role, err := targetedMailboxEnvelopeRole(envelope.GetRole())
	if err != nil {
		return err
	}
	if role != invite.GetRole() {
		return errors.New("targeted invitation role mismatch")
	}

	return validateTargetedMailboxInviteMessageSignature(inviteMsg)
}

// targetedMailboxEnvelopeRole maps an envelope role name to a participant role.
func targetedMailboxEnvelopeRole(role string) (sobject.SOParticipantRole, error) {
	switch role {
	case "", "reader":
		return sobject.SOParticipantRole_SOParticipantRole_READER, nil
	case "writer":
		return sobject.SOParticipantRole_SOParticipantRole_WRITER, nil
	default:
		return sobject.SOParticipantRole_SOParticipantRole_UNKNOWN, errors.New("unsupported targeted invitation role")
	}
}

// isCurrentTargetedInviteSigner reports whether peerID is a current owner.
func isCurrentTargetedInviteSigner(cfg *sobject.SharedObjectConfig, peerID string) bool {
	return slices.ContainsFunc(cfg.GetParticipants(), func(p *sobject.SOParticipantConfig) bool {
		return p.GetPeerId() == peerID && sobject.IsOwner(p.GetRole())
	})
}

// validateTargetedMailboxInviteMessageSignature verifies the invite message's
// signature by the owner peer the message references.
func validateTargetedMailboxInviteMessageSignature(inviteMsg *sobject.SOInviteMessage) error {
	// Read the signature and the signer's public key.
	if inviteMsg == nil {
		return errors.New("targeted invitation payload is required")
	}
	sig := inviteMsg.GetSignature()
	if sig == nil {
		return errors.New("targeted invitation payload signature is required")
	}
	ownerPeerID, err := peer.IDB58Decode(inviteMsg.GetOwnerPeerId())
	if err != nil {
		return errors.Wrap(err, "parse targeted invitation owner peer ID")
	}
	ownerPub, err := ownerPeerID.ExtractPublicKey()
	if err != nil {
		return errors.Wrap(err, "extract targeted invitation owner public key")
	}

	// Verify the signature over the unsigned message.
	body := inviteMsg.CloneVT()
	body.Signature = nil
	data, err := body.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal targeted invitation payload")
	}
	valid, err := sig.VerifyWithPublic("sobject invite", ownerPub, data)
	if err != nil {
		return errors.Wrap(err, "verify targeted invitation payload signature")
	}
	if !valid {
		return errors.New("targeted invitation payload signature is invalid")
	}

	return nil
}
