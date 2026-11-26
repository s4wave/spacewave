package provider_spacewave

import (
	"sync"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// soStateBacklogMaxBytes bounds the delta change data one Space's backlog
// holds. A longer run is dropped for a resync pull, which reads it back.
const soStateBacklogMaxBytes = 4 << 20

// soStateBatch is the inline state one Space has received and not yet applied.
// A resync batch holds no messages: its pull covers them.
type soStateBatch struct {
	// nonce is the highest block store nonce the received messages carried.
	nonce uint64
	// resync, when set, names why the batch pulls a fresh state in place of
	// its messages.
	resync SeedReason
	// snapshot is the newest snapshot message, or nil.
	snapshot *api.SOStateMessage
	// run is one delta whose entries are contiguous from its since, continuing
	// from snapshot when one is held, or nil.
	run *api.SOStateMessage
}

// soStateBacklog collapses the inline state the Session websocket delivers to
// one Space until the Space's worker takes it. The reader pushes in constant
// time per entry, so a slow Space delays only its own worker.
//
// A snapshot replaces the held messages. A delta contiguous with the held end
// extends the run. A gap, a run past soStateBacklogMaxBytes, or a new
// connection drops the held messages and marks a resync: the worker's pull
// reads the cloud after it takes the batch, so it covers every message
// dropped before the take. The SpaceBacklog TLA+ spec models these states.
type soStateBacklog struct {
	mtx      sync.Mutex
	held     soStateBatch
	runBytes int
}

// push records an inline snapshot or delta message and its block store nonce.
func (b *soStateBacklog) push(msg *api.SOStateMessage, nonce uint64) {
	// Hold the backlog against the worker's take.
	b.mtx.Lock()
	defer b.mtx.Unlock()

	// Raise the nonce, then drop the message when a resync will read it back.
	b.held.nonce = max(b.held.nonce, nonce)
	if b.held.resync != "" {
		return
	}

	// A snapshot replaces everything held; a delta extends the run.
	switch {
	case msg.GetSnapshot() != nil:
		b.held.snapshot, b.held.run, b.runBytes = msg, nil, 0
	case msg.GetDelta() != nil:
		b.pushDeltaLocked(msg)
	}
}

// pushDeltaLocked appends the delta's entries past the held end, or marks a
// resync when the delta leaves a gap or the run grows past its bound. The
// caller holds mtx.
func (b *soStateBacklog) pushDeltaLocked(msg *api.SOStateMessage) {
	// Hold the first delta whole.
	delta := msg.GetDelta()
	if len(delta.GetEntries()) == 0 {
		return
	}
	end, ok := b.heldEndLocked()
	if !ok {
		b.held.run = msg
		b.addRunBytesLocked(delta.GetEntries())
		return
	}

	// A delta past the held end leaves a gap only a pull can fill.
	if delta.GetSince() > end {
		b.resyncLocked(SeedReasonGapRecovery)
		return
	}

	// Append the entries past the held end, which must continue it, starting
	// the run at the held snapshot when only a snapshot is held.
	var added []*api.SOStateDeltaEntry
	for _, entry := range delta.GetEntries() {
		if entry.GetSeqno() <= end {
			continue
		}
		if entry.GetSeqno() != end+1 {
			b.resyncLocked(SeedReasonGapRecovery)
			return
		}
		added = append(added, entry)
		end++
	}
	if len(added) == 0 {
		return
	}
	if b.held.run == nil {
		b.held.run = &api.SOStateMessage{
			Content: &api.SOStateMessage_Delta{Delta: &api.SOStateDelta{Since: end - uint64(len(added))}},
		}
	}
	run := b.held.run.GetDelta()
	run.Entries = append(run.Entries, added...)
	b.held.run.Seqno = max(b.held.run.GetSeqno(), msg.GetSeqno())
	b.addRunBytesLocked(added)
}

// addRunBytesLocked counts entries added to the run, marking a resync when the
// run passes its bound. The caller holds mtx.
func (b *soStateBacklog) addRunBytesLocked(entries []*api.SOStateDeltaEntry) {
	for _, entry := range entries {
		b.runBytes += len(entry.GetChangeData())
	}
	if b.runBytes > soStateBacklogMaxBytes {
		b.resyncLocked(SeedReasonGapRecovery)
	}
}

// heldEndLocked returns the seqno the held messages reach. The caller holds
// mtx.
func (b *soStateBacklog) heldEndLocked() (uint64, bool) {
	if entries := b.held.run.GetDelta().GetEntries(); len(entries) != 0 {
		return entries[len(entries)-1].GetSeqno(), true
	}
	if b.held.snapshot != nil {
		return b.held.snapshot.GetSeqno(), true
	}
	return 0, false
}

// markResync drops the held messages for a pull made after the next take.
func (b *soStateBacklog) markResync(reason SeedReason) {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	b.resyncLocked(reason)
}

// resyncLocked drops the held messages and keeps the nonce for the pull. The
// caller holds mtx.
func (b *soStateBacklog) resyncLocked(reason SeedReason) {
	b.held = soStateBatch{nonce: b.held.nonce, resync: reason}
	b.runBytes = 0
}

// take returns the held batch and leaves the backlog empty.
func (b *soStateBacklog) take() soStateBatch {
	// Swap the held batch for an empty one.
	b.mtx.Lock()
	defer b.mtx.Unlock()
	batch := b.held
	b.held, b.runBytes = soStateBatch{}, 0
	return batch
}
