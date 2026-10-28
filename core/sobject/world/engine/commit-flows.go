package sobject_world_engine

import (
	"context"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/block"
)

// commitKind is a kind of record a World commit's publication stores.
type commitKind int

const (
	// kindWorld is the pages of the World the commit produced.
	kindWorld commitKind = iota
	// kindPayload is the blocks the commit's operation carries.
	kindPayload
	// kindReplay is the saved replay: its span blocks and its cursor.
	kindReplay
	// kindState is the SharedObject state the commit rewrites.
	kindState
	// numCommitKinds is the number of kinds.
	numCommitKinds
)

// commitKindNames are the names the flows give each kind.
var commitKindNames = [numCommitKinds]string{"world", "payload", "replay", "state"}

// checkpointOutcome is how a prepared stable checkpoint ended.
type checkpointOutcome int

const (
	// outcomeWritten is a checkpoint adopted into the state.
	outcomeWritten checkpointOutcome = iota
	// outcomeSuperseded is a checkpoint a later one replaced before the
	// checkpoint routine took it.
	outcomeSuperseded
	// outcomeStale is a checkpoint whose stable point the state moved past.
	outcomeStale
	// outcomeFailed is a checkpoint whose adoption failed.
	outcomeFailed
	// numCheckpointOutcomes is the number of outcomes.
	numCheckpointOutcomes
)

// checkpointOutcomeNames are the names the flows give each outcome.
var checkpointOutcomeNames = [numCheckpointOutcomes]string{"written", "superseded", "stale", "failed"}

// checkpointSkip is why a stable point was not checkpointed.
type checkpointSkip int

const (
	// skipNotHost is a SharedObject the engine cannot sign through.
	skipNotHost checkpointSkip = iota
	// skipGroupControl is a SharedObject whose voters decide checkpoints.
	skipGroupControl
	// skipNotCheckpointer is a local peer that is not the checkpointer.
	skipNotCheckpointer
	// skipDeparting is an owner handing over.
	skipDeparting
	// skipReceiptPending is an admission waiting for its receipt.
	skipReceiptPending
	// skipFewStable is fewer stable operations than a checkpoint needs.
	skipFewStable
	// skipNoWorld is a stable point whose World the replay does not hold.
	skipNoWorld
	// numCheckpointSkips is the number of reasons.
	numCheckpointSkips
)

// checkpointSkipNames are the names the flows give each reason.
var checkpointSkipNames = [numCheckpointSkips]string{
	"not-host", "group-control", "not-checkpointer", "departing",
	"receipt-pending", "few-stable", "no-world",
}

// CommitFlows counts what the process's World engines stored in the
// publications of their commits, and how their stable checkpoints went, since
// the process started.
type CommitFlows struct {
	// Publications is the number of commits whose publication the store
	// admitted.
	Publications uint64
	// Kinds are the blocks and bytes of each kind of record those publications
	// stored.
	Kinds []CommitKindFlow
	// Checkpoints counts the prepared stable checkpoints by outcome.
	Checkpoints []CheckpointCount
	// Skips counts the stable points not checkpointed, by reason.
	Skips []CheckpointCount
}

// CommitKindFlow counts one kind of record the commits stored.
type CommitKindFlow struct {
	// Kind names the record: "world", "payload", "replay" or "state".
	Kind string
	// Blocks is the number of blocks, zero for the state, which is a record
	// outside the block store.
	Blocks uint64
	// Bytes is their length, counted once per write even when the store
	// already held a block.
	Bytes uint64
}

// CheckpointCount counts the stable checkpoints with one outcome, or the
// stable points skipped for one reason.
type CheckpointCount struct {
	// Name names the outcome or reason.
	Name string
	// Count is the number counted.
	Count uint64
}

// commitFlows holds the process's counters.
var commitFlows struct {
	publications atomic.Uint64
	blocks       [numCommitKinds]atomic.Uint64
	bytes        [numCommitKinds]atomic.Uint64
	outcomes     [numCheckpointOutcomes]atomic.Uint64
	skips        [numCheckpointSkips]atomic.Uint64
}

// GetCommitFlows returns the process's commit flows, in a fixed order.
func GetCommitFlows() *CommitFlows {
	// Read each kind.
	flows := &CommitFlows{Publications: commitFlows.publications.Load()}
	for kind := range numCommitKinds {
		flows.Kinds = append(flows.Kinds, CommitKindFlow{
			Kind:   commitKindNames[kind],
			Blocks: commitFlows.blocks[kind].Load(),
			Bytes:  commitFlows.bytes[kind].Load(),
		})
	}

	// Read the checkpoint outcomes and skips.
	for outcome := range numCheckpointOutcomes {
		flows.Checkpoints = append(flows.Checkpoints, CheckpointCount{Name: checkpointOutcomeNames[outcome], Count: commitFlows.outcomes[outcome].Load()})
	}
	for skip := range numCheckpointSkips {
		flows.Skips = append(flows.Skips, CheckpointCount{Name: checkpointSkipNames[skip], Count: commitFlows.skips[skip].Load()})
	}
	return flows
}

// countPublication counts an admitted commit publication: the batch entries
// its payload roots reach are payload, the other entries World pages, and the
// save entries the replay's. A payload whose entry lacks its references counts
// only its root as payload.
func countPublication(entries []*block.PutBatchEntry, payloads []*block.BlockRef, save []*block.PutBatchEntry) {
	// Find the entries the payload roots reach within the batch.
	commitFlows.publications.Add(1)
	inPayload := payloadEntries(entries, payloads)

	// Count each entry under its kind.
	for _, e := range entries {
		kind := kindWorld
		if inPayload[e] {
			kind = kindPayload
		}
		countEntry(kind, e)
	}
	for _, e := range save {
		countEntry(kindReplay, e)
	}
}

// payloadEntries returns the entries that payloads reach through the
// references the entries declare, or nil when there are no payloads.
func payloadEntries(entries []*block.PutBatchEntry, payloads []*block.BlockRef) map[*block.PutBatchEntry]bool {
	// Index the entries by hash.
	if len(payloads) == 0 {
		return nil
	}
	byHash := make(map[string]*block.PutBatchEntry, len(entries))
	for _, e := range entries {
		byHash[string(e.Ref.GetHash().GetHash())] = e
	}

	// Walk from each payload root through the entries it reaches.
	reached := make(map[*block.PutBatchEntry]bool)
	stack := append([]*block.BlockRef(nil), payloads...)
	for len(stack) != 0 {
		ref := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		e := byHash[string(ref.GetHash().GetHash())]
		if e == nil || reached[e] {
			continue
		}
		reached[e] = true
		stack = append(stack, e.Refs...)
	}
	return reached
}

// countEntry counts a stored entry under kind. A tombstone stores nothing.
func countEntry(kind commitKind, e *block.PutBatchEntry) {
	if !e.Tombstone {
		commitFlows.blocks[kind].Add(1)
		commitFlows.bytes[kind].Add(uint64(len(e.Data)))
	}
}

// countedHead returns head with the length of each record it writes counted
// under kind, or nil for a nil head.
func countedHead(head *block.AtomicHeadUpdate, kind commitKind) *block.AtomicHeadUpdate {
	// Keep a nil head nil.
	if head == nil {
		return nil
	}

	// Count the replacement each time the store computes it.
	replace := head.Replace
	counted := *head
	counted.Replace = func(ctx context.Context, current []byte, found bool) ([]byte, error) {
		next, err := replace(ctx, current, found)
		if err == nil {
			commitFlows.bytes[kind].Add(uint64(len(next)))
		}
		return next, err
	}
	return &counted
}

// countCheckpoint counts a prepared stable checkpoint's outcome.
func countCheckpoint(outcome checkpointOutcome) {
	commitFlows.outcomes[outcome].Add(1)
}

// skipCheckpoint counts a stable point not checkpointed for reason.
func skipCheckpoint(reason checkpointSkip) {
	commitFlows.skips[reason].Add(1)
}
