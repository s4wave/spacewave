package s4db

import "context"

// Release returns the space the latest commits freed, which no snapshot
// reads, to the file system, without waiting for a later commit. A commit's
// frees are released once a later record or a durable checkpoint marks the
// commit durable, since recovery checks the values of records past the last
// mark. So a writer that has stopped committing, such as a volume refusing
// writes at its disk reserve, calls Release to see the space its last deletes
// freed. Release checkpoints when no mark covers the latest commit, which
// writes the index pages the commits changed.
func (db *DB) Release(ctx context.Context) error {
	// Take the writer lock while no background checkpoint runs.
	if err := db.lockIdle(ctx); err != nil {
		return err
	}

	// Mark the latest commit durable with a checkpoint when no mark does,
	// then release.
	var err error
	if _, logged, _ := db.flush.marks(); logged < db.cur.Load().seq {
		err = db.w.checkpointDurable(ctx)
	}
	if err == nil {
		err = db.w.releaseNow(ctx)
	}
	db.w.unlock(err)
	return err
}
