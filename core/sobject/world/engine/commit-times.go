package sobject_world_engine

import "time"

// CommitTimes are the stage times of one committed World write.
type CommitTimes struct {
	// Elapsed is the write from its writer lock wait to the end of Commit.
	Elapsed time.Duration
	// Hold is the writer lock hold: the stages that ran under the lock.
	Hold time.Duration
	// Optimistic is set when the write ran its callback without the lock.
	Optimistic bool
	// Stages lists every stage in order, including the ones that took no time.
	Stages []CommitStageTime
}
