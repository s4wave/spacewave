package provider_local_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
)

// joinRequestHarness holds an owner and a requester joined over a native transport.
type joinRequestHarness struct {
	ctx       context.Context
	t         *testing.T
	owner     *provider_local.ProviderAccount
	ownerKey  *provider_local.Session
	reader    *provider_local.ProviderAccount
	readerKey *provider_local.Session
}

// newJoinRequestHarness starts two providers with reachable session transports.
func newJoinRequestHarness(ctx context.Context, t *testing.T) *joinRequestHarness {
	// Mount the owner and reader accounts on separate providers.
	skipFullP2PSyncUnderGoScript(t)
	_, _, owner, ownerSession, releaseOwner := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseOwner)
	_, _, reader, readerSession, releaseReader := setupProviderAndSession(ctx, t)
	t.Cleanup(releaseReader)

	// Start both session transports and connect their backends.
	if err := owner.EnsureConfiguredSessionTransport(ctx, ownerSession.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(owner.StopSessionTransport)
	t.Cleanup(owner.StopP2PSync)
	if err := reader.EnsureConfiguredSessionTransport(ctx, readerSession.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reader.StopSessionTransport)
	t.Cleanup(reader.StopP2PSync)
	prepareSessionTransportBackends(ctx, t, owner.GetSessionTransport(), reader.GetSessionTransport())

	return &joinRequestHarness{ctx: ctx, t: t, owner: owner, ownerKey: ownerSession, reader: reader, readerKey: readerSession}
}

// createSpace creates and mounts an owner Space.
func (h *joinRequestHarness) createSpace(name string) *provider_local.SharedObject {
	// Create the Space and mount it for the test's lifetime.
	ref, err := h.owner.CreateSharedObject(h.ctx, name, &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		h.t.Fatal(err)
	}
	mounted, release, err := h.owner.MountSharedObject(h.ctx, ref, nil)
	if err != nil {
		h.t.Fatal(err)
	}
	h.t.Cleanup(release)
	return mounted.(*provider_local.SharedObject)
}

// invite adds a direct invite with terms to space.
func (h *joinRequestHarness) invite(space *provider_local.SharedObject, terms *sobject.SOInvite) *sobject.SOInviteMessage {
	// Sign the invite and publish it on the owner's session transport.
	terms.Role = sobject.SOParticipantRole_SOParticipantRole_WRITER
	msg, err := space.CreateSOInviteOp(h.ctx, space.GetPrivKey(), "local", terms)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := h.owner.PrepareDirectInvite(h.ctx, h.ownerKey.GetPrivKey(), space.GetPrivKey(), msg); err != nil {
		h.t.Fatal(err)
	}
	return msg
}

// redeem redeems msg as the reader and reports whether it was queued.
func (h *joinRequestHarness) redeem(msg *sobject.SOInviteMessage) bool {
	h.t.Helper()
	result, err := h.reader.JoinViaInvite(h.ctx, h.readerKey.GetPrivKey(), msg, "")
	if err != nil {
		h.t.Fatal(err)
	}
	return result.Pending
}

// refused checks that the owner refuses the reader's redemption of msg.
func (h *joinRequestHarness) refused(msg *sobject.SOInviteMessage) {
	h.t.Helper()
	_, err := h.reader.JoinViaInvite(h.ctx, h.readerKey.GetPrivKey(), msg, "")
	if err == nil || !strings.Contains(err.Error(), "invite does not admit this peer") {
		h.t.Fatalf("expected refusal, got %v", err)
	}
}

// participates reports whether the reader participates in space.
func (h *joinRequestHarness) participates(space *provider_local.SharedObject) bool {
	state, err := space.GetSOHostState(h.ctx)
	if err != nil {
		h.t.Fatal(err)
	}
	return slices.ContainsFunc(state.GetConfig().GetParticipants(), func(p *sobject.SOParticipantConfig) bool {
		return p.GetPeerId() == h.readerKey.GetPeerId().String()
	})
}

// requesters returns the peers with pending join requests on space.
func requesters(space *provider_local.SharedObject) []string {
	var peers []string
	for _, req := range space.GetJoinRequestsCtr().GetValue().GetRequests() {
		peers = append(peers, req.GetJoinResponse().GetResponderPeerId())
	}
	return peers
}

// TestJoinRequestApproval queues a redemption, refuses it, lets the requester
// withdraw a repeat, and grants another by personal invite.
func TestJoinRequestApproval(t *testing.T) {
	// Offer a Space by an invite that requires approval.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	h := newJoinRequestHarness(ctx, t)
	space := h.createSpace("knock-space")
	knock := h.invite(space, &sobject.SOInvite{ApprovalRequired: true})
	readerPeer := h.readerKey.GetPeerId().String()

	// The redemption waits for the owner without admitting the reader.
	if !h.redeem(knock) {
		t.Fatal("approval invite admitted the reader")
	}
	if h.participates(space) {
		t.Fatal("queued reader became a participant")
	}
	if got := requesters(space); !slices.Equal(got, []string{readerPeer}) {
		t.Fatalf("owner holds join requests %v", got)
	}

	// A refusal clears the request and admits nobody.
	if err := space.RemoveJoinRequest(ctx, readerPeer); err != nil {
		t.Fatal(err)
	}
	if len(requesters(space)) != 0 || h.participates(space) {
		t.Fatal("refusal left a request or a participant")
	}

	// The requester withdraws a request it no longer wants. Withdrawing again
	// finds nothing left and still succeeds.
	if !h.redeem(knock) {
		t.Fatal("request to withdraw was not queued")
	}
	for range 2 {
		if err := h.reader.WithdrawJoinRequest(ctx, h.readerKey.GetPrivKey(), knock); err != nil {
			t.Fatal(err)
		}
	}
	if len(requesters(space)) != 0 || h.participates(space) {
		t.Fatal("withdrawal left a request or a participant")
	}

	// Asking again makes a new request. Revoking the invite it redeemed, as a
	// change of admission terms does, keeps the role a grant reads.
	if !h.redeem(knock) {
		t.Fatal("repeat request was not queued")
	}
	if err := space.RevokeInvite(ctx, space.GetPrivKey(), knock.GetInviteId()); err != nil {
		t.Fatal(err)
	}
	state, err := space.GetSOHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if role := sobject.FindInvite(state, knock.GetInviteId()).GetRole(); role != sobject.SOParticipantRole_SOParticipantRole_WRITER {
		t.Fatalf("revoked invite grants role %v", role)
	}

	// The owner grants the request by personal invite.
	personal := h.invite(space, &sobject.SOInvite{TargetPeerId: readerPeer, MaxUses: 1})
	if err := space.RemoveJoinRequest(ctx, readerPeer); err != nil {
		t.Fatal(err)
	}
	if h.redeem(personal) {
		t.Fatal("personal invite was queued")
	}
	if !h.participates(space) {
		t.Fatal("granted reader is not a participant")
	}
	mountJoined(ctx, t, h.reader, space.GetSharedObjectID())
}

// TestJoinParticipantOf admits participants of a named Space and queues others when approval is allowed.
func TestJoinParticipantOf(t *testing.T) {
	// Restrict invites to participants of a parent Space.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	h := newJoinRequestHarness(ctx, t)
	parent := h.createSpace("parent")
	parentID := parent.GetSharedObjectID()
	restricted := &sobject.SOInviteParticipation{SharedObjectIds: []string{parentID}}

	// A stranger to the parent is refused; an empty list admits nobody.
	child := h.createSpace("child")
	childInvite := h.invite(child, &sobject.SOInvite{ParticipantOf: restricted})
	h.refused(childInvite)
	h.refused(h.invite(child, &sobject.SOInvite{ParticipantOf: &sobject.SOInviteParticipation{}}))

	// A Space the owner does not hold admits nobody.
	h.refused(h.invite(child, &sobject.SOInvite{
		ParticipantOf: &sobject.SOInviteParticipation{SharedObjectIds: []string{"unheld"}},
	}))

	// A participant of the parent is admitted.
	if h.redeem(h.invite(parent, &sobject.SOInvite{})) {
		t.Fatal("open invite was queued")
	}
	if h.redeem(childInvite) || !h.participates(child) {
		t.Fatal("parent participant was not admitted")
	}

	// A combined invite admits the participant at once.
	combined := h.createSpace("combined")
	if h.redeem(h.invite(combined, &sobject.SOInvite{ParticipantOf: restricted, ApprovalRequired: true})) {
		t.Fatal("combined invite queued a parent participant")
	}
	if !h.participates(combined) {
		t.Fatal("combined invite did not admit a parent participant")
	}

	// After leaving the parent, the reader is refused, or queued under approval.
	if err := h.reader.LeaveSharedObject(ctx, h.readerKey.GetPrivKey(), parentID, ""); err != nil {
		t.Fatal(err)
	}
	later := h.createSpace("later")
	h.refused(h.invite(later, &sobject.SOInvite{ParticipantOf: restricted}))
	if !h.redeem(h.invite(later, &sobject.SOInvite{ParticipantOf: restricted, ApprovalRequired: true})) {
		t.Fatal("combined invite admitted a former participant")
	}
	if h.participates(later) || len(requesters(later)) != 1 {
		t.Fatal("former participant was not queued")
	}
}
