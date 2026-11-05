package space_world

// The named roots a Space holds on its SharedObject's block store. Each holds
// one World root block, or one small block naming several roots, and moves
// only through SetRoots with the reference it held as Previous.
const (
	// RootReplayBase holds the World of the adopted checkpoint, which every
	// replay starts from.
	RootReplayBase = "replay-base"
	// RootCheckpoint holds the HeldRoots block of every World Root returned
	// since the last Install, so the one a checkpoint adopts stays held until
	// Install moves the replay base onto it.
	RootCheckpoint = "checkpoint"
	// RootReplayCursor holds the HeldRoots block of every World the replay
	// cursor was saved at since a checkpoint covered them, the last one the
	// World after the positions the saved cursor records.
	RootReplayCursor = "replay-cursor"
	// RootReplayPayloads holds the HeldRoots block naming the payload
	// roots of the operations above the checkpoint, and of the local
	// candidates queued after them.
	RootReplayPayloads = "replay-payloads"
	// RootRetained holds the RetainedRootSet block of the installed head's
	// retained roots.
	RootRetained = "retained"
	// RootPending holds the HeldRoots block of every local candidate whose
	// pages outgrew memory since a checkpoint covered them, so the pages its
	// writer's stage held stay held once the turn ends.
	RootPending = "pending"
	// RootBackfill holds what this device's backfill copied while its
	// backfill choice is on.
	RootBackfill = "backfill"
)

// heldRootNames are the roots an Adapter reads at open.
var heldRootNames = []string{
	RootReplayBase,
	RootCheckpoint,
	RootReplayCursor,
	RootReplayPayloads,
	RootRetained,
	RootPending,
	RootBackfill,
}
