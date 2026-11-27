package resource_session

import (
	"context"
	"encoding/base64"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// RedeemInvite redeems a platform invite code for the session's account.
func (r *SpacewaveSessionResource) RedeemInvite(
	ctx context.Context,
	req *s4wave_provider_spacewave.RedeemInviteRequest,
) (*s4wave_provider_spacewave.RedeemInviteResponse, error) {
	// Resolve the current signer before the cloud request.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}

	// Redeem the invite through the current signer.
	return cli.RedeemInvite(ctx, req.GetCode())
}

// Invite flow.
//
// The invite lifecycle spans the local invite host and the cloud:
//
//  1. Owner: SessionResource.CreateSpaceInvite signs an SOInviteMessage with
//     the owner's session key via InviteHost.CreateSOInviteOp. The caller
//     keeps the signed message and its token in memory. The SOInvite, which
//     holds only the token hash, is appended to the SO config chain.
//
//  2. Owner: for spacewave-backed sessions, RegisterInviteCode publishes a
//     short code (8 chars) that maps to the base64 SOInviteMessage with the
//     same expiry. The code is cloud-only convenience storage.
//
//  3. Joiner: LookupInviteCode below resolves a short code to the original
//     SOInviteMessage. The joiner submits a join request to the Space's cloud
//     mailbox with the token and a signed join response. The cloud checks
//     them against the SOInvite in the config chain. The owner applies the
//     enrollment via ProcessMailboxEntry.
//
// Local-vs-cloud authority boundary:
//
//   - Local (spacewave package): builds and signs the SOInviteMessage,
//     verifies the message, drives mailbox processing, and applies invite
//     mutations to the SO config chain. These actions are authoritative. The
//     cloud cannot forge them.
//   - Cloud: stores the short codes and the mailbox. Codes expire, and losing
//     one never compromises the invite state in the config chain. A failed
//     RegisterInviteCode is logged and ignored at the call site because the
//     message still works as a long-form invite link.

// LookupInviteCode resolves a short invite code to the full SOInviteMessage.
func (r *SpacewaveSessionResource) LookupInviteCode(
	ctx context.Context,
	req *s4wave_provider_spacewave.LookupInviteCodeRequest,
) (*s4wave_provider_spacewave.LookupInviteCodeResponse, error) {
	// Require an invite code before consulting the Cloud lookup table.
	code := req.GetCode()
	if code == "" {
		return nil, errors.New("code is required")
	}

	// Resolve the code to its stored invite message.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}
	lookupResp, err := cli.LookupInviteCode(ctx, code)
	if err != nil {
		return nil, err
	}

	// Decode the base64 invite message to SOInviteMessage proto.
	inviteMsgBytes, err := base64.StdEncoding.DecodeString(lookupResp.GetInviteMessage())
	if err != nil {
		return nil, errors.Wrap(err, "decode invite message")
	}
	inviteMsg := &sobject.SOInviteMessage{}
	if err := inviteMsg.UnmarshalVT(inviteMsgBytes); err != nil {
		return nil, errors.Wrap(err, "unmarshal invite message")
	}

	// Return the cloud result in the resource response.
	return &s4wave_provider_spacewave.LookupInviteCodeResponse{
		InviteId:      lookupResp.GetInviteId(),
		InviteMessage: inviteMsg,
	}, nil
}

// ProcessMailboxEntry accepts or rejects a mailbox entry.
func (r *SpacewaveSessionResource) ProcessMailboxEntry(
	ctx context.Context,
	req *s4wave_provider_spacewave.ProcessMailboxEntryRequest,
) (*s4wave_provider_spacewave.ProcessMailboxEntryResponse, error) {
	// Require the Space and mailbox entry before processing the decision.
	spaceID := req.GetSpaceId()
	if spaceID == "" {
		return nil, errors.New("space_id is required")
	}
	entryID := req.GetEntryId()
	if entryID == 0 {
		return nil, errors.New("entry_id is required")
	}

	// Apply the mailbox acceptance or rejection through the account tracker.
	if err := r.swAcc.ProcessMailboxEntry(ctx, spaceID, entryID, req.GetAccept()); err != nil {
		return nil, err
	}

	return &s4wave_provider_spacewave.ProcessMailboxEntryResponse{}, nil
}
