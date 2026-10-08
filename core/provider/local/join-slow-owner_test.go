package provider_local_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	sobject_invite "github.com/s4wave/spacewave/core/sobject/invite"
	"github.com/sirupsen/logrus"
)

// TestJoinViaInviteSlowOwner checks that a join outlasts the link hold-open
// time and the owner-online wait while the owner is still answering.
func TestJoinViaInviteSlowOwner(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	h := newJoinRequestHarness(ctx, t)
	space := h.createSpace("slow-owner-space")

	// Sign an invite routed to the owner's transport, which serves a Lookup
	// that outlasts both bounds before it fails.
	ownerTransport := h.owner.GetSessionTransport()
	msg, err := space.CreateSOInviteOp(ctx, space.GetPrivKey(), "local", &sobject.SOInvite{Role: sobject.SOParticipantRole_SOParticipantRole_WRITER, MaxUses: 1})
	if err != nil {
		t.Fatal(err)
	}
	msg.TransportPeerId = ownerTransport.GetPeerID().String()
	if err := msg.Sign(space.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	errStalled := errors.New("owner lookup stalled")
	ctrl, err := sobject_invite.NewInviteController(
		logrus.NewEntry(logrus.New()),
		ownerTransport.GetChildBus(),
		sobject_invite.Handlers{
			Lookup: func(ctx context.Context, _ []byte) (*sobject_invite.InviteLookupResult, error) {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(12 * time.Second):
					return nil, errStalled
				}
			},
		},
		[]string{ownerTransport.GetPeerID().String()},
	)
	if err != nil {
		t.Fatal(err)
	}
	relCtrl, err := ownerTransport.GetChildBus().AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relCtrl()

	// The owner's answer, not a closed link or a join timeout, ends the join.
	_, err = h.reader.JoinViaInvite(ctx, h.readerKey.GetPrivKey(), msg, "")
	if err == nil || !strings.Contains(err.Error(), errStalled.Error()) {
		t.Fatalf("expected the owner's lookup error, got %v", err)
	}
}
