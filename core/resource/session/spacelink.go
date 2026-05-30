package resource_session

import (
	"context"
	"net/url"
	"strings"
	"time"

	"github.com/pkg/errors"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	core_session "github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

const (
	spaceLinkAuthRequestVersion = 1
	spaceLinkNonceLength        = 16
)

type verifiedSpaceLinkTicket struct {
	ticket      *s4wave_provider_spacewave.SpaceLinkAuthTicket
	payload     *s4wave_provider_spacewave.SpaceLinkAuthRequest
	agentPeerID peer.ID
	agentPub    crypto.PubKey
}

type spaceLinkNonceConsumer interface {
	ConsumeSpaceLinkNonce(ctx context.Context, agentPeerID, nonce, payload []byte, expiresAt time.Time) error
}

type spaceLinkSessionRegistrar interface {
	RegisterSessionWithRequest(ctx context.Context, req *api.RegisterSessionRequest, turnstileToken string) (*api.RegisterSessionResponse, error)
	RollbackSessionRegistration(ctx context.Context, sessionPeerID string) error
}

type spaceLinkTargetSpace interface {
	requireApproverOwner(ctx context.Context, approverPeerID string) error
	addParticipant(ctx context.Context, peerID string, pub crypto.PubKey, role sobject.SOParticipantRole, accountID string) error
}

type mountedSpaceLinkTargetSpace struct {
	swSO *provider_spacewave.SharedObject
}

func (m mountedSpaceLinkTargetSpace) requireApproverOwner(ctx context.Context, approverPeerID string) error {
	return requireSpaceLinkApproverOwner(ctx, m.swSO, approverPeerID)
}

func (m mountedSpaceLinkTargetSpace) addParticipant(
	ctx context.Context,
	peerID string,
	pub crypto.PubKey,
	role sobject.SOParticipantRole,
	accountID string,
) error {
	_, err := m.swSO.AddParticipant(ctx, peerID, pub, role, accountID)
	return err
}

func verifySpaceLinkTicketData(data []byte, now time.Time) (*verifiedSpaceLinkTicket, error) {
	ticket, err := unmarshalSpaceLinkAuthTicket(data)
	if err != nil {
		return nil, err
	}
	payload := &s4wave_provider_spacewave.SpaceLinkAuthRequest{}
	if err := payload.UnmarshalVT(ticket.GetPayload()); err != nil {
		return nil, errors.Wrap(err, "unmarshal spacelink auth payload")
	}
	if payload.GetVersion() != spaceLinkAuthRequestVersion {
		return nil, errors.New("unsupported spacelink version")
	}

	agentPeerID, err := peer.IDFromBytes(payload.GetAgentPeerId())
	if err != nil {
		return nil, errors.Wrap(err, "parse spacelink agent peer id")
	}
	agentPub, err := agentPeerID.ExtractPublicKey()
	if err != nil {
		return nil, errors.Wrap(err, "extract spacelink agent public key")
	}
	ok, err := agentPub.Verify(ticket.GetPayload(), ticket.GetAgentSignature())
	if err != nil {
		return nil, errors.Wrap(err, "verify spacelink agent signature")
	}
	if !ok {
		return nil, errors.New("invalid spacelink agent signature")
	}

	if payload.GetExpiresAt() <= now.Unix() {
		return nil, errors.New("spacelink ticket expired")
	}
	if len(payload.GetNonce()) != spaceLinkNonceLength {
		return nil, errors.New("spacelink nonce must be 16 bytes")
	}
	if err := validateSpaceLinkSessionType(payload.GetSessionType()); err != nil {
		return nil, err
	}
	if err := validateSpaceLinkRequestedRole(payload.GetRequestedRole()); err != nil {
		return nil, err
	}
	if err := validateSpaceLinkCompletion(payload.GetCompletionMode(), payload.GetCallbackUrl()); err != nil {
		return nil, err
	}

	return &verifiedSpaceLinkTicket{
		ticket:      ticket,
		payload:     payload,
		agentPeerID: agentPeerID,
		agentPub:    agentPub,
	}, nil
}

func validateSpaceLinkSessionType(typ core_session.SessionType) error {
	switch typ {
	case core_session.SessionType_SESSION_TYPE_APP,
		core_session.SessionType_SESSION_TYPE_DEVICE:
		return nil
	default:
		return errors.New("unsupported spacelink session type")
	}
}

func validateSpaceLinkRequestedRole(role sobject.SOParticipantRole) error {
	switch role {
	case sobject.SOParticipantRole_SOParticipantRole_READER,
		sobject.SOParticipantRole_SOParticipantRole_WRITER:
		return nil
	default:
		return errors.New("unsupported spacelink requested role")
	}
}

func validateSpaceLinkCompletion(
	mode s4wave_provider_spacewave.SpaceLinkCompletionMode,
	callbackURL string,
) error {
	switch mode {
	case s4wave_provider_spacewave.SpaceLinkCompletionMode_SpaceLinkCompletionMode_BROWSER_CALLBACK:
		return validateSpaceLinkLoopbackCallbackURL(callbackURL)
	case s4wave_provider_spacewave.SpaceLinkCompletionMode_SpaceLinkCompletionMode_CLI:
		if callbackURL != "" {
			return errors.New("spacelink cli completion cannot include callback_url")
		}
		return nil
	default:
		return errors.New("unsupported spacelink completion mode")
	}
}

func validateSpaceLinkLoopbackCallbackURL(raw string) error {
	if raw == "" {
		return errors.New("spacelink callback_url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return errors.Wrap(err, "parse spacelink callback_url")
	}
	if u.Scheme != "http" {
		return errors.New("spacelink callback_url must use http")
	}
	if u.User != nil {
		return errors.New("spacelink callback_url must not include userinfo")
	}
	if u.Fragment != "" {
		return errors.New("spacelink callback_url must not include fragment")
	}
	if u.Port() == "" {
		return errors.New("spacelink callback_url must include an explicit port")
	}
	host := strings.ToLower(u.Hostname())
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return nil
	default:
		return errors.New("spacelink callback_url must be loopback")
	}
}

func approveVerifiedSpaceLink(
	ctx context.Context,
	verified *verifiedSpaceLinkTicket,
	resourceID []byte,
	approverPeerID string,
	nonceConsumer spaceLinkNonceConsumer,
	registrar spaceLinkSessionRegistrar,
	target spaceLinkTargetSpace,
) (*s4wave_provider_spacewave.ApproveSpaceLinkResponse, error) {
	if verified == nil || verified.payload == nil || verified.ticket == nil {
		return nil, errors.New("verified spacelink ticket is required")
	}
	payload := verified.payload
	resourceIDStr := string(resourceID)
	if resourceIDStr == "" {
		return nil, errors.New("resource_id is required")
	}
	if target == nil {
		return nil, errors.New("target space is required")
	}
	if nonceConsumer == nil {
		return nil, errors.New("spacelink nonce consumer is not ready")
	}
	if registrar == nil {
		return nil, errors.New("entity client is not ready")
	}

	if err := target.requireApproverOwner(ctx, approverPeerID); err != nil {
		return nil, err
	}
	if err := nonceConsumer.ConsumeSpaceLinkNonce(
		ctx,
		payload.GetAgentPeerId(),
		payload.GetNonce(),
		verified.ticket.GetPayload(),
		time.Unix(payload.GetExpiresAt(), 0),
	); err != nil {
		return nil, err
	}

	sessionPeerID := verified.agentPeerID.String()
	registerResp, err := registrar.RegisterSessionWithRequest(ctx, &api.RegisterSessionRequest{
		SessionPeerId: sessionPeerID,
		Type:          payload.GetSessionType(),
		Label:         payload.GetLabel(),
	}, "")
	if err != nil {
		return nil, err
	}
	if registerResp == nil {
		return nil, errors.New("session registration response is required")
	}
	if err := target.addParticipant(
		ctx,
		sessionPeerID,
		verified.agentPub,
		payload.GetRequestedRole(),
		registerResp.GetAccountId(),
	); err != nil {
		if registerResp.GetCreated() {
			if rollbackErr := registrar.RollbackSessionRegistration(ctx, sessionPeerID); rollbackErr != nil {
				return nil, errors.Wrapf(err, "add spacelink participant; rollback failed: %v", rollbackErr)
			}
		}
		return nil, err
	}

	completion := &s4wave_provider_spacewave.SpaceLinkCallback{
		Status:        s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_OK,
		Nonce:         payload.GetNonce(),
		AccountId:     registerResp.GetAccountId(),
		ResourceId:    resourceID,
		SessionPeerId: payload.GetAgentPeerId(),
	}
	return &s4wave_provider_spacewave.ApproveSpaceLinkResponse{
		AccountId:      registerResp.GetAccountId(),
		ResourceId:     resourceID,
		SessionPeerId:  payload.GetAgentPeerId(),
		Nonce:          payload.GetNonce(),
		CallbackUrl:    payload.GetCallbackUrl(),
		CompletionMode: payload.GetCompletionMode(),
		Completion:     completion,
	}, nil
}

func requireSpaceLinkApproverOwner(
	ctx context.Context,
	swSO *provider_spacewave.SharedObject,
	approverPeerID string,
) error {
	if swSO == nil {
		return errors.New("target space is required")
	}
	if approverPeerID == "" {
		return errors.New("approver peer id is required")
	}
	state, err := swSO.GetSOHost().GetHostState(ctx)
	if err != nil {
		return errors.Wrap(err, "get target space state")
	}
	for _, participant := range state.GetConfig().GetParticipants() {
		if participant.GetPeerId() == approverPeerID && sobject.IsOwner(participant.GetRole()) {
			return nil
		}
	}
	return errors.New("spacelink approver must be OWNER on the target space")
}
