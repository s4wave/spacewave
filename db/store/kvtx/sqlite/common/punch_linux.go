//go:build linux

package common

import (
	"context"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/s4wave/spacewave/db/sqlitefree"
)

// punchTurnWait bounds how long a punch round waits for the writer turn
// before it gives the round up.
const punchTurnWait = time.Second

// punchStep runs one freelist punch round when the punch is on and its
// interval has passed since the last round. The checkpointer calls it after
// each checkpoint round.
func (s *Store[T]) punchStep(ctx context.Context) {
	// Skip a store with the punch off, a reader, or one in memory.
	cfg := s.FreelistPunchConfig()
	if !cfg.Enabled || s.readOnly {
		return
	}
	file, err := s.dbFile()
	if err != nil || file == "" {
		return
	}

	// Run at most one round per interval, the first an interval after the
	// store opened.
	s.punchMtx.Lock()
	defer s.punchMtx.Unlock()
	if time.Since(s.punchLast) < cfg.Interval {
		return
	}
	s.punchLast = time.Now()

	// Count the round's outcome.
	switch err := s.punchRound(ctx, cfg, file); {
	case errors.Is(err, errPunchBusy):
		s.stats.punchSkippedBusy.Add(1)
	case err != nil:
		s.stats.punchRefused.Add(1)
	}
}

// errPunchBusy ends a punch round whose checkpoint could not empty the WAL,
// such as for a reader of an older snapshot.
var errPunchBusy = errors.New("sqlite: punch: the WAL could not be checkpointed to empty")

// punchRound punches one capped batch of the freelist. With the writer turn
// held, a checkpoint copies the whole WAL into the database file and empties
// it, and SQLite's write lock, taken next, keeps any connection from adding
// to the log; so while the round holds it no checkpoint can write the file,
// and the file alone holds the committed database. The walk is strict, and
// only free leaf pages past the reserve are punched. A free leaf holds
// nothing SQLite reads: it allocates a leaf from the freelist as a zeroed
// page without reading it.
func (s *Store[T]) punchRound(ctx context.Context, cfg FreelistPunch, file string) error {
	// Copy the WAL without the turn first, so the copy under the turn is
	// only of what writers add meanwhile.
	_, _ = s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)")

	// Take the writer turn behind every waiting foreground writer, giving the
	// round up when it does not come in time.
	turnCtx, cancel := context.WithTimeout(ctx, punchTurnWait)
	release, err := s.writeMtx.Lock(turnCtx, writerBackground)
	cancel()
	if err != nil {
		return err
	}
	held := time.Now()
	defer func() {
		release()
		s.stats.addPunchHold(time.Since(held))
	}()

	// Take a connection with a short busy wait, restored before it returns
	// to the pool.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout="+strconv.FormatInt(truncateBusyTimeout.Milliseconds(), 10)); err != nil {
		return err
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "PRAGMA busy_timeout=5000") }()

	// Checkpoint the WAL to empty, giving the round up when a reader or a
	// writer keeps any frame from being copied.
	var busy, frames, copied int
	if err := conn.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &frames, &copied); err != nil {
		if s.config.IsBusyError(err) {
			return errPunchBusy
		}
		return err
	}
	if busy != 0 || frames != copied {
		return errPunchBusy
	}
	if err := s.punchStage("checkpointed"); err != nil {
		return err
	}

	// Take SQLite's write lock for the rest of the round, so no connection,
	// in this process or another, adds a frame a checkpoint could copy into
	// the file while it is walked and punched.
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		if s.config.IsBusyError(err) {
			return errPunchBusy
		}
		return err
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "ROLLBACK") }()

	// Require the WAL still empty: a frame added between the checkpoint and
	// the lock is a commit the file does not hold.
	if info, err := os.Stat(file + "-wal"); err == nil && info.Size() != 0 {
		return errors.New("sqlite: punch: the WAL gained frames before the write lock")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Walk the freelist strictly and plan the punch.
	f, err := s.openPunchFile(file)
	if err != nil {
		return err
	}
	h, plan, err := strictFilePlan(f, cfg.Reserve)
	if err != nil {
		return err
	}
	if err := s.punchStage("walked"); err != nil {
		return err
	}

	// Punch the next ranges past the cursor that are not holes yet.
	batch := s.nextPunchBatch(f, h, plan.Ranges, cfg.MaxRanges)
	if err := sqlitefree.PunchRanges(f, h.PageSize, batch); err != nil {
		return err
	}
	if err := s.punchStage("punched"); err != nil {
		return err
	}

	// Flush the file before the lock and the turn are given back.
	if err := f.Sync(); err != nil {
		return err
	}
	var pages int64
	for _, r := range batch {
		pages += int64(r.Count)
	}
	s.stats.addPunch(len(batch), pages*h.PageSize)
	return nil
}

// strictFilePlan reads the header of f, refusing a stale page count, a file
// size unlike it or auto_vacuum, and walks the freelist strictly.
func strictFilePlan(f *os.File, reserve int64) (sqlitefree.Header, sqlitefree.Plan, error) {
	// Read and check the header against the file size.
	info, err := f.Stat()
	if err != nil {
		return sqlitefree.Header{}, sqlitefree.Plan{}, err
	}
	h, err := sqlitefree.ReadHeader(f, info.Size())
	if err != nil {
		return h, sqlitefree.Plan{}, err
	}
	if err := sqlitefree.CheckAutoVacuum(h); err != nil {
		return h, sqlitefree.Plan{}, err
	}

	// Walk strictly, refusing a count unlike the header's.
	_, plan, err := sqlitefree.StrictPlan(f, h, reserve)
	return h, plan, err
}

// nextPunchBatch picks up to max ranges from rs, in page order from the
// cursor, skipping ranges that are holes already, and moves the cursor past
// the last one picked, or back to the start when it reached the end.
func (s *Store[T]) nextPunchBatch(f *os.File, h sqlitefree.Header, rs []sqlitefree.Range, max int) []sqlitefree.Range {
	// Take the ranges past the cursor until the batch is full.
	var batch []sqlitefree.Range
	for _, r := range rs {
		if r.Start < s.punchCursor || sqlitefree.IsHole(f, h.PageSize, r) {
			continue
		}
		batch = append(batch, r)
		if len(batch) == max {
			s.punchCursor = r.Start + r.Count
			return batch
		}
	}

	// Start the next round from the beginning.
	s.punchCursor = 0
	return batch
}

// openPunchFile returns the database file open for the punch, opening it on
// first use.
func (s *Store[T]) openPunchFile(file string) (*os.File, error) {
	if s.punchFile == nil {
		f, err := os.OpenFile(file, os.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
		s.punchFile = f
	}
	return s.punchFile, nil
}

// punchStage runs the test hook at a named stage of a round.
func (s *Store[T]) punchStage(stage string) error {
	if s.punchHook == nil {
		return nil
	}
	return s.punchHook(stage)
}

// addPunch counts a punch round that released ranges ranges of bytes bytes.
func (s *storeStats) addPunch(ranges int, bytes int64) {
	s.punchRuns.Add(1)
	s.punchRanges.Add(uint64(max(ranges, 0))) //nolint:gosec // not negative.
	s.punchBytes.Add(uint64(max(bytes, 0)))   //nolint:gosec // not negative.
}

// addPunchHold notes that a punch round held the writer turn for d.
func (s *storeStats) addPunchHold(d time.Duration) {
	s.punchHeld.Add(uint64(max(d, 0)))       //nolint:gosec // d is not negative.
	s.lastPunchHold.Store(uint64(max(d, 0))) //nolint:gosec // d is not negative.
	storeMax(&s.maxPunchHold, d)
}
