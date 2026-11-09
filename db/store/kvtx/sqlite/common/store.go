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
	"github.com/s4wave/spacewave/db/kvtx"
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
// the database file without SQLite's write lock.
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
	// with kvtx.WithBackgroundWriteContext waits behind every waiting
	// foreground writer, and each priority goes in arrival order. Without it
	// a burst of background writes, such as upload marker clears, evictions
	// or cache fills, queues ahead of a player's write and sets its wait.
	writeMtx csync.PrioMutex
	// writersWaiting counts write transactions queued on writeMtx.
	writersWaiting atomic.Int32
	// stats counts the writer turn waits and checkpoints for Stats.
	stats storeStats

	// flusher makes ordered commits durable by flushing the WAL.
	flusher *kvtx.OrderedFlusher
	// walBound is the WAL size past which a complete checkpoint truncates it.
	walBound int64

	// fileMtx guards the fields below.
	fileMtx sync.Mutex
	// file is the main database file SQLite reports, read on first use; the
	// WAL is beside it.
	file string
	// fileRead marks file as read.
	fileRead bool

	// checkpointWake wakes the checkpointer after a commit.
	checkpointWake chan struct{}
	// stopCheckpoints ends the checkpointer.
	stopCheckpoints context.CancelFunc
	// checkpointsDone is closed when the checkpointer has returned.
	checkpointsDone chan struct{}

	// The checkpointer alone reads and writes the fields below.
	//
	// vacuumChecked marks vacuumOff as read from the file.
	vacuumChecked bool
	// vacuumOff marks a file without incremental auto-vacuum, whose free
	// pages stay in the file for later writes.
	vacuumOff bool
	// vacuumPages is how many pages the next incremental vacuum step frees.
	vacuumPages int64
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

// truncateBusyTimeout bounds how long the checkpoint that truncates the WAL
// waits for readers of the old log, while it holds SQLite's write lock.
const truncateBusyTimeout = 100 * time.Millisecond

// NewStore constructs a new key-value store from a SQLite database.
func NewStore[T SQLiteDriverConfig](db *sql.DB, table string, config T) (*Store[T], error) {
	return newStore(db, table, config, DefaultWALCheckpointBytes)
}

// newStore is NewStore with the WAL bound.
func newStore[T SQLiteDriverConfig](db *sql.DB, table string, config T, walBound int64) (*Store[T], error) {
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
		checkpointWake:  make(chan struct{}, 1),
		stopCheckpoints: stop,
		checkpointsDone: make(chan struct{}),
	}
	if s.snapshotReads {
		s.readTurns = newReadTurns()
	}
	s.flusher = kvtx.NewOrderedFlusher(s.flushWAL, SyncDeadline)
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

	// Open the pool through a connector that configures every connection.
	// The DSN sets WAL mode where the driver reads it, before any statement.
	db, err := openPool(config, path, pragmas)
	if err != nil {
		return nil, err
	}
	if poolConfigurator, ok := any(config).(SQLiteDriverPoolConfigurator); ok {
		poolConfigurator.ConfigureDBPool(db)
	}

	// Open a reader on the table as the file's writer left it.
	if pragmas.ReadOnly {
		return openReadOnly(ctx, db, table, config)
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
	return openDB(ctx, db, table, pragmas.WithoutRowID, walBound(pragmas), config)
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
// restarts it.
func openPool[T SQLiteDriverConfig](config T, path string, pragmas Pragmas) (*sql.DB, error) {
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
	if pragmas.FullSync {
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
	if pragmas.MmapSize != 0 {
		connPragmas = append(connPragmas, "PRAGMA mmap_size="+strconv.FormatInt(pragmas.MmapSize, 10))
	}
	if pragmas.TempStore != 0 {
		connPragmas = append(connPragmas, "PRAGMA temp_store="+strconv.FormatInt(int64(pragmas.TempStore), 10))
	}

	// Open the pool through the configuring connector.
	connector, err := NewPragmaConnector(drv, dsn, connPragmas)
	if err != nil {
		return nil, err
	}

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
func OpenDB[T SQLiteDriverConfig](ctx context.Context, db *sql.DB, table string, config T) (*Store[T], error) {
	return openDB(ctx, db, table, false, DefaultWALCheckpointBytes, config)
}

// openDB is OpenDB with the shape of a table it creates and the WAL bound.
func openDB[T SQLiteDriverConfig](ctx context.Context, db *sql.DB, table string, withoutRowID bool, walBound int64, config T) (*Store[T], error) {
	// Bind the validated table to its supplied database pool.
	store, err := newStore(db, table, config, walBound)
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
func openReadOnly[T SQLiteDriverConfig](ctx context.Context, db *sql.DB, table string, config T) (*Store[T], error) {
	// Bind the pool, then require the table, which a reader must not create.
	store, err := newStore(db, table, config, DefaultWALCheckpointBytes)
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
	// kvtx.WithBackgroundWriteContext.
	writerBackground = iota - 1
	// writerForeground is the priority of every other write transaction.
	writerForeground
)

// NewTransaction returns a new transaction against the store. A write
// transaction waits for the writer turn until ctx ends, behind every waiting
// foreground writer when ctx carries kvtx.WithBackgroundWriteContext.
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
	if kvtx.IsBackgroundWriteContext(ctx) {
		prio = writerBackground
	}
	s.writersWaiting.Add(1)
	queued := time.Now()
	release, err := s.writeMtx.Lock(ctx, prio)
	s.writersWaiting.Add(-1)
	if err != nil {
		return nil, err
	}
	s.stats.addWait(prio, time.Since(queued))

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
	return newWriteTx(conn, s.queries, s.flusher, s.wakeCheckpoint, release), nil
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
	openStores.Delete(s)
	s.stopCheckpoints()
	<-s.checkpointsDone
	return errors.Join(s.flusher.Close(), s.db.Close())
}

// flushWAL makes every commit the WAL holds durable through the driver.
func (s *Store[T]) flushWAL() error {
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
// checkpoint never takes SQLite's write lock, so no write waits on it. Once a
// passive checkpoint has copied all of the log, the next writer that finds no
// reader on it starts the log over and trims it to the bound. The checkpointer
// takes the write lock only for a log past walRestartFactor times its bound,
// which readers kept from restarting: a restarting checkpoint holds the lock
// through the database file's fsync, 2 to 15 s on a busy disk, past every
// writer's busy wait.
func (s *Store[T]) runCheckpoints(ctx context.Context) {
	defer close(s.checkpointsDone)
	for {
		// Wait for a commit, then let commits gather for one interval.
		select {
		case <-ctx.Done():
			return
		case <-s.checkpointWake:
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(CheckpointInterval):
		}

		// Copy the WAL into the database file; a failure retries after the
		// next commit.
		var busy, frames, copied int
		start := time.Now()
		err := s.db.QueryRowContext(ctx, "PRAGMA wal_checkpoint(PASSIVE)").Scan(&busy, &frames, &copied)
		s.stats.addPassive(time.Since(start))

		// Restart a fully copied WAL far past its bound.
		if err == nil && frames > 0 && copied >= frames && s.walPastBound(walRestartFactor) {
			s.truncateWAL(ctx)
		}

		// Hand one step of free pages back to the filesystem.
		s.vacuumStep(ctx)
	}
}

// Incremental vacuum steps. The checkpointer frees at most one step per
// round, so a file with many free pages shrinks over many rounds, each
// holding SQLite's write lock briefly, instead of in one long VACUUM.
const (
	// vacuumKeepPages is how many free pages the file keeps for later
	// writes before the checkpointer frees more.
	vacuumKeepPages = 1024
	// vacuumMinPages and vacuumMaxPages bound one step.
	vacuumMinPages = 64
	vacuumMaxPages = 16384
	// vacuumStepTarget is the time a step aims for: a step slower than
	// twice it halves the next, one faster than half of it doubles the next.
	vacuumStepTarget = 100 * time.Millisecond
	// vacuumTurnWait bounds how long a step waits for the writer turn
	// before it gives the round up.
	vacuumTurnWait = time.Second
)

// vacuumStep frees up to vacuumPages free pages past vacuumKeepPages from the
// end of the file, in one write transaction taken behind every waiting
// foreground writer. It does nothing on a file without incremental
// auto-vacuum. The freed pages leave the file at the next checkpoint that
// copies the whole log, so a step that freed pages wakes the checkpointer
// for the next round.
func (s *Store[T]) vacuumStep(ctx context.Context) {
	// Read the file's auto-vacuum mode once: 2 is INCREMENTAL.
	if s.vacuumOff {
		return
	}
	if !s.vacuumChecked {
		var mode int
		if err := s.db.QueryRowContext(ctx, "PRAGMA auto_vacuum").Scan(&mode); err != nil {
			return
		}
		s.vacuumChecked, s.vacuumOff = true, mode != 2
		if s.vacuumOff {
			return
		}
		s.vacuumPages = vacuumMinPages
	}

	// Count the free pages without the writer turn.
	var free int64
	if err := s.db.QueryRowContext(ctx, "PRAGMA freelist_count").Scan(&free); err != nil || free <= vacuumKeepPages {
		return
	}

	// Take the writer turn behind every foreground writer, giving the round
	// up when the turn does not come within vacuumTurnWait.
	turnCtx, cancel := context.WithTimeout(ctx, vacuumTurnWait)
	release, err := s.writeMtx.Lock(turnCtx, writerBackground)
	cancel()
	if err != nil {
		return
	}
	defer release()

	// Free one step. Each freed page is a result row, so the rows are read
	// to the end: a driver that steps the statement once frees one page.
	pages := min(free-vacuumKeepPages, s.vacuumPages)
	start := time.Now()
	rows, err := s.db.QueryContext(ctx, "PRAGMA incremental_vacuum("+strconv.FormatInt(pages, 10)+")")
	if err == nil {
		for rows.Next() {
		}
		err = errors.Join(rows.Err(), rows.Close())
	}
	took := time.Since(start)
	s.stats.addVacuum(pages, took)
	if err != nil {
		return
	}

	// Size the next step to the time this one held the lock, and come back
	// next round while free pages remain.
	switch {
	case took > 2*vacuumStepTarget:
		s.vacuumPages = max(s.vacuumPages/2, vacuumMinPages)
	case took < vacuumStepTarget/2:
		s.vacuumPages = min(s.vacuumPages*2, vacuumMaxPages)
	}
	s.wakeCheckpoint()
}

// walRestartFactor is how many times its bound a fully copied WAL grows before
// the checkpointer restarts it under the write lock.
const walRestartFactor = 4

// walPastBound reports whether the WAL file on this host is larger than factor
// times its bound.
func (s *Store[T]) walPastBound(factor int64) bool {
	file, err := s.dbFile()
	if err != nil || file == "" {
		return false
	}
	info, err := os.Stat(file + "-wal")
	return err == nil && info.Size() > factor*s.walBound
}

// truncateWAL restarts a WAL with nothing left to copy, so the next commit
// writes from the start of the file and trims it to the bound. The checkpoint
// holds SQLite's write lock while it waits for readers of the old log, so it
// runs on a connection of its own that waits at most truncateBusyTimeout, then
// gives up until a later round. It restarts rather than truncates: truncating
// a log of hundreds of MiB to zero freed and synced the whole file under the
// write lock, 7 to 10 s on a busy disk, past every writer's busy wait, and the
// file would only grow back to the bound.
func (s *Store[T]) truncateWAL(ctx context.Context) {
	// Take a connection and shorten its busy wait.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout="+strconv.FormatInt(truncateBusyTimeout.Milliseconds(), 10)); err != nil {
		return
	}

	// Truncate, then restore the pool's busy wait before the connection
	// returns to it.
	var busy, frames, copied int
	start := time.Now()
	_ = conn.QueryRowContext(ctx, "PRAGMA wal_checkpoint(RESTART)").Scan(&busy, &frames, &copied)
	s.stats.addTruncate(time.Since(start))
	_, _ = conn.ExecContext(context.Background(), "PRAGMA busy_timeout=5000")
}

// _ is a type assertion
var (
	_ kvtx.Store              = (*Store[SQLiteDriverConfig])(nil)
	_ kvtx.OrderedCommitStore = (*Store[SQLiteDriverConfig])(nil)
	_ kvtx.AtomicCommitStore  = (*Store[SQLiteDriverConfig])(nil)
	_ kvtx.WriterQueueStore   = (*Store[SQLiteDriverConfig])(nil)
)
