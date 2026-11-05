//go:build space_adapter_decl

package space_world

// The named roots a Space holds on its SharedObject's block store. Each holds
// one World root block, or one small block naming several roots, and moves
// only through SetRoots with the reference it held as Previous.
const (
	// RootReplayBase holds the World of the adopted checkpoint, which every
	// replay starts from.
	RootReplayBase = "replay-base"
	// RootCheckpoint holds the World a new checkpoint names from before the
	// SharedObject adopts it until Install moves the replay base onto it.
	RootCheckpoint = "checkpoint"
	// RootReplayCursor holds the World after the positions the saved replay
	// cursor records.
	RootReplayCursor = "replay-cursor"
	// RootReplayPayloads holds one block naming the payload roots of the
	// operations above the checkpoint that no World reaches.
	RootReplayPayloads = "replay-payloads"
	// RootRetained holds the RetainedRootSet block of the installed head's
	// retained roots.
	RootRetained = "retained"
	// RootBackfill holds what this device's backfill copied while its
	// backfill choice is on.
	RootBackfill = "backfill"
)
