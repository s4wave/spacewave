// Package common provides shared implementations for SQLite kvtx stores.
package common

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/csync"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/sirupsen/logrus"
)

// ValidateTableName validates that a table name is safe to use in SQL queries.
// It only allows alphanumeric characters and underscores, and must start with a letter or underscore.
func ValidateTableName(table string) error {
	// Require a nonempty SQLite table name before validating its characters.
	if table == "" {
		return errors.New("table name cannot be empty")
	}

	// Table name must match: start with letter/underscore, followed by letters/digits/underscores
	matched, err := regexp.MatchString(`^[a-zA-Z_][a-zA-Z0-9_]*$`, table)
	if err != nil {
		return err
	}
	if !matched {
		return errors.New("invalid table name: must start with letter or underscore and contain only alphanumeric characters and underscores")
	}

	return nil
}

// Store represents a generic SQLite store that can work with any driver.
//
// Write transactions take turns in arrival order, each on a connection of its
// own between BEGIN IMMEDIATE and COMMIT, so writers queue in this process
// and never wait in SQLite's busy handler. A commit appends to the WAL
// without a flush, as synchronous=NORMAL allows: an ordered commit returns
// then, and Commit also waits for a grouped flush of the WAL outside the
// writer turn. A crash may lose an ordered commit only with every commit after
// it, since SQLite recovers the WAL up to its first incomplete frame. The
// writer never checkpoints; a background passive checkpoint moves the WAL into
// the database file without SQLite's write lock, and SQLite itself resets the
// copied log. The checkpointer's work under the writer turn is bounded by
// walTurnHold, so it queues with the writers and holds them briefly.
type Store[T SQLiteDriverConfig] struct {
	// db owns the SQL connection pool and its transaction lifetimes.
	db *sql.DB
	// table is the validated key-value table name.
	table string
	// config classifies driver errors for bounded recovery.
	config T
	// withoutRowID creates a missing table as WITHOUT ROWID.
	withoutRowID bool
	// readOnly refuses write transactions, as Pragmas.ReadOnly describes.
	readOnly bool
	// queries are the statements of the table.
	queries *queries
	// snapshotReads runs each read transaction in one snapshot on a
	// connection of its own. It is off on a pool of one connection, where a
	// held connection would block every other transaction.
	snapshotReads bool
	// readTurns admits the snapshot reads' statements into SQLite, nil
	// without snapshot reads, whose statements wait for the pool instead.
	readTurns readTurns

	// writeMtx admits one write transaction at a time: a transaction opened
	// with block.WithBackground waits behind every waiting
	// foreground writer, and each priority goes in arrival order. Without it
	// a burst of background writes, such as upload marker clears, evictions
	// or cache fills, queues ahead of a player's write and sets its wait.
	writeMtx csync.PrioMutex
	// writersWaiting counts write transactions queued on writeMtx.
	writersWaiting atomic.Int32
	// stats counts the writer turn waits and holds and the passes for Stats.
	stats storeStats
	// flow holds back a writer whose commits outrun the checkpointer.
	flow *walFlow

	// flusher makes ordered commits durable by flushing the WAL.
	flusher *kvtx.OrderedFlusher
	// walBound is the WAL size past which the checkpointer opens a gap in
	// the writes for SQLite to reset the log.
	walBound int64

	// fileMtx guards the fields below.
	fileMtx sync.Mutex
	// file is the main database file SQLite reports, read on first use; the
	// WAL is beside it.
	file string
	// fileRead marks file as read.
	fileRead bool

	// hold is the Store's hold on its database file, taken before the pool
	// opened it and released in Close after the pool closes, as sharedFiles
	// describes. It is nil for a pool the caller opened, whose file the Store
	// learns only once the pool has opened it, so that Store writes back only
	// the log.
	hold *fileHold
	// backfill is the log's backfill mark after the checkpointer's last
	// pass, which only the checkpointer moves and reads.
	backfill int
	// copyFrame is how long the last pass that copied frames took per frame,
	// zero before one; only the checkpointer moves and reads it.
	copyFrame time.Duration

	// checkpointWake wakes the checkpointer after a commit.
	checkpointWake chan struct{}
	// stopCheckpoints ends the checkpointer.
	stopCheckpoints context.CancelFunc
	// checkpointsDone is closed when the checkpointer has returned.
	checkpointsDone chan struct{}
}

// ErrReadOnly is returned for a write transaction on a store opened with
// Pragmas.ReadOnly.
var ErrReadOnly = errors.New("sqlite: store is read-only")

// SyncDeadline is how long an ordered commit may stay pending before the store
// flushes the WAL on its own.
const SyncDeadline = time.Second

// CheckpointInterval is how long commits gather before the background
// checkpoint after the first of them.
const CheckpointInterval = time.Second

// DefaultWALCheckpointBytes is the WAL bound of a store whose Pragmas set
// none.
const DefaultWALCheckpointBytes = 64 << 20

// walGapQueueMultiple is how many times its bound the WAL may grow while
// fillWALGap waits for a free writer turn before it queues for the turn
// instead. Writers that run back to back leave the turn free rarely.
const walGapQueueMultiple = 2

// walHoldMultiple is how many times its bound the WAL may grow while the
// passes fall behind the writers before fillWALGap holds the writers back.
const walHoldMultiple = 8

// walTurnHold bounds each hold of the writer turn by the checkpointer: a pass
// under the turn runs with this deadline, which interrupts it, and a
// back-pressure slice releases the turn after it. An interrupted pass leaves
// the log's backfill mark where it was, so it loses its own progress and no
// commit. Forge copies about 8,600 frames a second, so a gap of a few
// hundred frames fits.
const walTurnHold = 25 * time.Millisecond

// walGapHold is the deadline of the pass that closes the gap under the turn:
// walFlowWaitMax, the wait the flow already allows a writer, and shorter only
// in tests that interrupt every gap. fillWALGap tries the gap only for frames
// the measured copy rate copies within walFlowWaitMax.
var walGapHold = walFlowWaitMax

// walGapFrames is the most frames a commit may add and count as small: walFlow
// holds a commit that adds more once the log is far past its bound. Before any
// pass has measured the copy rate, it is also the most frames fillWALGap
// copies under the turn: about 15 ms at 8,600 frames a second.
const walGapFrames = 128

// walWriteBackBytes is the log size past which a pass writes the log and the
// database file back around itself with writeBack. A smaller log leaves
// SQLite's flushes too little to write to delay a commit, and writing back
// beside a writer that flushes each commit only takes the device from it.
const walWriteBackBytes = 4 << 20

// walBehindBytes is how much the log may grow past its file's end while one
// pass runs before the checkpointer finds the pass behind the writers. A
// reader that only keeps SQLite from resetting the log, with the copy caught
// up, grows it by a few commits a pass and never holds the writers back.
const walBehindBytes = 1 << 20

// checkpointHook, when set, is told each step of fillWALGap, each copy a pass
// starts, and when the checkpointer takes and frees the writer turn, for tests
// that stop, slow or time the store between them.
var checkpointHook func(step string)

// copyDelay, set only by tests, lengthens each pass by that long before it
// copies, standing in for a slow disk: a deadline that ends during it cuts
// the pass as one that ends during a copy does.
var copyDelay time.Duration

// copyFrameDelay, set only by tests, lengthens each pass by that long for
// each frame it copied, standing in for a disk that copies at a known rate.
// It waits after the copy, for the frames SQLite copied, so the rate a pass
// measures is the one set; a deadline that ends during it fails the pass
// and leaves the backfill mark where it was, as a cut copy does.
var copyFrameDelay time.Duration

// diskless, set only by tests before a store opens, keeps the device out of
// every pass and commit: synchronous=OFF, and no flush or write-back, so a
// pass takes copyDelay and the copy through the page cache on any host.
var diskless bool

// tell tells checkpointHook, when set, that the checkpointer reached step.
func tell(step string) {
	if checkpointHook != nil {
		checkpointHook(step)
	}
}

// NewStore constructs a new key-value store from a SQLite database.
func NewStore[T SQLiteDriverConfig](db *sql.DB, table string, config T) (*Store[T], error) {
	return newStore(db, table, config, DefaultWALCheckpointBytes, nil)
}

// newStore is NewStore with the WAL bound and the hold on the database file,
// which the Store owns once newStore returns it.
func newStore[T SQLiteDriverConfig](db *sql.DB, table string, config T, walBound int64, hold *fileHold) (*Store[T], error) {
	// Reject an invalid table before binding it to the pool.
	if err := ValidateTableName(table); err != nil {
		return nil, err
	}

	// Bind the pool and start the background checkpointer.
	ctx, stop := context.WithCancel(context.Background()) //nolint:gosec // Close calls stop.
	s := &Store[T]{
		db:              db,
		table:           table,
		config:          config,
		queries:         newQueries(table),
		snapshotReads:   db.Stats().MaxOpenConnections != 1,
		walBound:        walBound,
		hold:            hold,
		checkpointWake:  make(chan struct{}, 1),
		stopCheckpoints: stop,
		checkpointsDone: make(chan struct{}),
	}
	if s.snapshotReads {
		s.readTurns = newReadTurns()
	}
	s.flusher = kvtx.NewOrderedFlusher(s.flushWAL, SyncDeadline)
	s.flow = newWALFlow(s.walSize, s.wakeCheckpoint, walBound, &s.stats)
	go s.runCheckpoints(ctx)
	openStores.Store(s, struct{}{})
	return s, nil
}

// Open opens a SQLite database store using the configured driver.
func Open[T SQLiteDriverConfig](ctx context.Context, path string, table string, config T) (*Store[T], error) {
	return OpenWithPragmas(ctx, path, table, Pragmas{}, config)
}

// OpenWithPragmas opens a SQLite database store using the configured driver
// and applies the supplied tunable pragmas. Zero-valued pragma fields leave
// the SQLite default in place.
func OpenWithPragmas[T SQLiteDriverConfig](ctx context.Context, path string, table string, pragmas Pragmas, config T) (*Store[T], error) {
	// Reject an invalid table before opening the database.
	if err := ValidateTableName(table); err != nil {
		return nil, err
	}

	// Hold the file before the pool opens it, and release the hold once a
	// failed open has closed the pool.
	hold := sharedFiles.hold(path)
	store, err := openHeld(ctx, path, table, pragmas, config, hold)
	if err != nil {
		return nil, errors.Join(err, hold.release())
	}
	return store, nil
}

// openHeld is OpenWithPragmas with the hold on the file at path taken. A Store
// it returns owns hold; on an error it has closed the pool.
func openHeld[T SQLiteDriverConfig](ctx context.Context, path string, table string, pragmas Pragmas, config T, hold *fileHold) (*Store[T], error) {
	// Open the pool through a connector that configures every connection.
	// The DSN sets WAL mode where the driver reads it, before any statement.
	db, err := openPool(config, path, pragmas, hold)
	if err != nil {
		return nil, err
	}
	if poolConfigurator, ok := any(config).(SQLiteDriverPoolConfigurator); ok {
		poolConfigurator.ConfigureDBPool(db)
	}

	// Open a reader on the table as the file's writer left it.
	if pragmas.ReadOnly {
		return openReadOnly(ctx, db, table, config, hold)
	}

	// page_size must be applied before the WAL header is written and before
	// any tables are created on a fresh database. On an existing database it
	// has no effect unless followed by VACUUM, which we do not run here.
	if pragmas.PageSize != 0 {
		if _, err := db.ExecContext(ctx, "PRAGMA page_size="+strconv.FormatInt(int64(pragmas.PageSize), 10)); err != nil {
			db.Close()
			return nil, err
		}
	}

	// Select WAL mode, which the database keeps across connections.
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, err
	}

	// Initialize the table through the same path used by injected SQL pools.
	return openDB(ctx, db, table, pragmas.WithoutRowID, walBound(pragmas), config, hold)
}

// walBound returns the WAL bound pragmas set, or the default.
func walBound(pragmas Pragmas) int64 {
	if pragmas.WALCheckpointBytes > 0 {
		return pragmas.WALCheckpointBytes
	}
	return DefaultWALCheckpointBytes
}

// openPool opens a pool on the database at path whose connections each run
// the connection-scoped pragmas: the durability and busy wait, the tunables
// set in pragmas, no automatic checkpoint, since the store checkpoints in the
// background, and the WAL bound, to which SQLite truncates the log whenever it
// restarts it. Each connection holds the file through hold for its life.
func openPool[T SQLiteDriverConfig](config T, path string, pragmas Pragmas, hold *fileHold) (*sql.DB, error) {
	// Find the driver registered under the config's name, opening a reader
	// through its read-only DSN.
	dsn := config.OpenDSN(path)
	if pragmas.ReadOnly {
		ro, ok := any(config).(SQLiteReadOnlyDSN)
		if !ok {
			return nil, errors.New("sqlite: the driver cannot open a database read-only")
		}
		dsn = ro.OpenReadOnlyDSN(path)
	}
	probe, err := sql.Open(config.DriverName(), dsn)
	if err != nil {
		return nil, err
	}
	drv := probe.Driver()
	if err := probe.Close(); err != nil {
		return nil, err
	}

	// List the pragmas every connection runs.
	synchronous := "NORMAL"
	switch {
	case diskless:
		synchronous = "OFF"
	case pragmas.FullSync:
		synchronous = "FULL"
	}
	connPragmas := []string{
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=" + synchronous,
		"PRAGMA wal_autocheckpoint=0",
		"PRAGMA journal_size_limit=" + strconv.FormatInt(walBound(pragmas), 10),
	}
	if pragmas.ReadOnly {
		connPragmas = append(connPragmas, "PRAGMA query_only=1")
	}
	if pragmas.CacheSize != 0 {
		connPragmas = append(connPragmas, "PRAGMA cache_size="+strconv.FormatInt(int64(pragmas.CacheSize), 10))
	}
	if pragmas.DisableMmap || pragmas.MmapSize != 0 {
		mmapSize := pragmas.MmapSize
		if pragmas.DisableMmap {
			mmapSize = 0
		}
		connPragmas = append(connPragmas, "PRAGMA mmap_size="+strconv.FormatInt(mmapSize, 10))
	}
	if pragmas.TempStore != 0 {
		connPragmas = append(connPragmas, "PRAGMA temp_store="+strconv.FormatInt(int64(pragmas.TempStore), 10))
	}

	// Open the pool through the configuring connector.
	connector, err := NewPragmaConnector(drv, dsn, connPragmas)
	if err != nil {
		return nil, err
	}
	connector.file = hold

	// Keep enough idle connections for the concurrent read transactions,
	// each of which holds one for its life, so a burst of reads reuses
	// warm connections instead of opening new ones.
	db := sql.OpenDB(connector)
	db.SetMaxIdleConns(idleConns())
	return db, nil
}

// idleConns is the number of idle connections a pool keeps: eight per
// processor, and at least 16. A burst of views, such as every client
// reconnecting after a restart, holds hundreds of read transactions at once;
// a connection past the idle limit closes when its transaction ends, taking
// its prepared statements and page cache with it, and the next burst opens
// it again.
func idleConns() int {
	return max(16, 8*runtime.GOMAXPROCS(0))
}

// OpenDB takes ownership of an already configured SQL pool and opens its KV table.
// It closes the pool on failure; the caller selects durability on every connection.
// Its checkpointer writes back only the log, as Store's hold describes.
func OpenDB[T SQLiteDriverConfig](ctx context.Context, db *sql.DB, table string, config T) (*Store[T], error) {
	return openDB(ctx, db, table, false, DefaultWALCheckpointBytes, config, nil)
}

// openDB is OpenDB with the shape of a table it creates and the WAL bound.
func openDB[T SQLiteDriverConfig](ctx context.Context, db *sql.DB, table string, withoutRowID bool, walBound int64, config T, hold *fileHold) (*Store[T], error) {
	// Bind the validated table to its supplied database pool.
	store, err := newStore(db, table, config, walBound, hold)
	if err != nil {
		db.Close()
		return nil, err
	}
	store.withoutRowID = withoutRowID

	// Initialize table with retry on SQLITE_BUSY.
	// This can happen when another process is closing the database and holds
	// an exclusive lock during WAL cleanup, or when recovering from a crash.
	if err := store.initTableWithRetry(ctx); err != nil {
		return nil, errors.Join(err, store.Close())
	}

	return store, nil
}

// openReadOnly binds a read-only store to db's existing table. Its
// checkpointer never runs, since only this store's commits wake it.
func openReadOnly[T SQLiteDriverConfig](ctx context.Context, db *sql.DB, table string, config T, hold *fileHold) (*Store[T], error) {
	// Bind the pool, then require the table, which a reader must not create.
	store, err := newStore(db, table, config, DefaultWALCheckpointBytes, hold)
	if err != nil {
		db.Close()
		return nil, err
	}
	store.readOnly = true
	var name string
	err = db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		err = errors.New("sqlite: table " + table + " does not exist")
	}
	if err != nil {
		return nil, errors.Join(err, store.Close())
	}
	return store, nil
}

// OpenWithMode opens a SQLite database store with file mode.
func OpenWithMode[T SQLiteDriverConfig](ctx context.Context, path string, mode os.FileMode, table string, config T) (*Store[T], error) {
	return OpenWithModeAndPragmas(ctx, path, mode, table, Pragmas{}, config)
}

// OpenWithModeAndPragmas opens a SQLite database store with file mode and the
// supplied tunable pragmas.
func OpenWithModeAndPragmas[T SQLiteDriverConfig](ctx context.Context, path string, mode os.FileMode, table string, pragmas Pragmas, config T) (*Store[T], error) {
	// For SQLite, we can create the file with the specified mode before opening
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if file, err := os.OpenFile(path, os.O_CREATE, mode); err == nil {
			file.Close()
		}
	}

	return OpenWithPragmas(ctx, path, table, pragmas, config)
}

// GetDB returns the SQL DB.
func (s *Store[T]) GetDB() *sql.DB {
	return s.db
}

// initTable creates the key-value table if it doesn't exist.
func (s *Store[T]) initTable() error {
	query := `CREATE TABLE IF NOT EXISTS ` + s.table + ` ( ` + //nolint:gosec
		`key BLOB PRIMARY KEY, value BLOB)`
	if s.withoutRowID {
		query += ` WITHOUT ROWID`
	}
	_, err := s.db.Exec(query)
	return err
}

// initTableWithRetry creates the key-value table with retry on SQLITE_BUSY.
func (s *Store[T]) initTableWithRetry(ctx context.Context) error {
	// Bound SQLite table initialization retries and their backoff.
	backoff := 50 * time.Millisecond
	maxBackoff := 2 * time.Second
	maxRetries := 10

	// Retry SQLite table initialization while retaining the last busy error.
	var lastErr error
	for range maxRetries {
		if err := ctx.Err(); err != nil {
			return err
		}

		err := s.initTable()
		if err == nil {
			return nil
		}

		lastErr = err
		if !s.config.IsBusyError(err) {
			return err
		}

		// Backoff before retry
		select {
		case <-ctx.Done():
			return ctx.Err()
		//gostyle:timer backoff
		case <-time.After(backoff):
		}

		// Exponential backoff with cap
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return errors.Join(errors.New("sqlite: database busy during initialization after retries"), lastErr)
}

// Priorities of Store.writeMtx.
const (
	// writerBackground is the priority of write transactions opened with
	// block.WithBackground.
	writerBackground = iota - 1
	// writerForeground is the priority of every other write transaction.
	writerForeground
	// writerSlice is the priority of a held pass's next slice once the
	// commits admitted since its last slice spent their frame budget, as
	// lockSlice describes.
	writerSlice
)

// NewTransaction returns a new transaction against the store. A write
// transaction waits for the writer turn until ctx ends, behind every waiting
// foreground writer when ctx carries block.WithBackground.
func (s *Store[T]) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// Read transactions open their connection at their first read.
	if !write {
		return newReadTx(s.db, s.queries, s.snapshotReads, s.readTurns), nil
	}

	// A read-only store never takes the writer's lock from the file's writer.
	if s.readOnly {
		return nil, ErrReadOnly
	}

	// Queue for the writer turn, counted for WritersWaiting.
	prio := writerForeground
	if block.IsBackground(ctx) {
		prio = writerBackground
	}
	s.writersWaiting.Add(1)
	queued := time.Now()
	release, err := s.writeMtx.Lock(ctx, prio)
	s.writersWaiting.Add(-1)
	if err != nil {
		return nil, err
	}

	// Count the wait, and the hold when the transaction returns the turn.
	turn := time.Now()
	s.stats.addWait(prio, turn.Sub(queued))
	release = func(release func()) func() {
		return func() {
			s.stats.addHold(prio, time.Since(turn))
			release()
		}
	}(release)

	// Begin on a connection of its own, taking SQLite's write lock at once so
	// the transaction never waits to upgrade from a read.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		release()
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		discardConn(conn)
		release()
		return nil, err
	}
	return newWriteTx(conn, s.queries, s.flusher, s.flow, release), nil
}

// WritersWaiting reports whether a write transaction waits for the writer
// turn.
func (s *Store[T]) WritersWaiting() bool {
	return s.writersWaiting.Load() > 0
}

// Sync makes every completed ordered commit durable, flushing the WAL once
// for all of them.
func (s *Store[T]) Sync(ctx context.Context) error {
	return s.flusher.Sync(ctx)
}

// WaitDurable waits until every ordered commit completed before the call is
// durable, without forcing a flush. It returns the deadline flush error if
// that flush fails first.
func (s *Store[T]) WaitDurable(ctx context.Context) error {
	return s.flusher.WaitDurable(ctx)
}

// SupportsAtomicCommit reports true: a write transaction is one SQLite
// transaction that takes the reserved lock when it opens, so its reads see
// every committed write and no other writer commits until it ends, and its
// Commit lands all of its writes or none of them, durable once the store's
// WAL flush returns.
func (s *Store[T]) SupportsAtomicCommit() bool {
	return true
}

// Execute executes the given store.
func (s *Store[T]) Execute(ctx context.Context) error {
	return nil
}

// Close stops the checkpointer, flushes pending ordered commits, and closes
// the database.
func (s *Store[T]) Close() error {
	// Leave the stats registry, then wait for the checkpointer's running step.
	openStores.Delete(s)
	s.stopCheckpoints()
	<-s.checkpointsDone

	// Release the file after the pool. A connection a caller still uses
	// holds the file itself until it closes, so SQLite keeps its locks.
	return errors.Join(s.flusher.Close(), s.db.Close(), s.hold.release())
}

// flushWAL makes every commit the WAL holds durable through the driver.
func (s *Store[T]) flushWAL() error {
	if diskless {
		return nil
	}
	file, err := s.dbFile()
	if err != nil {
		return err
	}
	return s.config.SyncWAL(file)
}

// dbFile returns the main database file SQLite reports, empty for one in
// memory, reading it on first use.
func (s *Store[T]) dbFile() (string, error) {
	// Return the file read before.
	s.fileMtx.Lock()
	defer s.fileMtx.Unlock()
	if s.fileRead {
		return s.file, nil
	}

	// Ask SQLite for the main database.
	var seq int
	var name, file string
	if err := s.db.QueryRow("PRAGMA database_list").Scan(&seq, &name, &file); err != nil {
		return "", err
	}
	s.file, s.fileRead = file, true
	return file, nil
}

// wakeCheckpoint wakes the checkpointer unless a wake is already pending.
func (s *Store[T]) wakeCheckpoint() {
	select {
	case s.checkpointWake <- struct{}{}:
	default:
	}
}

// runCheckpoints moves the WAL into the database file in the background, at
// most once per CheckpointInterval while commits continue. A passive
// checkpoint never takes SQLite's write lock, so no write waits on it.
//
// SQLite resets the log itself (walRestartLog): a writer whose snapshot began
// on a fully copied log, with no older reader still in it, writes from the
// log's first frame again, and journal_size_limit trims the file to the
// bound. Writes with gaps between them begin on a copied log often. Writers
// that run back to back, such as a World regeneration's build, never do: a
// commit always lands between a pass and the next writer's begin. So once
// the log is past its bound, the checkpointer opens the gap itself with
// fillWALGap.
//
// No frame is overwritten before it is synced to the database file. The
// checkpointer only copies frames; under synchronous=NORMAL a checkpoint
// syncs the database file before it advances the log's backfill mark, and
// only SQLite rewinds the log, once that mark reaches the log's last frame.
func (s *Store[T]) runCheckpoints(ctx context.Context) {
	// Start in the mode of a log nobody pins, logging no failure, and end
	// every commit's wait once the checkpointer returns.
	defer close(s.checkpointsDone)
	defer s.flow.stop()
	mode := holdOnFall
	var failing error
	var pinned bool
	rest := true
	for {
		// Wait for a commit, ending the wait of each the last pass copied,
		// then let commits gather for one interval. Far past its bound,
		// with a gap pass cut short of the log's end and no reader keeping
		// the log, pass again at once instead: a writer waiting in walFlow
		// commits nothing to wake the checkpointer. Once a pass copied the
		// whole log, or found it empty, wait whatever its size: only the
		// next writer's begin resets it, and a writer waiting in walFlow
		// begins only after this settle. A log is empty and its file large
		// after a transaction spilled into a copied log, which restarts it
		// at once, and rolled back; only the next commit trims the file.
		if rest || mode == holdNever || s.walSize() < walHoldMultiple*s.walBound {
			s.flow.settle(mode == holdNever)
			select {
			case <-ctx.Done():
				return
			case <-s.checkpointWake:
			}
		}
		// Skip the interval, or cut it short, once the log is far past its
		// bound, so writers that outrun the passes are held back before it
		// grows on and a writer waiting in walFlow waits on no interval,
		// unless a reader keeps the log, which no pass can shrink.
		gather := time.NewTimer(CheckpointInterval)
		for gathering := mode == holdNever || s.walSize() < walHoldMultiple*s.walBound; gathering; {
			select {
			case <-ctx.Done():
				gather.Stop()
				return
			case <-gather.C:
				gathering = false
			case <-s.checkpointWake:
				gathering = mode == holdNever || s.walSize() < walHoldMultiple*s.walBound
			}
		}
		gather.Stop()

		// Copy the WAL into the database file, holding the writers back
		// from the start once it is far past its bound; a failure retries
		// after the next commit. Holding from the start changes how often
		// a writer meets a hold, not how long: each slice is walTurnHold,
		// and a writer queued at foreground waits behind at most one, or
		// behind the gap pass, cut at walGapHold (walFlowWaitMax, 250 ms),
		// and its flush of the pages it copied.
		if mode == holdOnFall && s.walSize() >= walHoldMultiple*s.walBound {
			mode = holdAtOnce
		}
		var c checkpoint
		c, mode = s.pass(ctx, s.db, mode)
		if ctx.Err() != nil {
			return
		}
		failing = s.logCheckpoint(failing, c.err)
		pinned = s.logPin(pinned, mode, c)
		if rest = !c.caught() || c.logged <= 0; rest {
			continue
		}

		// Let SQLite reset a log past its bound.
		if s.walSize() > s.walBound {
			mode, rest = s.fillWALGap(ctx, mode)
		}
	}
}

// logCheckpoint logs a pass that failed with err after one that had not, and
// one that succeeded after one that had failed with was, so a lasting failure
// logs once. It returns err. The store has no health report to carry it.
func (s *Store[T]) logCheckpoint(was, err error) error {
	switch {
	case err != nil && was == nil:
		file, _ := s.dbFile()
		logrus.WithError(err).WithField("file", file).Warn("sqlite: checkpoint failing; the log grows until one succeeds")
	case err == nil && was != nil:
		file, _ := s.dbFile()
		logrus.WithField("file", file).Info("sqlite: checkpoint succeeds again")
	}
	return err
}

// logPin logs a reader pinning the log once the log passes walHoldMultiple
// times its bound, and the pin's end once a pass copies again, given was,
// whether the pin is logged, mode, the next pass's, and c, the pass that
// decided it, whose readings the log carries so a misjudged pin explains
// itself. It returns whether the pin is logged. It stats the log only while a
// reader pins it, once a pass.
func (s *Store[T]) logPin(was bool, mode holdMode, c checkpoint) bool {
	// Log the change between a large pin and copying.
	pinned := mode == holdNever
	if pinned == was {
		return was
	}
	size := s.walSize()
	if pinned && size < walHoldMultiple*s.walBound {
		return false
	}
	file, _ := s.dbFile()
	le := logrus.WithFields(logrus.Fields{
		"file":      file,
		"wal-bytes": size,
		"wal-bound": s.walBound,
		"logged":    c.logged,
		"copied":    c.copied,
		"busy":      c.busy,
		"last-mark": c.mark,
	})
	if pinned {
		le.Warn("sqlite: a reader pins the log; it grows until the reader ends")
	} else {
		le.Info("sqlite: the reader left the log; checkpoints copy again")
	}
	return pinned
}

// walSize returns the size of the WAL file on this host, or zero when it is
// unknown.
func (s *Store[T]) walSize() int64 {
	// Stat the log beside the database file.
	file, err := s.dbFile()
	if err != nil || file == "" {
		return 0
	}
	info, err := os.Stat(file + "-wal")
	if err != nil {
		return 0
	}
	return info.Size()
}

// fillWALGap copies the whole log into the database file between two write
// transactions, so the next writer begins on a fully copied log and SQLite
// resets it. A passive pass that ends caught up rarely opens that gap by
// itself: the writer holding the turn began on the uncopied log, and its
// commit lands before the next writer begins.
//
// fillWALGap never waits for the turn while a pass runs, since a turn granted
// during a pass would sit idle until the pass ends. After each commit it
// copies outside the turn. A pass copies the log as it was when the pass
// began, so it passes again at once until a pass finds no frame the last one
// had not seen; then, when the turn is free, it takes the turn, copies the
// little that landed between, and releases it. Once the log reaches
// walGapQueueMultiple times its bound with the passes caught up, it queues for
// the turn behind every waiting writer instead of waiting for a free one.
//
// It keeps two invariants:
//
//   - The checkpointer holds the writer turn at most walTurnHold at a time
//     for a back-pressure slice, which releases the turn and queues behind
//     every writer waiting until their commits spend the slice's frame
//     budget, as lockSlice describes, and at most walGapHold (walFlowWaitMax, 250 ms)
//     for the gap pass, which runs with that deadline. The gap pass is tried
//     only for the frames the last pass measured it copies within
//     walFlowWaitMax, so on a disk that copies them in 10 ms it holds the
//     turn about 10 ms. The one step no deadline interrupts is SQLite's
//     flush of the database file at the end of a gap pass that copies
//     everything, and the write-backs around each pass leave it only the
//     pages the last pass and the gap copied. So a commit waits behind the
//     checkpointer at most one walGapHold plus that flush, and at most one
//     hold per commit: 26 to 35 ms on forge with a writer of 180 MiB/s held
//     back.
//   - While the passes fall behind the writers, or a writer commits during
//     every pass, the log passes walHoldMultiple times its bound only by
//     what the writers commit while passes run with them held back: one
//     slice budget, the frames the copy rate moves in walTurnHold, per
//     walTurnHold, until a pass finds new frames that copy within
//     walFlowWaitMax and the gap closes. The budget paces the writers to
//     the disk however many there are, so a held pass is never outrun; a
//     small commit then waits for the frames queued ahead of it at the copy
//     rate plus one hold, and a large commit at most its own copy plus
//     walFlowWaitMax, as walFlow describes. Below walHoldMultiple times its bound, writers
//     grow the log unchecked.
//
// A reader whose snapshot predates a pass limits how far it copies and keeps
// the next writer from resetting the log. Holding writers back cannot shrink
// that log, so fillWALGap does not: once a pass finds the log pinned, the
// passes hold no writer and run once a CheckpointInterval until a pass copies
// again. The log then grows without bound until the reader moves on, where the
// restart this replaced capped it by stopping the writers until the reader
// finished.
//
// On a pool of several connections the passes run on one of the
// checkpointer's own, taken before the turn, so the pass under the turn never
// waits for the pool; a pool of one lends its connection to each pass, since
// holding it would keep the writer holding the turn from committing. A
// passive checkpoint never calls the busy handler: a reader limits how far it
// copies, and a concurrent checkpoint makes it return busy at once.
//
// It starts its first pass in mode and returns the mode of the pass after its
// last, and rest, whether the checkpointer may wait for the next commit: false
// only when the gap pass ended at its deadline short of the log's end, which
// no commit comes to finish while the writers it held wait in walFlow.
func (s *Store[T]) fillWALGap(ctx context.Context, mode holdMode) (holdMode, bool) {
	// Pick the connection the passes run on.
	var q rowQuerier = s.db
	if s.snapshotReads {
		conn, err := s.db.Conn(ctx)
		if err != nil {
			return mode, true
		}
		defer conn.Close()
		q = conn
	}

	// gap copies under the held turn within walGapHold, releases it, and
	// reports whether it copied the whole log.
	gap := func(release func()) bool {
		// Copy within the deadline, then release the turn.
		tell("turn")
		hold, cancel := context.WithTimeout(ctx, walGapHold)
		c := s.passOnce(hold, q)
		cancel()
		release()
		tell("free")
		tell("gap " + strconv.FormatBool(c.caught()))
		return c.caught()
	}

	// seen is the frame count of the last caught-up pass, or -1 before one.
	// A pass that fell behind the writers holds them back from the start of
	// each pass after it until the gap closes.
	seen := -1
	for {
		// Copy outside the turn.
		tell("pass")
		var c checkpoint
		c, mode = s.pass(ctx, q, mode)
		if ctx.Err() != nil {
			return mode, true
		}
		size := s.walSize()
		behind := mode == holdAtOnce
		switch {
		case size <= s.walBound:
			// SQLite reset the log.
			return holdOnFall, true

		case mode == holdNever:
			// A reader keeps the log: pass again after an interval.
			return mode, true

		case c.caught() && (c.logged == seen || behind && seen >= 0 && s.gapFits(c.logged-seen)):
			// Caught up with no new frame, or with the writers held back
			// few enough to copy within walFlowWaitMax: close the gap in a
			// free turn, or queue for one once the log is far past its
			// bound, in arrival order behind the writers then waiting: a
			// lower priority would yield to every writer arriving after it,
			// and writers that never pause would leave the gap unclosed.
			if release, ok := s.writeMtx.TryLock(); ok {
				return holdOnFall, gap(release)
			}
			if size >= walGapQueueMultiple*s.walBound {
				release, err := s.writeMtx.Lock(ctx, writerForeground)
				if err != nil {
					return mode, true
				}
				return holdOnFall, gap(release)
			}

		case c.caught():
			// New frames landed during the pass: pass again at once. Far
			// past the bound, hold the writers back from its start: a
			// writer slower than the copy that commits during every pass
			// never lets a pass fall behind, nor one find no new frame.
			seen = c.logged
			if size >= walHoldMultiple*s.walBound {
				mode = holdAtOnce
			}
			continue
		}
		seen = c.logged

		// Wait for the next commit, ending the wait of each this pass copied.
		s.flow.settle(false)
		select {
		case <-ctx.Done():
			return mode, true
		case <-s.checkpointWake:
		}
	}
}

// rowQuerier is the pool or one of its connections.
type rowQuerier interface {
	// QueryRowContext runs a query that returns one row.
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// checkpoint is the result of one passive pass.
type checkpoint struct {
	// err is why the pass failed: a cancel, its deadline, or SQLite's error.
	err error
	// busy reports that another checkpoint held the log, so the pass copied
	// nothing.
	busy bool
	// logged is the log's frame count when the pass began, and copied the
	// frames the database file holds after it. SQLite reads the log's
	// header once before it copies and reports that header's count, so
	// frames a writer appends during the pass are never in logged.
	logged, copied int
	// pinned reports that a reader kept the pass from copying any frame past
	// where the last pass left the log.
	pinned bool
	// mark is the frame count the last pass left copied, which pinned
	// compares copied against.
	mark int
}

// caught reports whether the pass copied every frame logged when it began.
func (c checkpoint) caught() bool {
	return c.err == nil && !c.busy && c.copied == c.logged
}

// passOnce runs one passive checkpoint on q. It copies the log as it was when
// the pass began, up to the oldest snapshot a reader holds.
//
// SQLite reports as logged the frame count of the log header it read before
// copying, so frames a writer appends during the pass are never in it, and it
// stops a passive pass short of that count only at a reader's snapshot, never
// failing it for one. A reader that began after the last pass lets this one
// copy up to its snapshot; only a reader older than the last pass stops this
// one where it began. So the pass is pinned when it ends short with no error
// and copied no further than the last, which backfill keeps. A short reader
// whose snapshot sits exactly at the last pass's mark also counts as a pin,
// for that one pass: the passes then wait one CheckpointInterval holding no
// writer, and the next copies past it. Such a snapshot began with the copy at
// the log's end, which writers outrunning the passes never leave. After a
// writer resets the log, the first pass compares against the mark of the old
// log, which can misjudge the pin the same way.
func (s *Store[T]) passOnce(ctx context.Context, q rowQuerier) checkpoint {
	// Run the pass; a busy or failed one judges no pin.
	var c checkpoint
	start := time.Now()
	if c.err = waitCopyDelay(ctx, copyDelay); c.err != nil {
		return c
	}
	c.err = q.QueryRowContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)").Scan(&c.busy, &c.logged, &c.copied)
	if c.err == nil && !c.busy && c.copied > s.backfill {
		c.err = waitCopyDelay(ctx, time.Duration(c.copied-s.backfill)*copyFrameDelay)
	}
	took := time.Since(start)
	s.stats.addPassive(took)
	if c.err != nil || c.busy {
		return c
	}

	// Judge the pin against the last pass's mark, then keep this one's, and
	// the time this pass took per frame it copied.
	c.mark = s.backfill
	c.pinned = c.copied < c.logged && c.copied == c.mark
	s.backfill = c.copied
	if n := c.copied - c.mark; n > 0 {
		s.copyFrame = took / time.Duration(n)
	}
	return c
}

// waitCopyDelay waits out d, a test's stand-in for a slow disk, or returns
// ctx's error once it ends.
func waitCopyDelay(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	delay := time.NewTimer(d)
	defer delay.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-delay.C:
		return nil
	}
}

// gapFits reports whether a pass under the turn would copy frames within
// walFlowWaitMax at the copy rate the last pass measured, or, before one has,
// whether they are at most walGapFrames. The rate includes each pass's fixed
// cost, its flush, spread over its frames, so a small gap after a large pass
// may take longer than estimated; walGapHold then cuts it.
func (s *Store[T]) gapFits(frames int) bool {
	if s.copyFrame <= 0 {
		return frames <= walGapFrames
	}
	return time.Duration(frames)*s.copyFrame <= walFlowWaitMax
}

// holdMode is when a pass holds the writers back.
type holdMode int

const (
	// holdOnFall holds them once the pass falls behind the writers.
	holdOnFall holdMode = iota
	// holdAtOnce holds them from the start: the pass before fell behind
	// and left the log at walHoldMultiple times its bound.
	holdAtOnce
	// holdNever never holds them: the pass before was pinned by a reader,
	// and holding writers back cannot shrink a log a reader keeps.
	holdNever
)

// pass runs passOnce on q outside the writer turn, between write-backs of
// the log and the database file while the log is past its bound and
// walWriteBackBytes, and returns its result and the mode of the next pass.
// In holdOnFall it finds itself behind the writers once the log at
// walHoldMultiple times its bound grows walBehindBytes past its file's end
// while it runs. A pass behind, or one in holdAtOnce, holds the writers back
// until it ends: it holds the writer turn in slices of walTurnHold, and after
// each releases it and queues for it again at foreground priority, behind
// every writer then waiting, so each waiting writer commits once per slice and
// waits behind at most one. The size check runs every walTurnHold, since the
// log's size has no watch. A pass in holdNever holds nothing.
//
// Once ctx is cancelled, pass starts no step and never queues for the turn
// again, so Close waits only for the step running when it cancelled. A
// running pass stops at its next frame, except when the cancel lands before
// SQLite begins the step, which clears the interrupt: that pass copies the
// frames logged when it began, bounded by the log's size then and not by how
// long writers run after it. Its cost is not that copy alone: beside a free
// writer of 170 MiB/s on forge, one such pass over a log of about 13 MB took
// 3.5 s, where the copy takes about 0.1 s; the rest, unmeasured, is likely
// SQLite's flush of the log before it copies, behind the writer's I/O.
func (s *Store[T]) pass(ctx context.Context, q rowQuerier, mode holdMode) (checkpoint, holdMode) {
	// Run the pass beside the check, copying every commit counted so far.
	// The slice budget reads the copy rate before the pass moves it.
	s.flow.pass()
	budget := s.sliceFrames()
	done := make(chan checkpoint, 1)
	go func() {
		// Write the log back, copy, and write the database file back,
		// stopping before any step once cancelled.
		tell("copy")
		over := s.walSize() > max(s.walBound, walWriteBackBytes)
		if over && ctx.Err() == nil {
			s.writeBackLog()
		}
		c := checkpoint{err: ctx.Err()}
		if c.err == nil {
			c = s.passOnce(ctx, q)
		}
		if over && ctx.Err() == nil {
			s.writeBackDB()
			tell("written back")
		}
		done <- c
	}()

	// next is the mode of the pass after one that ended with c, and marks
	// the commits it copied.
	held := false
	next := func(c checkpoint) (checkpoint, holdMode) {
		if c.caught() {
			s.flow.caught()
		}
		switch {
		case c.pinned:
			return c, holdNever
		case (held || mode == holdAtOnce) && s.walSize() >= walHoldMultiple*s.walBound:
			return c, holdAtOnce
		}
		return c, holdOnFall
	}
	if mode == holdNever {
		return next(<-done)
	}
	defer s.flow.disarm()

	// Wait for the pass; once it falls behind, hold the writers back. At
	// once, the check fires at the start; on a fall, each commit during the
	// pass wakes it, so the hold begins at the commit that leaves the pass
	// behind and not up to a slice later, when a writer of small ordered
	// commits, which return before any flush, may have grown the log by
	// tens of MiB. A wake taken here is posted back on return, for
	// fillWALGap's wait for the next commit.
	start := s.walSize()
	check := time.NewTimer(0)
	defer check.Stop()

	// Wake on each commit while holding on a fall.
	var wake <-chan struct{}
	if mode == holdOnFall {
		check.Stop()
		wake = s.checkpointWake
	}

	// Post back a wake taken here, and keep the armed budget's channel
	// across slices.
	woke := false
	var spent <-chan struct{}
	defer func() {
		if woke {
			s.wakeCheckpoint()
		}
	}()
	for {
		select {
		case c := <-done:
			return next(c)
		case <-check.C:
		case <-wake:
			woke = true
			size := s.walSize()
			if size < walHoldMultiple*s.walBound || size-start < walBehindBytes {
				continue
			}
			wake = nil
		}
		if ctx.Err() != nil {
			return next(<-done)
		}
		if !held {
			held = true
			tell("hold")
		}
		if spent == nil {
			spent = s.flow.arm(budget)
		}
		release, err := s.lockSlice(ctx, spent)
		s.flow.disarm()
		spent = nil
		if err != nil {
			return next(<-done)
		}
		tell("turn")
		check.Reset(walTurnHold)
		select {
		case c := <-done:
			release()
			tell("free")
			return next(c)
		case <-check.C:
			spent = s.flow.arm(budget)
			release()
			tell("free")
			check.Reset(0)
		}
	}
}

// lockSlice takes the writer turn for the next slice of a held pass. It
// queues as a foreground writer, behind every writer then waiting, until the
// commits admitted since the last slice spend their frame budget, signalled
// by spent; it then leaves the queue and takes the next turn at writerSlice,
// ahead of the writers, who keep their arrival order. A commit larger than
// the budget is still admitted, alone, so no writer waits for a turn that
// never comes.
func (s *Store[T]) lockSlice(ctx context.Context, spent <-chan struct{}) (func(), error) {
	// Leave the foreground queue once the budget is spent.
	queued, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-spent:
			cancel()
		case <-queued.Done():
		}
	}()
	release, err := s.writeMtx.Lock(queued, writerForeground)
	if err == nil || ctx.Err() != nil {
		return release, err
	}
	return s.writeMtx.Lock(ctx, writerSlice)
}

// sliceFrames is the frame budget of the commits a held pass admits between
// two slices: the frames the last pass's copy rate moves in one walTurnHold,
// so the writers together add no more than the passes copy, or walGapFrames
// before a pass has measured the rate, and at least one. The rate includes
// each pass's fixed cost, so the budget errs small. Only the checkpointer
// calls it, between passes.
func (s *Store[T]) sliceFrames() int64 {
	if s.copyFrame <= 0 {
		return walGapFrames
	}
	return max(1, int64(walTurnHold/s.copyFrame))
}

// writeBackLog and writeBackDB write the dirty pages of the log and of the
// database file back outside the writer turn. SQLite flushes the log at the
// start of each pass that copies, and the database file at the end of one
// that leaves no frame to copy, which beside a running writer happens only
// under the turn, in the pass that closes the gap. A flush forces a journal
// commit on ext4 that writes every page dirtied since the last: about a second
// on forge for a log of 150 MiB, which no deadline interrupts, and a writer
// appending to the log waits behind it. Written back around each pass first,
// those flushes find little left: the turn waits 25 ms where it waited 0.8 to
// 1 s. The write-back forces no journal commit and reorders no write: SQLite
// advances the backfill mark only after its own flushes.
//
// SQLite takes fcntl locks on the database file and on its -shm file, never
// on the log, so writeBackLog opens and closes the log each pass, while
// writeBackDB writes through the descriptor of the Store's hold.
func (s *Store[T]) writeBackLog() {
	// Open the log and write its dirty pages back.
	file, err := s.dbFile()
	if err != nil || file == "" || diskless {
		return
	}
	f, err := os.OpenFile(file+"-wal", os.O_RDWR, 0)
	if err != nil {
		return
	}
	_ = writeBackFile(f)
	_ = f.Close()
}

// writeBackDB writes the database file's dirty pages back through the
// descriptor of the Store's hold.
func (s *Store[T]) writeBackDB() {
	// Write back the file SQLite reports, a database in memory having none.
	file, err := s.dbFile()
	if err != nil || file == "" || diskless {
		return
	}
	s.hold.writeBack(file)
}

// _ is a type assertion
var (
	_ kvtx.Store              = (*Store[SQLiteDriverConfig])(nil)
	_ kvtx.OrderedCommitStore = (*Store[SQLiteDriverConfig])(nil)
	_ kvtx.AtomicCommitStore  = (*Store[SQLiteDriverConfig])(nil)
	_ kvtx.WriterQueueStore   = (*Store[SQLiteDriverConfig])(nil)
)
