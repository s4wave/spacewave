package s4db

import "context"

// SpaceReport details where a file's free space lies and what holds it, to
// explain a file longer than the data it holds.
type SpaceReport struct {
	// FreeRuns groups the free runs by length, shortest first.
	FreeRuns []FreeRunClass
	// LargestFreeBytes is the length of the longest free run.
	LargestFreeBytes int64
	// ValueEndBytes is the end of the highest page holding a live value.
	// Compaction can shorten the file toward it.
	ValueEndBytes int64
	// CompactGoalBytes is the end background compaction is moving the file
	// toward, or zero.
	CompactGoalBytes int64
	// PendingValueRuns counts the freed value runs a snapshot may still read.
	PendingValueRuns int
	// PendingPageRuns counts the freed index pages and log chunks a snapshot
	// may still read.
	PendingPageRuns int
	// ReservedBytes is the length of the runs this process's checkpoint is
	// writing into.
	ReservedBytes int64
	// UnpunchedBytes is the length of the runs freed since the last hole
	// punch.
	UnpunchedBytes int64
	// Slots lists the reader slots holding a pin.
	Slots []SlotPin
	// DiskBytes is the space the file system allocates to the file, or -1
	// when it cannot tell.
	DiskBytes int64
	// FreeDiskBytes is the part of DiskBytes inside free runs, which holes
	// were not punched for, or -1 when it cannot tell.
	FreeDiskBytes int64
}

// SpaceReport reports where the file's free space lies.
func (db *DB) SpaceReport(ctx context.Context) (SpaceReport, error) {
	return SpaceReport{}, ErrUnimplemented
}
