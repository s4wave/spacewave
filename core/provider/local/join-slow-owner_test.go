package provider_local_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_invite "github.com/s4wave/spacewave/core/sobject/invite"
	"github.com/sirupsen/logrus"
)

// TestJoinViaInviteSlowOwner checks that a join outlasts the link hold-open
// time and the owner-online wait while the owner is still answering.
func TestJoinViaInviteSlowOwner(t *testing.T) {
	// Bound the test's setup, slow owner response, and join.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	// Start the owner and requester's local transports.
	h := newJoinRequestHarness(ctx, t)

	// Create the space whose owner will answer slowly.
	space := h.createSpace("slow-owner-space")

	// Retain the owner transport for the test's invite service.
	ownerTransport := h.owner.GetSessionTransport()

	// Expire the invite so session startup does not register a competing
	// production handler; this test's slow lookup answers before invite checks.
	msg, err := space.CreateSOInviteOp(
		ctx,
		space.GetPrivKey(),
		"local",
		&sobject.SOInvite{
			Role:      sobject.SOParticipantRole_SOParticipantRole_WRITER,
			MaxUses:   1,
			ExpiresAt: timestamppb.New(time.Now().Add(-time.Minute)),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Route the signed invite to the owner transport.
	msg.TransportPeerId = ownerTransport.GetPeerID().String()
	if err := msg.Sign(space.GetPrivKey()); err != nil {
		t.Fatal(err)
	}

	// Name the error returned after the owner stalls.
	errStalled := errors.New("owner lookup stalled")

	// Install a lookup handler that outlasts both owner-link bounds.
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

	// Serve the slow lookup on the owner transport.
	relCtrl, err := ownerTransport.GetChildBus().AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relCtrl()

	// Require the owner's answer, not a closed link or a join timeout, to end the join.
	_, err = h.reader.JoinViaInvite(ctx, h.readerKey.GetPrivKey(), msg, "")
	if err == nil || !strings.Contains(err.Error(), errStalled.Error()) {
		t.Fatalf("expected the owner's lookup error, got %v", err)
	}
}
