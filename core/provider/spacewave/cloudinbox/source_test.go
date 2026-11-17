package cloudinbox

import (
	"bytes"
	"testing"

	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// TestBatches checks that a page becomes bounded batches: the gap leads once,
// events at or below the cursor are skipped, each batch's cursor is its last
// seqno, kinds pick ports, and an oversized body is dropped from its payload.
func TestBatches(t *testing.T) {
	// A page after cursor 3 whose gap reaches 5, with one stale event.
	big := bytes.Repeat([]byte{'x'}, s4wave_flowgraph.MaxEventPayload)
	body := bytes.Repeat([]byte{'y'}, maxBatchPayload/4-4<<10)
	page := &ListInboxEventsResponse{
		GapThrough: 5,
		LastSeqno:  11,
		Events: []*InboxEvent{
			{Seqno: 5, Kind: "check_run.completed"},
			{Seqno: 6, Kind: "check_suite.completed", Subject: "o/r@a", Payload: body},
			{Seqno: 7, Kind: "workflow_run.completed", Payload: body},
			{Seqno: 8, Kind: "pull_request.opened", Payload: body},
			{Seqno: 9, Kind: "check_run", Payload: body},
			{Seqno: 10, Kind: "check_run.completed", Payload: body},
			{Seqno: 11, Kind: "check_run.completed", Payload: big},
		},
	}
	batches := Batches(page, 3)

	// The gap and four bodies fill the first batch; the fifth overflows it.
	type want struct {
		ids    []string
		ports  []string
		cursor string
	}
	wants := []want{
		{[]string{"gap/5", "6", "7", "8", "9"}, []string{PortGap, PortCheckSuite, PortWorkflowRun, PortOther, PortCheckRun}, "9"},
		{[]string{"10", "11"}, []string{PortCheckRun, PortCheckRun}, "11"},
	}
	if len(batches) != len(wants) {
		t.Fatalf("got %d batches, want %d", len(batches), len(wants))
	}
	for i, w := range wants {
		b := batches[i]
		if got := string(b.GetNextCursor()); got != w.cursor {
			t.Fatalf("batch %d cursor %q, want %q", i, got, w.cursor)
		}
		if len(b.GetEvents()) != len(w.ids) {
			t.Fatalf("batch %d has %d events, want %d", i, len(b.GetEvents()), len(w.ids))
		}
		for j, ev := range b.GetEvents() {
			if ev.GetEventId() != w.ids[j] || ev.GetPort() != w.ports[j] {
				t.Fatalf("batch %d event %d = %s on %s, want %s on %s", i, j, ev.GetEventId(), ev.GetPort(), w.ids[j], w.ports[j])
			}
		}
	}

	// The gap reports the cursor it skipped from; the oversized body is gone.
	gap := &InboxGap{}
	if err := gap.UnmarshalVT(batches[0].GetEvents()[0].GetPayload()); err != nil || gap.GetGapThrough() != 5 || gap.GetCursor() != 3 {
		t.Fatalf("gap payload %v, %v", gap, err)
	}
	last := &InboxEvent{}
	if err := last.UnmarshalVT(batches[1].GetEvents()[1].GetPayload()); err != nil || len(last.GetPayload()) != 0 || last.GetSeqno() != 11 {
		t.Fatalf("oversized event kept its body or lost its seqno: %v", err)
	}

	// A cursor past the gap yields no gap event.
	if again := Batches(page, 11); len(again) != 0 {
		t.Fatalf("cursor at the end yielded %d batches", len(again))
	}
}
