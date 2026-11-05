package common

import (
	"context"
	"database/sql"
	"os"
	"sync/atomic"
)

// walCheckpointer keeps a database's write-ahead log near its bound. After a
// write commits it compares the log's size with the bound and, past it,
// checkpoints in the background, one checkpoint at a time.
//
// The checkpoint runs in two passes. PASSIVE copies every frame no reader
// needs without taking the writer lock, so writers never wait on the copy,
// however long the log has grown. Only once it has copied the whole log does
// TRUNCATE run: it takes the writer lock to reset and truncate the log, and
// with nothing left to copy it holds it only while it waits, within the busy
// timeout, for readers of the old log to finish. A log readers still need
// stays until a later commit tries again.
type walCheckpointer struct {
	// db is the pool the checkpoint runs on.
	db *sql.DB
	// path is the write-ahead log file.
	path string
	// limit is the log size that starts a checkpoint.
	limit int64
	// running is set while a checkpoint runs.
	running atomic.Bool
}

// afterCommit starts a checkpoint when the log has passed its bound and none
// runs. It reads only the log's size, so a commit pays one stat.
func (c *walCheckpointer) afterCommit() {
	info, err := os.Stat(c.path)
	if err != nil || info.Size() <= c.limit || !c.running.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer c.running.Store(false)
		c.checkpoint(context.Background())
	}()
}

// checkpoint copies the log into the database, then truncates the log when
// the copy reached its end. A failure leaves the log for the next attempt.
func (c *walCheckpointer) checkpoint(ctx context.Context) {
	// Copy what no reader needs, without blocking writers.
	var busy, logFrames, copied int64
	if err := c.db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)").Scan(&busy, &logFrames, &copied); err != nil {
		return
	}
	if logFrames <= 0 || copied < logFrames {
		return
	}

	// Reset and truncate the copied log.
	_ = c.db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &logFrames, &copied)
}
