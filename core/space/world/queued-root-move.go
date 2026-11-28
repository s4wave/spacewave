package space_world

import "github.com/s4wave/spacewave/db/block"

// queuedRootMove is one root move an admission queued in rootMoveQueue.
type queuedRootMove struct {
	// seq numbers the move in queue order.
	seq uint64
	// owner is the place in the queue of the admission that made the move.
	owner *admissionMoves
	// entries are the blocks the move stores.
	entries []*block.PutBatchEntry
	// roots are the moves, each with the Previous the adapter held.
	roots []block.NamedRoot
	// release ends the stages that hold the move's blocks until it lands or
	// is dropped.
	release []func()
}

// admissionMoves is one admission's place in rootMoveQueue. The queue's mtx
// guards its fields.
type admissionMoves struct {
	// seq numbers the newest move queued for the admission, or is zero.
	seq uint64
	// err is the error that dropped one of its moves, which fails its
	// durable step.
	err error
}
