package provider_spacewave

import (
	"context"
	"errors"
	"testing"
	"time"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

func TestWaitMailboxRequestDecisionReturnsAcceptedAfterMailboxEvent(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")
	acc.TrackMailboxRequest("so-1", "inv-1", "peer-1", "pending")

	done := make(chan struct{})
	var (
		got string
		err error
	)
	go func() {
		defer close(done)
		got, err = acc.WaitMailboxRequestDecision(
			context.Background(),
			"so-1",
			"inv-1",
			"peer-1",
		)
	}()

	acc.ApplyMailboxEntryEvent("so-1", &api.MailboxEntry{
		Id:       1,
		InviteId: "inv-1",
		PeerId:   "peer-1",
		Status:   "accepted",
	}, 1)

	<-done
	if err != nil {
		t.Fatalf("wait mailbox request decision: %v", err)
	}
	if got != "accepted" {
		t.Fatalf("expected accepted status, got %q", got)
	}
}

func TestWaitMailboxRequestDecisionPendingUntilLaterAcceptedEvent(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")
	acc.TrackMailboxRequest("so-1", "inv-1", "peer-1", "pending")

	waitCtx, waitCancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer waitCancel()

	_, err := acc.WaitMailboxRequestDecision(waitCtx, "so-1", "inv-1", "peer-1")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected pending wait to time out, got %v", err)
	}

	acc.ApplyMailboxEntryEvent("so-1", &api.MailboxEntry{
		Id:       1,
		InviteId: "inv-1",
		PeerId:   "peer-1",
		Status:   "accepted",
	}, 1)

	got, err := acc.WaitMailboxRequestDecision(
		context.Background(),
		"so-1",
		"inv-1",
		"peer-1",
	)
	if err != nil {
		t.Fatalf("wait mailbox request decision after accepted event: %v", err)
	}
	if got != "accepted" {
		t.Fatalf("expected accepted status after later event, got %q", got)
	}
}

func TestWaitMailboxRequestDecisionReturnsRejectedAfterMailboxEvent(t *testing.T) {
	acc := NewTestProviderAccount(t, "http://example.invalid")
	acc.TrackMailboxRequest("so-1", "inv-1", "peer-1", "pending")

	acc.ApplyMailboxEntryEvent("so-1", &api.MailboxEntry{
		Id:       1,
		InviteId: "inv-1",
		PeerId:   "peer-1",
		Status:   "rejected",
	}, 1)

	got, err := acc.WaitMailboxRequestDecision(
		context.Background(),
		"so-1",
		"inv-1",
		"peer-1",
	)
	if err != nil {
		t.Fatalf("wait mailbox request decision after rejected event: %v", err)
	}
	if got != "rejected" {
		t.Fatalf("expected rejected status, got %q", got)
	}
}
