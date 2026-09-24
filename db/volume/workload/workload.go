// Package workload records the logical operations a Volume's storage engine
// serves, as runtime/trace log events, so the workload captured from a running
// app can be extracted from its execution trace and replayed against another
// engine.
//
// Recording costs one trace.IsEnabled check per operation when no trace is
// running. Keys are recorded verbatim and values only by length, so a trace
// carries key material: capture traces from test data only.
package workload

// Category is the runtime/trace log category of every workload record.
const Category = "hydra/volume-workload"

// Op names one logical storage operation.
type Op string

// Key-value transaction operations. ID is the transaction, except for
// iterator operations, where ID is the iterator and Parent its transaction.
const (
	// OpTxRead opens a read-only transaction.
	OpTxRead Op = "tx-read"
	// OpTxWrite opens a read-write transaction.
	OpTxWrite Op = "tx-write"
	// OpGet reads Key; Size is the value length, or -1 when absent.
	OpGet Op = "get"
	// OpExists checks Key; Size is 1 when present and 0 when absent.
	OpExists Op = "exists"
	// OpSet writes a value of Size bytes to Key.
	OpSet Op = "set"
	// OpDelete deletes Key.
	OpDelete Op = "delete"
	// OpIterate opens an iterator over the Key prefix; Size is 1 when reverse.
	OpIterate Op = "iterate"
	// OpSeek positions an iterator at Key.
	OpSeek Op = "seek"
	// OpIterEnd closes an iterator that visited Size entries.
	OpIterEnd Op = "iter-end"
	// OpCommit commits a transaction with Size pending mutations.
	OpCommit Op = "commit"
	// OpDiscard discards a transaction.
	OpDiscard Op = "discard"
)

// Block store operations. Key is the binary block reference. ID is the read
// scope or batch the operation belongs to, or 0 for a direct store call.
const (
	// OpPut writes a block of Size bytes.
	OpPut Op = "put"
	// OpTombstone writes a deletion marker within a batch.
	OpTombstone Op = "tombstone"
	// OpPutBatch opens batch ID of Size entries; its entries follow.
	OpPutBatch Op = "put-batch"
	// OpRm removes a block.
	OpRm Op = "rm"
	// OpGetBlock reads a block; Size is its length, or -1 when absent.
	OpGetBlock Op = "get-block"
	// OpStatBlock stats a block; Size is its length, or -1 when absent.
	OpStatBlock Op = "stat-block"
	// OpBlockExists checks a block; Size is 1 when present and 0 when absent.
	OpBlockExists Op = "block-exists"
	// OpExistsBatch checks Size blocks in read scope ID; its checks follow.
	OpExistsBatch Op = "exists-batch"
	// OpReadBegin opens read scope ID over one consistent block view.
	OpReadBegin Op = "read-begin"
	// OpReadEnd releases read scope ID.
	OpReadEnd Op = "read-end"
	// OpSync makes every earlier block write durable.
	OpSync Op = "sync"
)

// Garbage collection operations.
const (
	// OpJournalAppend durably journals a reference change of Size encoded bytes.
	OpJournalAppend Op = "journal-append"
	// OpJournalReplay applies and removes Size journal entries.
	OpJournalReplay Op = "journal-replay"
)
