package resource_session

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

func TestVerifySpaceLinkTicketData(t *testing.T) {
	now := time.Unix(100, 0)
	ticketBytes, payload := buildTestSpaceLinkTicket(t, now, func(req *s4wave_provider_spacewave.SpaceLinkAuthRequest) {})

	verified, err := verifySpaceLinkTicketData(ticketBytes, now)
	if err != nil {
		t.Fatalf("verify ticket: %v", err)
	}
	if verified.payload.GetLabel() != payload.GetLabel() {
		t.Fatalf("label = %q, want %q", verified.payload.GetLabel(), payload.GetLabel())
	}
	if string(verified.payload.GetAgentPeerId()) != string(payload.GetAgentPeerId()) {
		t.Fatal("agent peer id mismatch")
	}
}
func TestVerifySpaceLinkTicketDataRejectsMalformedTickets(t *testing.T) {
	now := time.Unix(100, 0)
	for _, data := range [][]byte{nil, []byte("not-a-ticket")} {
		if _, err := verifySpaceLinkTicketData(data, now); err == nil {
			t.Fatalf("verify malformed ticket %q succeeded", data)
		}
	}
}

func TestVerifySpaceLinkTicketDataRejectsInvalidInputs(t *testing.T) {
	now := time.Unix(100, 0)
	tests := []struct {
		name    string
		mutate  func(*s4wave_provider_spacewave.SpaceLinkAuthRequest)
		tamper  func([]byte) []byte
		wantErr string
	}{
		{
			name: "bad signature",
			tamper: func(ticketBytes []byte) []byte {
				ticket := &s4wave_provider_spacewave.SpaceLinkAuthTicket{}
				if err := ticket.UnmarshalVT(ticketBytes); err != nil {
					t.Fatalf("unmarshal ticket: %v", err)
				}
				ticket.AgentSignature[0] ^= 0xff
				data, err := ticket.MarshalVT()
				if err != nil {
					t.Fatalf("marshal ticket: %v", err)
				}
				return data
			},
			wantErr: "invalid spacelink agent signature",
		},
		{
			name: "expired",
			mutate: func(req *s4wave_provider_spacewave.SpaceLinkAuthRequest) {
				req.ExpiresAt = now.Unix()
			},
			wantErr: "spacelink ticket expired",
		},
		{
			name: "invalid role",
			mutate: func(req *s4wave_provider_spacewave.SpaceLinkAuthRequest) {
				req.RequestedRole = sobject.SOParticipantRole_SOParticipantRole_OWNER
			},
			wantErr: "unsupported spacelink requested role",
		},
		{
			name: "invalid session type",
			mutate: func(req *s4wave_provider_spacewave.SpaceLinkAuthRequest) {
				req.SessionType = session.SessionType_SESSION_TYPE_USER
			},
			wantErr: "unsupported spacelink session type",
		},
		{
			name: "invalid completion mode",
			mutate: func(req *s4wave_provider_spacewave.SpaceLinkAuthRequest) {
				req.CompletionMode = s4wave_provider_spacewave.SpaceLinkCompletionMode_SpaceLinkCompletionMode_UNKNOWN
			},
			wantErr: "unsupported spacelink completion mode",
		},
		{
			name: "hosted callback",
			mutate: func(req *s4wave_provider_spacewave.SpaceLinkAuthRequest) {
				req.CallbackUrl = "https://example.com/callback"
			},
			wantErr: "spacelink callback_url must use http",
		},
		{
			name: "cli callback",
			mutate: func(req *s4wave_provider_spacewave.SpaceLinkAuthRequest) {
				req.CompletionMode = s4wave_provider_spacewave.SpaceLinkCompletionMode_SpaceLinkCompletionMode_CLI
				req.CallbackUrl = "http://127.0.0.1:9000/callback"
			},
			wantErr: "spacelink cli completion cannot include callback_url",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ticketBytes, _ := buildTestSpaceLinkTicket(t, now, tt.mutate)
			if tt.tamper != nil {
				ticketBytes = tt.tamper(ticketBytes)
			}
			_, err := verifySpaceLinkTicketData(ticketBytes, now)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("verify err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestApproveVerifiedSpaceLinkRegistersGrantsAndReturnsCompletion(t *testing.T) {
	now := time.Unix(100, 0)
	ticketBytes, payload := buildTestSpaceLinkTicket(t, now, nil)
	verified, err := verifySpaceLinkTicketData(ticketBytes, now)
	if err != nil {
		t.Fatalf("verify ticket: %v", err)
	}
	var events []string
	nonce := &testSpaceLinkNonceConsumer{events: &events}
	registrar := &testSpaceLinkRegistrar{
		events: &events,
		resp: &api.RegisterSessionResponse{
			AccountId: "acct-1",
			Created:   true,
		},
	}
	target := &testSpaceLinkTarget{events: &events}

	resp, err := approveVerifiedSpaceLink(
		context.Background(),
		verified,
		[]byte("space-1"),
		"approver-peer",
		nonce,
		registrar,
		target,
	)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	requireSpaceLinkEvents(t, events, "owner", "consume", "register", "add")
	if registrar.req.GetSessionPeerId() != verified.agentPeerID.String() {
		t.Fatalf("registered peer = %q, want %q", registrar.req.GetSessionPeerId(), verified.agentPeerID.String())
	}
	if registrar.req.GetType() != payload.GetSessionType() {
		t.Fatalf("registered type = %v, want %v", registrar.req.GetType(), payload.GetSessionType())
	}
	if registrar.req.GetLabel() != payload.GetLabel() {
		t.Fatalf("registered label = %q, want %q", registrar.req.GetLabel(), payload.GetLabel())
	}
	if target.addPeerID != verified.agentPeerID.String() {
		t.Fatalf("grant peer = %q, want %q", target.addPeerID, verified.agentPeerID.String())
	}
	if target.addPub == nil {
		t.Fatal("grant public key was not provided")
	}
	if target.addRole != payload.GetRequestedRole() {
		t.Fatalf("grant role = %v, want %v", target.addRole, payload.GetRequestedRole())
	}
	if target.addAccountID != "acct-1" {
		t.Fatalf("grant account = %q, want acct-1", target.addAccountID)
	}
	if resp.GetAccountId() != "acct-1" {
		t.Fatalf("response account = %q, want acct-1", resp.GetAccountId())
	}
	if string(resp.GetResourceId()) != "space-1" {
		t.Fatalf("response resource = %q, want space-1", string(resp.GetResourceId()))
	}
	if string(resp.GetSessionPeerId()) != string(payload.GetAgentPeerId()) {
		t.Fatal("response session peer id mismatch")
	}
	if resp.GetCompletion().GetStatus() != s4wave_provider_spacewave.SpaceLinkCallbackStatus_SpaceLinkCallbackStatus_OK {
		t.Fatalf("completion status = %v, want OK", resp.GetCompletion().GetStatus())
	}
	if string(resp.GetCompletion().GetSessionPeerId()) != string(payload.GetAgentPeerId()) {
		t.Fatal("completion session peer id mismatch")
	}
}

func TestApproveVerifiedSpaceLinkRejectsNonOwnerBeforeMutation(t *testing.T) {
	now := time.Unix(100, 0)
	ticketBytes, _ := buildTestSpaceLinkTicket(t, now, nil)
	verified, err := verifySpaceLinkTicketData(ticketBytes, now)
	if err != nil {
		t.Fatalf("verify ticket: %v", err)
	}
	var events []string
	nonce := &testSpaceLinkNonceConsumer{events: &events}
	registrar := &testSpaceLinkRegistrar{
		events: &events,
		resp:   &api.RegisterSessionResponse{AccountId: "acct-1", Created: true},
	}
	target := &testSpaceLinkTarget{
		events:   &events,
		ownerErr: errors.New("not owner"),
	}

	_, err = approveVerifiedSpaceLink(
		context.Background(),
		verified,
		[]byte("space-1"),
		"approver-peer",
		nonce,
		registrar,
		target,
	)
	if err == nil || !strings.Contains(err.Error(), "not owner") {
		t.Fatalf("approve err = %v, want not owner", err)
	}
	requireSpaceLinkEvents(t, events, "owner")
}

func TestApproveVerifiedSpaceLinkStopsOnReplayBeforeRegistration(t *testing.T) {
	now := time.Unix(100, 0)
	ticketBytes, _ := buildTestSpaceLinkTicket(t, now, nil)
	verified, err := verifySpaceLinkTicketData(ticketBytes, now)
	if err != nil {
		t.Fatalf("verify ticket: %v", err)
	}
	var events []string
	nonce := &testSpaceLinkNonceConsumer{
		events: &events,
		err:    provider_spacewave.ErrSpaceLinkNonceConsumed,
	}
	registrar := &testSpaceLinkRegistrar{
		events: &events,
		resp:   &api.RegisterSessionResponse{AccountId: "acct-1", Created: true},
	}
	target := &testSpaceLinkTarget{events: &events}

	_, err = approveVerifiedSpaceLink(
		context.Background(),
		verified,
		[]byte("space-1"),
		"approver-peer",
		nonce,
		registrar,
		target,
	)
	if !errors.Is(err, provider_spacewave.ErrSpaceLinkNonceConsumed) {
		t.Fatalf("approve err = %v, want ErrSpaceLinkNonceConsumed", err)
	}
	requireSpaceLinkEvents(t, events, "owner", "consume")
}

func TestApproveVerifiedSpaceLinkRegistrationFailureDoesNotGrantOrRollback(t *testing.T) {
	now := time.Unix(100, 0)
	ticketBytes, _ := buildTestSpaceLinkTicket(t, now, nil)
	verified, err := verifySpaceLinkTicketData(ticketBytes, now)
	if err != nil {
		t.Fatalf("verify ticket: %v", err)
	}
	var events []string
	nonce := &testSpaceLinkNonceConsumer{events: &events}
	registrar := &testSpaceLinkRegistrar{
		events: &events,
		err:    errors.New("registration failed"),
	}
	target := &testSpaceLinkTarget{events: &events}

	_, err = approveVerifiedSpaceLink(
		context.Background(),
		verified,
		[]byte("space-1"),
		"approver-peer",
		nonce,
		registrar,
		target,
	)
	if err == nil || !strings.Contains(err.Error(), "registration failed") {
		t.Fatalf("approve err = %v, want registration failed", err)
	}
	requireSpaceLinkEvents(t, events, "owner", "consume", "register")
	if len(registrar.rollbackPeerIDs) != 0 {
		t.Fatalf("rollback called for registration failure: %v", registrar.rollbackPeerIDs)
	}
}

func TestApproveVerifiedSpaceLinkRollsBackOnlyNewRowsOnGrantFailure(t *testing.T) {
	now := time.Unix(100, 0)
	ticketBytes, _ := buildTestSpaceLinkTicket(t, now, nil)
	verified, err := verifySpaceLinkTicketData(ticketBytes, now)
	if err != nil {
		t.Fatalf("verify ticket: %v", err)
	}
	tests := []struct {
		name         string
		created      bool
		wantRollback bool
		wantEvents   []string
	}{
		{
			name:         "created row",
			created:      true,
			wantRollback: true,
			wantEvents:   []string{"owner", "consume", "register", "add", "rollback"},
		},
		{
			name:       "reused row",
			created:    false,
			wantEvents: []string{"owner", "consume", "register", "add"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var events []string
			nonce := &testSpaceLinkNonceConsumer{events: &events}
			registrar := &testSpaceLinkRegistrar{
				events: &events,
				resp: &api.RegisterSessionResponse{
					AccountId: "acct-1",
					Created:   tt.created,
				},
			}
			target := &testSpaceLinkTarget{
				events: &events,
				addErr: errors.New("grant failed"),
			}

			_, err = approveVerifiedSpaceLink(
				context.Background(),
				verified,
				[]byte("space-1"),
				"approver-peer",
				nonce,
				registrar,
				target,
			)
			if err == nil || !strings.Contains(err.Error(), "grant failed") {
				t.Fatalf("approve err = %v, want grant failed", err)
			}
			requireSpaceLinkEvents(t, events, tt.wantEvents...)
			if got := len(registrar.rollbackPeerIDs) != 0; got != tt.wantRollback {
				t.Fatalf("rollback called = %v, want %v", got, tt.wantRollback)
			}
			if tt.wantRollback && registrar.rollbackPeerIDs[0] != verified.agentPeerID.String() {
				t.Fatalf("rollback peer = %q, want %q", registrar.rollbackPeerIDs[0], verified.agentPeerID.String())
			}
		})
	}
}

func buildTestSpaceLinkTicket(
	t *testing.T,
	now time.Time,
	mutate func(*s4wave_provider_spacewave.SpaceLinkAuthRequest),
) ([]byte, *s4wave_provider_spacewave.SpaceLinkAuthRequest) {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("peer id: %v", err)
	}
	payload := &s4wave_provider_spacewave.SpaceLinkAuthRequest{
		Version:        spaceLinkAuthRequestVersion,
		SessionType:    session.SessionType_SESSION_TYPE_DEVICE,
		AgentPeerId:    []byte(pid),
		Label:          "test device",
		CallbackUrl:    "http://127.0.0.1:9000/callback",
		RequestedRole:  sobject.SOParticipantRole_SOParticipantRole_WRITER,
		Nonce:          []byte("1234567890abcdef"),
		ExpiresAt:      now.Add(time.Minute).Unix(),
		CompletionMode: s4wave_provider_spacewave.SpaceLinkCompletionMode_SpaceLinkCompletionMode_BROWSER_CALLBACK,
	}
	if mutate != nil {
		mutate(payload)
	}
	payloadBytes, err := payload.MarshalVT()
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	sig, err := priv.Sign(payloadBytes)
	if err != nil {
		t.Fatalf("sign payload: %v", err)
	}
	ticket := &s4wave_provider_spacewave.SpaceLinkAuthTicket{
		Payload:        payloadBytes,
		AgentSignature: sig,
	}
	ticketBytes, err := ticket.MarshalVT()
	if err != nil {
		t.Fatalf("marshal ticket: %v", err)
	}
	return ticketBytes, payload
}

type testSpaceLinkNonceConsumer struct {
	events *[]string
	err    error
}

func (n *testSpaceLinkNonceConsumer) ConsumeSpaceLinkNonce(
	_ context.Context,
	_, _, _ []byte,
	_ time.Time,
) error {
	appendSpaceLinkEvent(n.events, "consume")
	return n.err
}

type testSpaceLinkRegistrar struct {
	events          *[]string
	resp            *api.RegisterSessionResponse
	err             error
	rollbackErr     error
	req             *api.RegisterSessionRequest
	rollbackPeerIDs []string
}

func (r *testSpaceLinkRegistrar) RegisterSessionWithRequest(
	_ context.Context,
	req *api.RegisterSessionRequest,
	_ string,
) (*api.RegisterSessionResponse, error) {
	appendSpaceLinkEvent(r.events, "register")
	r.req = req
	return r.resp, r.err
}

func (r *testSpaceLinkRegistrar) RollbackSessionRegistration(_ context.Context, sessionPeerID string) error {
	appendSpaceLinkEvent(r.events, "rollback")
	r.rollbackPeerIDs = append(r.rollbackPeerIDs, sessionPeerID)
	return r.rollbackErr
}

type testSpaceLinkTarget struct {
	events       *[]string
	ownerErr     error
	addErr       error
	ownerPeerID  string
	addPeerID    string
	addPub       crypto.PubKey
	addRole      sobject.SOParticipantRole
	addAccountID string
}

func (t *testSpaceLinkTarget) requireApproverOwner(_ context.Context, approverPeerID string) error {
	appendSpaceLinkEvent(t.events, "owner")
	t.ownerPeerID = approverPeerID
	return t.ownerErr
}

func (t *testSpaceLinkTarget) addParticipant(
	_ context.Context,
	peerID string,
	pub crypto.PubKey,
	role sobject.SOParticipantRole,
	accountID string,
) error {
	appendSpaceLinkEvent(t.events, "add")
	t.addPeerID = peerID
	t.addPub = pub
	t.addRole = role
	t.addAccountID = accountID
	return t.addErr
}

func appendSpaceLinkEvent(events *[]string, event string) {
	if events != nil {
		*events = append(*events, event)
	}
}

func requireSpaceLinkEvents(t *testing.T, events []string, want ...string) {
	t.Helper()
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("events = %v, want %v", events, want)
	}
}
