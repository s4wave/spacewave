package provider_spacewave

import (
	"context"
	"testing"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
)

// queueAndApplySONotify records payload as the websocket reader does, then
// runs the host's state routine once.
func queueAndApplySONotify(h *cloudSOHost, payload *api.SONotifyEventPayload) {
	h.soNotify(payload)
	h.applyStateBacklog(context.Background())
}

// testDeltaMessage returns a delta from since holding entries since+1..to.
func testDeltaMessage(since, to uint64, changeBytes int) *api.SOStateMessage {
	delta := &api.SOStateDelta{Since: since}
	for seqno := since + 1; seqno <= to; seqno++ {
		delta.Entries = append(delta.Entries, &api.SOStateDeltaEntry{
			Seqno:      seqno,
			ChangeType: "op",
			ChangeData: make([]byte, changeBytes),
		})
	}
	return &api.SOStateMessage{Seqno: to, Content: &api.SOStateMessage_Delta{Delta: delta}}
}

// testRunSeqnos returns the seqnos of a batch's run.
func testRunSeqnos(batch soStateBatch) []uint64 {
	var out []uint64
	for _, entry := range batch.run.GetDelta().GetEntries() {
		out = append(out, entry.GetSeqno())
	}
	return out
}

// TestSOStateBacklogCollapses collapses a burst into a snapshot and one
// contiguous run, and turns a gap, an overflow or a reconnect into a resync
// that drops the held messages but keeps the highest nonce.
func TestSOStateBacklogCollapses(t *testing.T) {
	// A snapshot, a contiguous delta and an overlapping resend form one run.
	var b soStateBacklog
	b.push(testDeltaMessage(1, 2, 1), 3)
	b.push(&api.SOStateMessage{Seqno: 5, Content: &api.SOStateMessage_Snapshot{Snapshot: &sobject.SOState{}}}, 7)
	b.push(testDeltaMessage(5, 6, 1), 0)
	b.push(testDeltaMessage(4, 7, 1), 9)
	batch := b.take()
	if batch.resync != "" || batch.snapshot.GetSeqno() != 5 || batch.nonce != 9 {
		t.Fatalf("unexpected batch: resync=%q snapshot=%d nonce=%d", batch.resync, batch.snapshot.GetSeqno(), batch.nonce)
	}
	if got := testRunSeqnos(batch); len(got) != 2 || got[0] != 6 || got[1] != 7 || batch.run.GetDelta().GetSince() != 5 {
		t.Fatalf("run = since %d entries %v, want since 5 entries [6 7]", batch.run.GetDelta().GetSince(), got)
	}
	if again := b.take(); again.snapshot != nil || again.run != nil || again.resync != "" || again.nonce != 0 {
		t.Fatal("take did not empty the backlog")
	}

	// A gap drops the held run. Later messages only raise the nonce.
	b.push(testDeltaMessage(7, 8, 1), 0)
	b.push(testDeltaMessage(9, 10, 1), 0)
	b.push(testDeltaMessage(10, 11, 1), 12)
	batch = b.take()
	if batch.resync != SeedReasonGapRecovery || batch.run != nil || batch.snapshot != nil || batch.nonce != 12 {
		t.Fatalf("gap batch: resync=%q run=%v nonce=%d", batch.resync, batch.run != nil, batch.nonce)
	}

	// A run past the byte bound resyncs.
	b.push(testDeltaMessage(11, 12, soStateBacklogMaxBytes/2), 0)
	b.push(testDeltaMessage(12, 13, soStateBacklogMaxBytes/2+1), 0)
	if batch = b.take(); batch.resync != SeedReasonGapRecovery || batch.run != nil {
		t.Fatalf("overflow batch: resync=%q run=%v", batch.resync, batch.run != nil)
	}

	// A new connection drops what the old one delivered.
	b.push(testDeltaMessage(13, 14, 1), 0)
	b.markResync(SeedReasonReconnect)
	if batch = b.take(); batch.resync != SeedReasonReconnect || batch.run != nil {
		t.Fatalf("reconnect batch: resync=%q run=%v", batch.resync, batch.run != nil)
	}
}
