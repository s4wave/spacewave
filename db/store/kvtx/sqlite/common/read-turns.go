package common

import (
	"context"
	"runtime"
)

// readTurns admits the statements of a store's snapshot reads into SQLite a
// bounded number at a time, twice the processors; a nil readTurns admits
// every statement at once.
//
// After a restart every client reconnects, and thousands of read
// transactions run on one file at once. Each statement's call holds an OS
// thread inside SQLite, so the writer's thread gets a sliver of the CPU and
// World writes stall. Measured with TestSQLiteReadBurst on a 24 processor
// host, 3.3 GB file, 8,000 readers of 4 rows each for 15 s beside a writer
// committing 4 rows about 50 times a second: without turns the writer
// committed 21 times and the slowest read took 3.7 s; with 48 turns it
// committed 662 times and the slowest read took 1.1 s.
//
// A turn covers only SQLite calls on the transaction's own connection, never
// a wait for a pooled connection or the caller's code between statements,
// such as a scan's callback, so no reader waits for a turn holding one and
// every turn ends. ReadTurns.tla checks this. Write transactions and the
// checkpointer take no turn.
type readTurns chan struct{}

// newReadTurns returns the turns of a store's snapshot reads.
func newReadTurns() readTurns {
	return make(readTurns, 2*runtime.GOMAXPROCS(0))
}

// enter waits for a turn until ctx ends.
func (r readTurns) enter(ctx context.Context) error {
	if r == nil {
		return nil
	}
	select {
	case r <- struct{}{}:
		return nil
	default:
	}
	select {
	case r <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// leave ends the turn enter began.
func (r readTurns) leave() {
	if r != nil {
		<-r
	}
}

// queryRow runs query on q in a turn and scans its row into dest.
func (r readTurns) queryRow(ctx context.Context, q sqliteQueryer, query string, args []any, dest ...any) error {
	if err := r.enter(ctx); err != nil {
		return err
	}
	defer r.leave()
	return q.QueryRowContext(ctx, query, args...).Scan(dest...)
}
