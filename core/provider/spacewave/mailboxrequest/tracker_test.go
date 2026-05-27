package mailboxrequest

import "testing"

func TestDecisionReturnsAcceptedStatus(t *testing.T) {
	var tracker Tracker
	if !tracker.Track("so-1", "inv-1", "peer-1", "pending") {
		t.Fatal("expected initial pending status to change tracker")
	}
	if changed := tracker.Track("so-1", "inv-1", "peer-1", "pending"); changed {
		t.Fatal("expected duplicate pending status to be unchanged")
	}
	if !tracker.Track("so-1", "inv-1", "peer-1", "accepted") {
		t.Fatal("expected accepted status to change tracker")
	}

	got, ok := tracker.Decision("so-1", "inv-1", "peer-1")
	if !ok {
		t.Fatal("expected terminal decision")
	}
	if got != "accepted" {
		t.Fatalf("decision = %q, want accepted", got)
	}
}

func TestDecisionPendingUntilTerminalStatus(t *testing.T) {
	var tracker Tracker
	tracker.Track("so-1", "inv-1", "peer-1", "pending")

	if got, ok := tracker.Decision("so-1", "inv-1", "peer-1"); ok {
		t.Fatalf("pending decision = %q, want not ready", got)
	}

	tracker.Track("so-1", "inv-1", "peer-1", "rejected")
	got, ok := tracker.Decision("so-1", "inv-1", "peer-1")
	if !ok {
		t.Fatal("expected rejected terminal decision")
	}
	if got != "rejected" {
		t.Fatalf("decision = %q, want rejected", got)
	}
}

func TestTrackRejectsIncompleteKey(t *testing.T) {
	var tracker Tracker
	if tracker.Track("", "inv-1", "peer-1", "accepted") {
		t.Fatal("expected empty shared object id to be ignored")
	}
	if got, ok := tracker.Decision("", "inv-1", "peer-1"); ok {
		t.Fatalf("empty-key decision = %q, want not ready", got)
	}
}
