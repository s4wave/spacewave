//go:build linux

package common

import (
	"context"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/s4wave/spacewave/db/sqlitefree"
	"golang.org/x/sys/unix"
)

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
	case errors.Is(err, errPunchFloor):
		s.stats.punchSkippedFloor.Add(1)
	case err != nil:
		s.stats.punchRefused.Add(1)
	}
}

// errPunchBusy ends a punch round whose checkpoint could not empty the WAL,
// such as for a reader of an older snapshot, or whose WAL gained a frame
// before the write lock.
var errPunchBusy = errors.New("sqlite: punch: the WAL could not be checkpointed to empty")

// errPunchFloor ends a punch round on a disk without its floor of free space.
var errPunchFloor = errors.New("sqlite: punch: free disk space less the file's holes is under the floor")

// punchDiskFree returns the bytes free to an unprivileged writer on the
// filesystem holding path. Tests replace it.
var punchDiskFree = func(path string) (int64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * st.Bsize, nil //nolint:gosec // a filesystem's free bytes fit in int64.
}

// punchRound punches one capped batch of the freelist. It rests on one
// rule of SQLite's WAL: a checkpoint writes frames into the database file
// only while it holds READ_LOCK(0) exclusively (walCheckpoint), and a read
// transaction begun on a WAL with every frame copied holds READ_LOCK(0)
// shared until it ends. So the round checkpoints the WAL to empty, begins a
// read transaction and requires the WAL still empty, which proves its read
// mark is 0; from then until the transaction ends, after the punch and the
// fsync, no connection in any process can write the file. Writers go on
// committing, and may take pages the walk found free, but their copies of
// those pages stay in the WAL, which every reader of a later snapshot reads
// them from; the file's copy of a page free in the round's snapshot is read
// by no one, so punching it loses nothing, and a checkpoint after the round
// writes the WAL's copy over the hole. The round never holds the write lock
// itself, and the writer turn not at all: only the truncating checkpoint
// holds the write lock, for what writers added during the passive copy. The
// walk is strict, and only free leaf pages past the reserve are punched. A
// free leaf holds nothing SQLite reads: it allocates a leaf from the
// freelist as a zeroed page without reading it.
func (s *Store[T]) punchRound(ctx context.Context, cfg FreelistPunch, file string) error {
	// Refuse the round on a disk that could not take back every hole.
	if err := punchFloorCheck(file, cfg.Floor); err != nil {
		return err
	}

	// Copy the WAL without holding SQLite's write lock first, so the
	// truncating checkpoint, which holds it, copies only what writers add
	// meanwhile.
	_, _ = s.db.ExecContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)")

	// Flush what the passive copy wrote, also without the write lock. SQLite
	// flushes the file after a copy only when no writer committed during it,
	// and the truncating checkpoint flushes it whole, so on a busy store the
	// passive copy's pages would otherwise all be flushed under the lock.
	f, err := s.openPunchFile(file)
	if err != nil {
		return err
	}
	if err := syncFileData(f); err != nil {
		return err
	}
	if err := s.punchStage("flushed"); err != nil {
		return err
	}

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
	// writer keeps any frame from being copied. The checkpoint holds
	// SQLite's write lock while it runs, and the round no longer after.
	held := time.Now()
	var busy, frames, copied int
	err = conn.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &frames, &copied)
	s.stats.addPunchHold(time.Since(held))
	if err != nil {
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

	// Begin a read transaction, which takes its read mark at its first
	// read, and keep it to the end of the round.
	if _, err := conn.ExecContext(ctx, "BEGIN"); err != nil {
		return err
	}
	snap := time.Now()
	defer func() {
		_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		s.stats.addPunchSnapshot(time.Since(snap))
	}()
	var one int
	if err := conn.QueryRowContext(ctx, "SELECT 1 FROM sqlite_schema UNION ALL SELECT 1 LIMIT 1").Scan(&one); err != nil {
		if s.config.IsBusyError(err) {
			return errPunchBusy
		}
		return err
	}

	// Require the WAL still empty with the read held: then the read mark is
	// 0, as a frame the transaction could read would keep the WAL from
	// being truncated, and no checkpoint can write the file until the
	// transaction ends. A frame added between the checkpoint and the read
	// may sit under the read mark, where a checkpoint could copy it in.
	if info, err := os.Stat(file + "-wal"); err == nil && info.Size() != 0 {
		return errPunchBusy
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	// Walk the freelist strictly and plan the punch.
	h, plan, err := strictFilePlan(f, cfg.Reserve)
	if err != nil {
		return err
	}
	if err := s.punchStage("walked"); err != nil {
		return err
	}

	// Pick the next ranges past the cursor that are not holes yet.
	batch := s.nextPunchBatch(f, h, plan.Ranges, cfg.MaxRanges)

	// Re-read the header before punching and refuse a file it shows was
	// written since the walk. The read mark is what rules that out; the
	// header alone cannot, since a checkpoint may copy pages taken off the
	// freelist while holding back a newer frame of page 1.
	if err := sameHeader(f, h); err != nil {
		return err
	}

	// Punch the batch.
	if err := sqlitefree.PunchRanges(f, h.PageSize, batch); err != nil {
		return err
	}
	if err := s.punchStage("punched"); err != nil {
		return err
	}

	// Flush the file before the read mark is given back.
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

// sameHeader reads the header of f again and returns an error unless it
// matches h, the header the walk read.
func sameHeader(f *os.File, h sqlitefree.Header) error {
	// Read the header as it is now, against the file's size now.
	info, err := f.Stat()
	if err != nil {
		return err
	}
	now, err := sqlitefree.ReadHeader(f, info.Size())
	if err != nil {
		return err
	}

	// Refuse any field unlike the walk's.
	if now != h {
		return errors.New("sqlite: punch: the file changed under the round's read mark")
	}
	return nil
}

// punchFloorCheck returns errPunchFloor when the free space of the disk
// holding file, less the bytes of holes already in file, is under floor.
// Punching adds as much to the free space as to the holes, and SQLite's
// reuse of free pages takes back at most the holes, so the rule holds the
// floor free after every hole is refilled.
func punchFloorCheck(file string, floor int64) error {
	// Measure the holes as the file's size past its allocated blocks.
	var st unix.Stat_t
	if err := unix.Stat(file, &st); err != nil {
		return err
	}
	holes := max(st.Size-st.Blocks*512, 0)

	// Compare the free space past the holes with the floor.
	free, err := punchDiskFree(file)
	if err != nil {
		return err
	}
	if free-holes < floor {
		return errPunchFloor
	}
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
// the last range it looked at, or back to the start when it reached the end.
// It looks at no more than punchLookFactor times max ranges, so a round
// after the cursor wraps over a file punched through costs as much as any.
func (s *Store[T]) nextPunchBatch(f *os.File, h sqlitefree.Header, rs []sqlitefree.Range, max int) []sqlitefree.Range {
	// Take the ranges past the cursor until the batch is full or the look
	// budget is spent.
	var batch []sqlitefree.Range
	looked := 0
	for _, r := range rs {
		if r.Start < s.punchCursor {
			continue
		}
		looked++
		if !sqlitefree.IsHole(f, h.PageSize, r) {
			batch = append(batch, r)
		}
		if len(batch) == max || looked == punchLookFactor*max {
			s.punchCursor = r.Start + r.Count
			return batch
		}
	}

	// Start the next round from the beginning.
	s.punchCursor = 0
	return batch
}

// punchLookFactor bounds the ranges a round looks at, in its MaxRanges.
const punchLookFactor = 4

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

// addPunchSnapshot notes that a punch round held its read mark, and so
// kept checkpoints from writing the file, for d.
func (s *storeStats) addPunchSnapshot(d time.Duration) {
	s.punchSnapshotHeld.Add(uint64(max(d, 0)))   //nolint:gosec // d is not negative.
	s.lastPunchSnapshot.Store(uint64(max(d, 0))) //nolint:gosec // d is not negative.
	storeMax(&s.maxPunchSnapshot, d)
}

// addPunchHold notes that a punch round held SQLite's write lock for d.
func (s *storeStats) addPunchHold(d time.Duration) {
	s.punchHeld.Add(uint64(max(d, 0)))       //nolint:gosec // d is not negative.
	s.lastPunchHold.Store(uint64(max(d, 0))) //nolint:gosec // d is not negative.
	storeMax(&s.maxPunchHold, d)
}
