// Package measure measures a frozen block DAG with a disk frontier and exact
// disk deduplication. No graph-sized collection is retained in memory.
package measure

import (
	"context"
	"database/sql"
	"io"
	"math"
	"os"
	"strconv"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
)

// ErrInvalidExtent marks index metadata or a complete response inconsistent with its extent.
var ErrInvalidExtent = errors.New("invalid measurement extent")

// ErrSourceConfiguration stops a run whose source repeatedly reports missing objects.
var ErrSourceConfiguration = errors.New("measurement source configuration error; start new work after correcting the source")

// Walker owns a resumable scratch database, separate from every input store.
// A block's count and outgoing edges commit together. Repeating an interrupted
// range may download its bytes again, but never counts a block twice.
type Walker struct {
	// DB holds the local frontier, cache, counts and parent edges.
	DB *sql.DB
	// store owns the scratch connection lifecycle.
	store sqlite.Store
	// wal is the scratch database's write-ahead log, which checkpoint bounds.
	wal string
	// Locate returns the frozen index's pack number, offset and encoded length.
	Locate func(context.Context, *block.BlockRef) (uint32, int64, int, bool, error)
	// ReadRange reads a bounded range from the named pack, or stops the walk.
	ReadRange func(context.Context, string, int64, int) ([]byte, error)
	// Local reads the frozen Volume before consulting the pack index.
	Local func(context.Context, *block.BlockRef) (*block.StoredBlock, error)
	// Children reads repaired child lists from the collector snapshot.
	Children func(context.Context, *block.BlockRef) ([]*block.BlockRef, bool, error)
	// Describe returns a type label and typed children, without storing state.
	Describe func(context.Context, *block.BlockRef, []byte, []byte) (string, []Child, error)
	// Inflight is the most frontier range reads outstanding at once; below 1 means 1.
	// ReadRange must then be safe for concurrent calls, including a type lookup's.
	Inflight int

	// types caches the interned id of each type descriptor; the walk sees a handful.
	types map[string]int64
	// sighted is the lowest depth among unmerged sightings, or -1 when none are known.
	sighted int
}

// Open creates or resumes a scratch database. Close releases it. Callers must
// bind immutable input identities with Bind before seeding or resuming.
func Open(path string) (*Walker, error) {
	// Bound SQLite's page cache. Commits run at synchronous=NORMAL: a power
	// loss may roll back recent visits, which resume repeats, and a commit
	// that must survive one, such as a spending reservation, calls Sync.
	store, err := sqlite.OpenWithPragmas(context.Background(), path, "scratch", sqlite.Pragmas{CacheSize: -4096, TempStore: 1})
	if err != nil {
		return nil, err
	}
	db := store.GetDB()
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS inputs(name TEXT PRIMARY KEY, value BLOB NOT NULL);
 CREATE TABLE IF NOT EXISTS packs(num INTEGER PRIMARY KEY,id TEXT NOT NULL,size INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS nodes(
 ref TEXT PRIMARY KEY, type BLOB NOT NULL, depth INTEGER NOT NULL, pack INTEGER NOT NULL,
 off INTEGER NOT NULL, n INTEGER NOT NULL, state INTEGER NOT NULL DEFAULT 0,
 kind TEXT NOT NULL DEFAULT '', bytes INTEGER NOT NULL DEFAULT 0);
 CREATE TABLE IF NOT EXISTS edges(child TEXT NOT NULL,parent TEXT NOT NULL,PRIMARY KEY(child,parent));
 CREATE TABLE IF NOT EXISTS cache(ref TEXT PRIMARY KEY,data BLOB NOT NULL);
 CREATE TABLE IF NOT EXISTS location_holes(ref TEXT PRIMARY KEY,kind TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS locations(ref TEXT PRIMARY KEY,pack INTEGER NOT NULL,n INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS descriptors(ref TEXT NOT NULL,type BLOB NOT NULL,done INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(ref,type));
 CREATE TABLE IF NOT EXISTS sources(ref TEXT PRIMARY KEY,kind TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS failures(ref TEXT PRIMARY KEY,kind TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS types(id INTEGER PRIMARY KEY,type BLOB NOT NULL UNIQUE);
 CREATE TABLE IF NOT EXISTS sightings(child TEXT NOT NULL,parent TEXT NOT NULL,type INTEGER NOT NULL,depth INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS merge_cursor(child TEXT NOT NULL,rid INTEGER NOT NULL);`)
	if err == nil {
		// An interrupted merge keeps the frontier index dropped until it finishes.
		var merging bool
		if err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM merge_cursor)").Scan(&merging); err == nil && !merging {
			_, err = db.Exec(frontierIndex)
		}
	}
	if err != nil {
		store.Close()
		return nil, err
	}
	return &Walker{DB: db, store: store, wal: path + "-wal", types: map[string]int64{}, sighted: -1}, nil
}

// frontierIndex orders pending nodes by level and pack position for next.
const frontierIndex = "CREATE INDEX IF NOT EXISTS frontier ON nodes(state,depth,pack,off)"

// walBound is the WAL size past which the walker checkpoints and truncates it.
var walBound int64 = 256 << 20

// checkpoint bounds the scratch WAL. The store's pool disables SQLite's
// automatic checkpoint, and its background checkpointer wakes only on the
// store's own key-value commits, never on the commits the walker makes
// through DB. Without this the WAL keeps every frame of the walk, and its
// index grows until SQLite runs out of memory. The walker holds the pool's
// one connection, so the truncating checkpoint waits only for an outside
// reader; a busy result leaves the log for the next call.
func (w *Walker) checkpoint(ctx context.Context) error {
	// One stat per frontier run keeps the check off the per-block path.
	info, err := os.Stat(w.wal)
	if os.IsNotExist(err) || (err == nil && info.Size() < walBound) {
		return nil
	}
	if err != nil {
		return err
	}
	var busy, frames, copied int
	return w.DB.QueryRowContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &frames, &copied)
}

// Sync makes every commit so far durable across a power loss. At
// synchronous=NORMAL a commit reaches the write-ahead log without a flush, and
// SQLite recovers the log only up to its first unflushed frame, so flushing
// the log keeps every earlier commit. A checkpoint flushes the log before it
// copies or truncates it, so a commit it moved into the database stays durable.
func (w *Walker) Sync() error {
	// Flush the log file through its own descriptor, which shares the inode.
	f, err := os.OpenFile(w.wal, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	err = f.Sync()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// Close closes the scratch database after its last committed position.
func (w *Walker) Close() error { return w.store.Close() }

// Bind prevents resume with a different head, root snapshot, index or source.
// Input identities are immutable; attempt and byte ledgers use separate names.
func (w *Walker) Bind(ctx context.Context, name string, value []byte) error {
	// Keep the original identity, even after an interrupted setup.
	_, err := w.DB.ExecContext(ctx, "INSERT OR IGNORE INTO inputs VALUES(?,?)", name, value)
	if err != nil {
		return err
	}
	var same bool
	err = w.DB.QueryRowContext(ctx, "SELECT value=? FROM inputs WHERE name=?", value, name).Scan(&same)
	if err != nil {
		return err
	}
	if !same {
		return errors.New("measurement input changed: " + name)
	}
	return nil
}

// AddPack registers a pack from the frozen index without loading a pack table
// into memory. Repeating setup is idempotent.
func (w *Walker) AddPack(ctx context.Context, num uint32, id string, size uint64) error {
	if size > math.MaxInt64 {
		return errors.New("pack length exceeds SQL integer range")
	}
	_, err := w.DB.ExecContext(ctx, "INSERT OR IGNORE INTO packs VALUES(?,?,?)", int64(num), id, int64(size))
	return err
}

// Seed adds a named root; the name is retained as a hole's parent.
func (w *Walker) Seed(ctx context.Context, name string, ref *block.BlockRef, typ []byte) error {
	// Commit local bytes and their root edge together.
	tx, err := w.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = w.enqueue(ctx, tx, Child{Ref: ref, Type: typ}, "root:"+name, 0); err != nil {
		return err
	}
	return tx.Commit()
}

// SeedAll commits a streaming root scan in one scratch transaction.
func (w *Walker) SeedAll(ctx context.Context, scan func(func(string, *block.BlockRef) error) error) error {
	// Bound memory to the root scanner's current seed and amortize the commit.
	tx, err := w.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = scan(func(name string, ref *block.BlockRef) error {
		return w.enqueue(ctx, tx, Child{Ref: ref}, "root:"+name, 0)
	}); err != nil {
		return err
	}
	return tx.Commit()
}

// executor is the common write surface of a database and a transaction.
type executor interface {
	// QueryRowContext reads one bounded scratch record.
	QueryRowContext(context.Context, string, ...any) *sql.Row
	// ExecContext changes only the scratch database.
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// enqueue deduplicates a block by its hash and keeps every parent on disk.
func (w *Walker) enqueue(ctx context.Context, db executor, c Child, parent string, depth int) error {
	// Deduplicate before reading the Volume or index again.
	if c.Ref.GetEmpty() {
		return nil
	}
	key := c.Ref.MarshalString()
	var exists bool
	if err := db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM nodes WHERE ref=?)", key).Scan(&exists); err != nil {
		return err
	}
	var num uint32
	var off int64
	var n int
	if !exists {
		// Cache the local Volume block before falling back to the index.
		stored, err := w.local(ctx, c.Ref, db)
		if errors.Is(err, block.ErrNotFound) {
			err = nil
		}
		if err != nil {
			return err
		}

		// Keep pack membership separate from which source served the bytes.
		var found bool
		num, off, n, found, err = w.Locate(ctx, c.Ref)
		if err == nil && found {
			var size int64
			err = db.QueryRowContext(ctx, "SELECT size FROM packs WHERE num=?", int64(num)).Scan(&size)
			if errors.Is(err, sql.ErrNoRows) || (err == nil && (off < 0 || n <= 0 || off > size-int64(n))) {
				err = ErrInvalidExtent
			}
		}
		if errors.Is(err, ErrInvalidExtent) {
			table := "failures"
			if stored != nil {
				table = "location_holes"
			}
			if _, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO "+table+" VALUES(?,'hole:extent')", key); err != nil {
				return err
			}
			found = false
		}
		if err != nil {
			return err
		}
		if found {
			if _, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO locations VALUES(?,?,?)", key, int64(num), n); err != nil {
				return err
			}
		}
		if !found {
			n = 0
		}
		if stored != nil && n == 0 {
			if err = db.QueryRowContext(ctx, "SELECT length(data) FROM cache WHERE ref=?", key).Scan(&n); err != nil {
				return err
			}
		}
	}

	// Persist one identity and type, upgrading an earlier untyped sighting.
	typ := c.Type
	if typ == nil {
		typ = []byte{}
	}
	_, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO nodes(ref,type,depth,pack,off,n) VALUES(?,?,?,?,?,?)`, key, typ, depth, int64(num), off, n)
	if err != nil {
		return err
	}
	if len(typ) != 0 {
		result, err := db.ExecContext(ctx, "INSERT OR IGNORE INTO descriptors(ref,type) VALUES(?,?)", key, typ)
		if err != nil {
			return err
		}
		added, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if added != 0 {
			if _, err = db.ExecContext(ctx, `UPDATE nodes SET type=?,state=0
 WHERE ref=? AND (kind NOT LIKE 'hole:%' OR EXISTS(SELECT 1 FROM cache WHERE cache.ref=nodes.ref))`, typ, key); err != nil {
				return err
			}
		}
	}

	// Apply an earlier off-frontier judgment before this node can be read.
	if _, err = db.ExecContext(ctx, `UPDATE nodes SET state=1,kind=(SELECT kind FROM failures WHERE failures.ref=nodes.ref)
 WHERE ref=? AND EXISTS(SELECT 1 FROM failures WHERE failures.ref=nodes.ref)`, key); err != nil {
		return err
	}

	// Retain all parents even when a block was already reached.
	_, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO edges VALUES(?,?)", key, parent)
	return err
}

// item is one bounded frontier entry.
type item struct {
	// ref is the encoded hash.
	ref string
	// typ is its serialized type descriptor.
	typ []byte
	// depth is its breadth-first level.
	depth int
	// pack is the frozen index's pack number.
	pack uint32
	// off starts the encoded block in the pack.
	off int64
	// cached reports that the verified encoded block is already on disk.
	cached bool
	// n is the encoded block length, zero for an index hole.
	n int
}

// position is a place in the frontier's order within one level.
type position struct {
	// depth is the level.
	depth int
	// pack is the pack number of the last entry taken.
	pack uint32
	// off is that entry's offset; -1 precedes every entry of pack 0.
	off int64
}

// frontierColumns reads one pending entry, its next unfinished type and
// whether its bytes are cached.
const frontierColumns = `ref,coalesce((SELECT type FROM descriptors d WHERE d.ref=nodes.ref AND d.done=0 LIMIT 1),type),depth,pack,off,n,
 EXISTS(SELECT 1 FROM cache WHERE cache.ref=nodes.ref)`

// next returns one bounded contiguous run of pending entries. With no
// position it starts at the lowest pending level; otherwise it continues the
// position's level after it. The frontier index orders a whole level by pack
// and offset without collecting it, and each half of the continuation seeks
// that index exactly.
func (w *Walker) next(ctx context.Context, from *position) ([]item, error) {
	// Take at most 4096 refs in pack order.
	query := `SELECT ` + frontierColumns + ` FROM nodes WHERE state=0 ORDER BY depth,pack,off LIMIT 4096`
	var args []any
	if from != nil {
		query = `SELECT * FROM (SELECT ` + frontierColumns + ` FROM nodes INDEXED BY frontier WHERE state=0 AND depth=?1 AND pack=?2 AND off>?3 ORDER BY off LIMIT 4096)
 UNION ALL SELECT * FROM (SELECT ` + frontierColumns + ` FROM nodes INDEXED BY frontier WHERE state=0 AND depth=?1 AND pack>?2 ORDER BY pack,off LIMIT 4096)
 LIMIT 4096`
		args = []any{from.depth, int64(from.pack), from.off}
	}

	// Cut the rows into the first run of nearby entries in one pack.
	rows, err := w.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var run []item
	var end int64
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.ref, &it.typ, &it.depth, &it.pack, &it.off, &it.n, &it.cached); err != nil {
			return nil, err
		}
		if len(run) != 0 && (it.cached != run[0].cached || it.depth != run[0].depth ||
			it.pack != run[0].pack || it.n == 0 || it.off > end+(512<<10) ||
			it.off+int64(it.n)-run[0].off > 4<<20) {
			break
		}
		run = append(run, it)
		end = max(end, it.off+int64(it.n))
		if it.n == 0 {
			break
		}
	}
	return run, rows.Err()
}

// Run measures until the frontier drains or any read fails. Transport errors
// leave the outstanding runs pending; no error is silently converted into a hole.
func (w *Walker) Run(ctx context.Context) error {
	// A configuration stop survives process replacement without spending another request.
	if err := w.checkSource(ctx); err != nil {
		return err
	}

	// Finish an interrupted merge first: next needs the frontier index it rebuilds.
	var merging bool
	if err := w.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM merge_cursor), coalesce((SELECT min(depth) FROM sightings),-1)").Scan(&merging, &w.sighted); err != nil {
		return err
	}
	if merging {
		if err := w.merge(ctx); err != nil {
			return err
		}
	}

	// Walk one level pass, or one merge, per iteration.
	for {
		// Bound the log the previous pass's commits added.
		if err := w.checkpoint(ctx); err != nil {
			return err
		}

		// Read the lowest pending run, outside any scratch transaction.
		run, err := w.next(ctx, nil)
		if err != nil {
			return err
		}

		// File a level's sightings before any node at or below their depth is read.
		if w.sighted >= 0 && (len(run) == 0 || run[0].depth >= w.sighted) {
			if err = w.merge(ctx); err != nil {
				return err
			}
			continue
		}
		if len(run) == 0 {
			return nil
		}
		if err = w.pass(ctx, run); err != nil {
			return err
		}
	}
}

// pass walks one level from its first run, keeping up to Inflight range reads
// outstanding. The calling goroutine owns the scratch database: fetchers only
// call ReadRange, and their results commit here in arrival order. Any order
// within a level builds the same graph, because a visit appends only
// sightings for the next level, which merge files after the level ends. A
// position past each run taken keeps a run from going out twice. An entry the
// pass leaves pending, such as one with a second reaching type or one whose
// read was still out when the pass stopped, keeps state 0, so the next pass
// or a resume takes it again; a resume may download its bytes again.
func (w *Walker) pass(ctx context.Context, run []item) error {
	// Cancel the outstanding reads when any result stops the pass.
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	type fetched struct {
		run  []item
		data []byte
		err  error
	}
	results := make(chan fetched, max(1, w.Inflight))
	from := &position{depth: run[0].depth, off: -1}
	inflight, ended := 0, false
	var failed error
	for {
		// Dispatch until every lane is busy or the level has no run past the position.
		for failed == nil && !ended && inflight < cap(results) {
			if run == nil {
				if run, failed = w.next(ctx, from); failed != nil {
					break
				}
				if len(run) == 0 {
					ended = true
					break
				}
			}
			r := run
			run = nil
			last := r[len(r)-1]
			from = &position{depth: last.depth, pack: last.pack, off: last.off}
			id, size, err := w.prepare(ctx, r)
			if err != nil {
				failed = err
				break
			}
			if size == 0 {
				continue
			}
			inflight++
			go func() {
				data, err := w.ReadRange(ctx, id, r[0].off, size)
				results <- fetched{run: r, data: data, err: err}
			}()
		}
		if failed != nil {
			cancel(failed)
		}
		if inflight == 0 {
			return failed
		}

		// Settle one read; after a failure, only wait for the rest to return.
		f := <-results
		inflight--
		if failed != nil {
			continue
		}
		failed = w.settle(ctx, f.run, f.data, f.err)
		if failed == nil {
			failed = w.checkpoint(ctx)
		}
	}
}

// prepare settles a run that needs no read: an index hole, cached bytes, or
// extents the frozen index already contradicts. Otherwise it returns the pack
// ID and the length of the one range read that covers the run.
func (w *Walker) prepare(ctx context.Context, run []item) (string, int, error) {
	// An unlocated entry is a hole until a later sighting supplies its bytes.
	if run[0].n == 0 {
		_, err := w.DB.ExecContext(ctx, "UPDATE nodes SET state=1,kind='hole:index' WHERE ref=?", run[0].ref)
		return "", 0, err
	}
	if run[0].cached {
		for _, it := range run {
			if err := w.visit(ctx, it); err != nil {
				return "", 0, err
			}
		}
		return "", 0, nil
	}
	var id string
	var size int64
	if err := w.DB.QueryRowContext(ctx, "SELECT id,size FROM packs WHERE num=?", int64(run[0].pack)).Scan(&id, &size); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			for _, it := range run {
				if err = w.hole(ctx, it.ref, "hole:extent"); err != nil {
					return "", 0, err
				}
			}
			return "", 0, nil
		}
		return "", 0, err
	}

	// Remove only the invalid extents, preserving neighboring valid blocks.
	var end int64
	invalid := false
	for _, it := range run {
		if it.off < 0 || it.n <= 0 || it.off > size-int64(it.n) {
			if err := w.hole(ctx, it.ref, "hole:extent"); err != nil {
				return "", 0, err
			}
			invalid = true
		}
		end = max(end, it.off+int64(it.n))
	}
	if invalid {
		return "", 0, nil
	}
	return id, int(end - run[0].off), nil
}

// settle commits one completed range read: a source judgment becomes holes,
// and verified bytes are cached and visited. A transport failure is returned
// with the run still pending.
func (w *Walker) settle(ctx context.Context, run []item, data []byte, err error) error {
	// Classify only completed source judgments.
	if errors.Is(err, ErrSourceConfiguration) {
		refs := make([]string, 0, len(run))
		for _, it := range run {
			refs = append(refs, it.ref)
		}
		return w.stopSource(ctx, refs...)
	}
	if errors.Is(err, block.ErrNotFound) || errors.Is(err, ErrInvalidExtent) {
		kind := "hole:source"
		if errors.Is(err, ErrInvalidExtent) {
			kind = "hole:extent"
		}
		for _, it := range run {
			if err = w.hole(ctx, it.ref, kind); err != nil {
				return err
			}
		}
		return nil
	}
	if err != nil {
		return err
	}

	// Cache each verified encoded block for type lookups and crash resume.
	for _, it := range run {
		at := it.off - run[0].off
		if at < 0 || at+int64(it.n) > int64(len(data)) {
			if err = w.hole(ctx, it.ref, "hole:extent"); err != nil {
				return err
			}
			continue
		}
		ref, err := block.UnmarshalBlockRefB58(it.ref)
		if err != nil {
			return err
		}
		raw := data[at : at+int64(it.n)]
		if _, err = decode(ref, raw); err != nil {
			if err = w.hole(ctx, it.ref, "hole:corrupt"); err != nil {
				return err
			}
			continue
		}
		if err = w.cache(ctx, it.ref, raw, "pack-index"); err != nil {
			return err
		}
	}

	// Commit a block's result together with its deduplicated children.
	for _, it := range run {
		if err = w.visit(ctx, it); err != nil {
			return err
		}
	}
	return nil
}

// decode verifies the payload before any child list is followed.
func decode(ref *block.BlockRef, raw []byte) (*block.StoredBlock, error) {
	stored, err := block.DecodeBlockObject(raw)
	if err != nil {
		return nil, err
	}
	if err := ref.VerifyData(stored.Data, false); err != nil {
		return nil, err
	}
	return stored, nil
}

// GetStoredBlock serves a type lookup from local cache, fetching once when
// needed. It never marks an off-frontier lookup as live or changes its count.
func (w *Walker) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Reuse bytes a frontier read or earlier type lookup already fetched.
	var raw []byte
	err := w.DB.QueryRowContext(ctx, "SELECT data FROM cache WHERE ref=?", ref.MarshalString()).Scan(&raw)
	if err == nil {
		return decode(ref, raw)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// Reuse a terminal corrupt or unavailable judgment without another GET.
	var failed bool
	if err = w.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM failures WHERE ref=?
 UNION ALL SELECT 1 FROM nodes WHERE ref=? AND kind LIKE 'hole:%')`, ref.MarshalString(), ref.MarshalString()).Scan(&failed); err != nil {
		return nil, err
	}
	if failed {
		return nil, block.ErrNotFound
	}
	stored, err := w.local(ctx, ref, w.DB)
	if err != nil || stored != nil {
		return stored, err
	}

	// Read only the indexed extent and retain it on disk for the later walk.
	num, off, n, found, err := w.Locate(ctx, ref)
	if errors.Is(err, ErrInvalidExtent) {
		return nil, w.lookupHole(ctx, ref, "hole:extent")
	}
	if err != nil || !found {
		return nil, err
	}
	var id string
	var size int64
	if err = w.DB.QueryRowContext(ctx, "SELECT id,size FROM packs WHERE num=?", int64(num)).Scan(&id, &size); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, w.lookupHole(ctx, ref, "hole:extent")
		}
		return nil, err
	}

	// Persist invalid extents without issuing a request.
	if off < 0 || n <= 0 || off > size-int64(n) {
		return nil, w.lookupHole(ctx, ref, "hole:extent")
	}

	// Verify a newly downloaded extent before making it reusable.
	if err = w.checkSource(ctx); err != nil {
		return nil, err
	}
	raw, err = w.ReadRange(ctx, id, off, n)

	// Classify only completed source judgments; transport failures stay pending.
	if errors.Is(err, ErrSourceConfiguration) {
		return nil, w.stopSource(ctx, ref.MarshalString())
	}
	if errors.Is(err, ErrInvalidExtent) {
		return nil, w.lookupHole(ctx, ref, "hole:extent")
	}
	if errors.Is(err, block.ErrNotFound) {
		return nil, w.lookupHole(ctx, ref, "hole:source")
	}
	if err != nil {
		return nil, err
	}

	// Verify the payload before it enters the reusable cache.
	stored, err = decode(ref, raw)
	if err != nil {
		return nil, w.lookupHole(ctx, ref, "hole:corrupt")
	}
	err = w.cache(ctx, ref.MarshalString(), raw, "pack-index")
	return stored, err
}

// visit describes one cached block and atomically commits its graph edges.
func (w *Walker) visit(ctx context.Context, it item) error {
	// A terminal judgment survives typed sightings and every resume.
	var terminal bool
	if err := w.DB.QueryRowContext(ctx, "SELECT state<>0 FROM nodes WHERE ref=?", it.ref).Scan(&terminal); err != nil {
		return err
	}
	if terminal {
		return nil
	}

	// Decode its type outside the write transaction: type lookup may read cache.
	ref, err := block.UnmarshalBlockRefB58(it.ref)
	if err != nil {
		return err
	}
	stored, err := w.GetStoredBlock(ctx, ref)
	if errors.Is(err, block.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	// Resolve typed children before consulting repaired collector lists.
	kind, kids, err := w.Describe(ctx, ref, stored.Data, it.typ)
	if err != nil {
		return err
	}
	if !stored.RefsKnown && w.Children != nil {
		refs, found, err := w.Children(ctx, ref)
		if err != nil {
			return err
		}
		if found {
			stored.Refs = refs
			stored.RefsKnown = true
		}
	}
	if !stored.RefsKnown && kind == "untyped" {
		kind = "hole:refs-unknown"
	}

	// Preserve the stored closure even when a type is unknown or its list differs.
	kids = kids[:len(kids):len(kids)]
	for _, kid := range stored.Refs {
		kids = append(kids, Child{Ref: kid})
	}
	ids := make([]int64, len(kids))
	for i, kid := range kids {
		if ids[i], err = w.typeID(ctx, kid.Type); err != nil {
			return err
		}
	}

	// Append the children to the sighting log; merge files them in ref order.
	tx, err := w.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for i, kid := range kids {
		if kid.Ref.GetEmpty() {
			continue
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO sightings VALUES(?,?,?,?)", kid.Ref.MarshalString(), it.ref, ids[i], it.depth+1); err != nil {
			return err
		}
	}

	// Finish only this reaching context; other roots may type the same ref differently.
	if _, err = tx.ExecContext(ctx, "UPDATE descriptors SET done=1 WHERE ref=? AND type=?", it.ref, it.typ); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `UPDATE nodes SET state=CASE WHEN EXISTS(SELECT 1 FROM descriptors d WHERE d.ref=nodes.ref AND d.done=0) THEN 0 ELSE 1 END,
 kind=CASE WHEN kind LIKE 'hole:%' THEN kind ELSE ? END,bytes=? WHERE ref=?`, kind, len(stored.Data), it.ref)
	if err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if len(kids) != 0 && (w.sighted < 0 || it.depth+1 < w.sighted) {
		w.sighted = it.depth + 1
	}
	return nil
}

// typeID interns a type descriptor in its own commit, so a rolled-back visit
// never leaves a sighting that names a missing type. Zero means untyped.
func (w *Walker) typeID(ctx context.Context, typ []byte) (int64, error) {
	// Serve the untyped id and known types without a statement.
	if len(typ) == 0 {
		return 0, nil
	}
	if id, ok := w.types[string(typ)]; ok {
		return id, nil
	}

	// Intern a new type once and keep its id for the rest of the process.
	if _, err := w.DB.ExecContext(ctx, "INSERT OR IGNORE INTO types(type) VALUES(?)", typ); err != nil {
		return 0, err
	}
	var id int64
	if err := w.DB.QueryRowContext(ctx, "SELECT id FROM types WHERE type=?", typ).Scan(&id); err != nil {
		return 0, err
	}
	w.types[string(typ)] = id
	return id, nil
}

// mergeChunk bounds the sightings one merge commit files.
var mergeChunk = 4096

// merge files every logged sighting into the graph in ref order. Filing a
// child dirties the same sorted leaves of nodes, edges, descriptors and
// locations as its neighbors, so each page is written once per chunk instead
// of once per child. The frontier index, ordered by pack, would take one
// random leaf per new node, so it is dropped during the merge and rebuilt in
// one sorted pass. A cursor commits with each chunk: an interrupted merge
// resumes after the last filed sighting, and enqueue is idempotent, so a
// repeated chunk changes nothing.
func (w *Walker) merge(ctx context.Context) error {
	// Begin once: the sort index, the cursor and the dropped frontier commit together.
	tx, err := w.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO merge_cursor SELECT '',0 WHERE NOT EXISTS(SELECT 1 FROM merge_cursor);
 DROP INDEX IF EXISTS frontier;
 CREATE INDEX IF NOT EXISTS sightings_order ON sightings(child)`); err != nil {
		return err
	}
	var cursor string
	var rid int64
	if err = tx.QueryRowContext(ctx, "SELECT child,rid FROM merge_cursor").Scan(&cursor, &rid); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}

	// Stream the log past the cursor, one bounded chunk at a time.
	type sighting struct {
		rid    int64
		child  string
		parent string
		typ    []byte
		depth  int
	}
	for {
		// Read one chunk past the cursor, then release the only connection. The
		// two halves each seek the index exactly, even past a child with many parents.
		rows, err := w.DB.QueryContext(ctx, `SELECT s.rid,s.child,s.parent,coalesce(t.type,x''),s.depth FROM (
 SELECT * FROM (SELECT rowid AS rid,* FROM sightings INDEXED BY sightings_order WHERE child=?1 AND rowid>?2 ORDER BY rowid LIMIT ?3)
 UNION ALL SELECT * FROM (SELECT rowid AS rid,* FROM sightings INDEXED BY sightings_order WHERE child>?1 ORDER BY child,rowid LIMIT ?3)
 LIMIT ?3) s LEFT JOIN types t ON t.id=s.type`, cursor, rid, mergeChunk)
		if err != nil {
			return err
		}
		var chunk []sighting
		for rows.Next() {
			var s sighting
			if err = rows.Scan(&s.rid, &s.child, &s.parent, &s.typ, &s.depth); err != nil {
				break
			}
			chunk = append(chunk, s)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return err
		}
		if len(chunk) == 0 {
			break
		}

		// File the chunk and advance the cursor in one commit.
		tx, err := w.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		for _, s := range chunk {
			ref, err := block.UnmarshalBlockRefB58(s.child)
			if err == nil {
				err = w.enqueue(ctx, tx, Child{Ref: ref, Type: s.typ}, s.parent, s.depth)
			}
			if err != nil {
				tx.Rollback()
				return err
			}
		}
		last := chunk[len(chunk)-1]
		if _, err = tx.ExecContext(ctx, "UPDATE merge_cursor SET child=?,rid=?", last.child, last.rid); err != nil {
			tx.Rollback()
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
		cursor, rid = last.child, last.rid
		if err = w.checkpoint(ctx); err != nil {
			return err
		}
	}

	// Finish in idempotent steps; the cursor row goes last, after the frontier exists.
	if _, err = w.DB.ExecContext(ctx, "DELETE FROM sightings; DROP INDEX IF EXISTS sightings_order"); err != nil {
		return err
	}
	if _, err = w.DB.ExecContext(ctx, frontierIndex); err != nil {
		return err
	}
	if _, err = w.DB.ExecContext(ctx, "DELETE FROM merge_cursor"); err != nil {
		return err
	}
	w.sighted = -1
	return w.checkpoint(ctx)
}

// Report streams stable tab-separated totals, histograms, pack live fractions
// and unresolved holes with all parents. Index holes are not confirmed B2 loss.
func (w *Walker) Report(ctx context.Context, out io.Writer) error {
	queries := []string{
		`SELECT name,value FROM inputs WHERE name IN ('reserved-bytes','reserved-requests')`,
		`SELECT 'seed-kind',CASE WHEN parent='root:HEAD' THEN 'HEAD'
 WHEN parent LIKE 'root:retained/%' THEN 'retained' ELSE 'collector' END,count(*)
 FROM edges WHERE parent LIKE 'root:%' GROUP BY 2`,
		`SELECT 'collector-kind',substr(owner,1,CASE WHEN instr(owner,':')=0 THEN length(owner) ELSE instr(owner,':')-1 END),count(*)
 FROM (SELECT substr(parent,length('root:collector/')+1) AS owner FROM edges WHERE parent LIKE 'root:collector/%') GROUP BY 2`,
		`SELECT 'seeds',parent,count(*) FROM edges WHERE parent LIKE 'root:%' GROUP BY parent`,
		`SELECT 'source',s.kind,count(*) FROM sources s JOIN nodes n ON n.ref=s.ref WHERE n.state=1 GROUP BY s.kind`,
		`SELECT 'holes:pack-location',count(*) FROM location_holes`,
		`SELECT h.kind,h.ref,e.parent FROM location_holes h JOIN edges e ON h.ref=e.child`,
		`SELECT 'holes',kind,count(*) FROM nodes WHERE kind LIKE 'hole:%' GROUP BY kind`,
		`SELECT 'total',count(*),coalesce(sum(bytes),0) FROM nodes WHERE state=1 AND kind NOT LIKE 'hole:%'`,
		`SELECT 'pending',count(*),0 FROM nodes WHERE state=0`,
		`SELECT 'pending:sightings',count(*),0 FROM sightings`,
		`SELECT 'holes:index-unresolved',count(*),0 FROM nodes WHERE kind='hole:index'`,
		`SELECT 'holes:source-unavailable',count(*),0 FROM nodes WHERE kind='hole:source'`,
		`SELECT 'histogram',kind,bin,count(*),sum(bytes),sum(n) FROM
 (SELECT kind,bytes,n,CASE WHEN bytes<1024 THEN 0 WHEN bytes<4096 THEN 1024
 WHEN bytes<16384 THEN 4096 WHEN bytes<65536 THEN 16384
 WHEN bytes<1048576 THEN 65536 ELSE 1048576 END AS bin
 FROM nodes WHERE state=1 AND kind NOT LIKE 'hole:%') GROUP BY kind,bin`,
		`SELECT 'pack',p.id,p.size,count(*),sum(l.n),1.0*sum(l.n)/p.size
 FROM nodes n JOIN locations l ON l.ref=n.ref JOIN packs p ON p.num=l.pack
 WHERE n.state=1 AND n.kind NOT LIKE 'hole:%' GROUP BY p.num`,
		`SELECT CASE WHEN n.kind='hole:index' THEN 'hole:index-unresolved' WHEN n.kind='hole:source' THEN 'hole:source-unavailable' ELSE n.kind END,n.ref,e.parent
 FROM nodes n JOIN edges e ON e.child=n.ref WHERE n.kind LIKE 'hole:%'`,
	}
	for _, query := range queries {
		// Stream SQL's bounded aggregate rows directly into the report.
		rows, err := w.DB.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return err
		}
		values := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		for rows.Next() {
			if err = rows.Scan(ptrs...); err != nil {
				break
			}
			for i, v := range values {
				if i != 0 {
					_, err = io.WriteString(out, "\t")
					if err != nil {
						break
					}
				}
				var s string
				switch x := v.(type) {
				case string:
					s = x
				case int64:
					s = strconv.FormatInt(x, 10)
				case float64:
					s = strconv.FormatFloat(x, 'g', -1, 64)
				case []byte:
					s = string(x)
				case nil:
					s = "0"
				}
				_, err = io.WriteString(out, s)
				if err != nil {
					break
				}
			}
			if err != nil {
				break
			}
			_, err = io.WriteString(out, "\n")
			if err != nil {
				break
			}
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// local copies a Volume block into scratch, without opening or upgrading a Volume.
func (w *Walker) local(ctx context.Context, ref *block.BlockRef, db executor) (*block.StoredBlock, error) {
	// Read at most the one local block requested by the frontier or typer.
	if w.Local == nil {
		return nil, nil
	}
	stored, err := w.Local(ctx, ref)
	if err != nil || stored == nil {
		return stored, err
	}
	if err = ref.VerifyData(stored.Data, false); err != nil {
		if _, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO failures VALUES(?,'hole:corrupt')", ref.MarshalString()); err != nil {
			return nil, err
		}
		return nil, block.ErrNotFound
	}
	raw, err := block.EncodeBlockObject(stored.Data, stored.Refs, stored.RefsKnown)
	if err != nil {
		return nil, err
	}

	// Save reusable bytes and provenance in the caller's scratch transaction.
	if db == w.DB {
		return stored, w.cache(ctx, ref.MarshalString(), raw, "volume")
	}
	if _, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO cache VALUES(?,?)", ref.MarshalString(), raw); err != nil {
		return nil, err
	}
	_, err = db.ExecContext(ctx, "INSERT OR IGNORE INTO sources VALUES(?,'volume')", ref.MarshalString())
	return stored, err
}

// hole commits a terminal judgment before a later run can read the extent again.
func (w *Walker) hole(ctx context.Context, ref, kind string) error {
	// Commit both lookup and frontier judgments atomically.
	tx, err := w.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO failures VALUES(?,?)", ref, kind); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE nodes SET state=1,kind=? WHERE ref=?", kind, ref); err != nil {
		return err
	}
	return tx.Commit()
}

// lookupHole saves an off-frontier judgment and reports absence to type lookup.
func (w *Walker) lookupHole(ctx context.Context, ref *block.BlockRef, kind string) error {
	if err := w.hole(ctx, ref.MarshalString(), kind); err != nil {
		return err
	}
	return block.ErrNotFound
}

// cache atomically saves verified bytes and the source that served them.
func (w *Walker) cache(ctx context.Context, ref string, raw []byte, source string) error {
	// Keep a crash from saving bytes without their provenance.
	tx, err := w.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO cache VALUES(?,?)", ref, raw); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO sources VALUES(?,?)", ref, source); err != nil {
		return err
	}
	return tx.Commit()
}

// checkSource refuses to resume a source that reached the configuration stop.
func (w *Walker) checkSource(ctx context.Context) error {
	// Read one durable stop marker before any new remote request.
	var stopped bool
	if err := w.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM inputs WHERE name='source-configuration-stop')").Scan(&stopped); err != nil {
		return err
	}
	if stopped {
		return ErrSourceConfiguration
	}
	return nil
}

// stopSource latches the configuration error across process replacement.
func (w *Walker) stopSource(ctx context.Context, refs ...string) error {
	// Commit the judged range and stop marker together, including off-frontier lookups.
	tx, err := w.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, ref := range refs {
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO failures VALUES(?,'hole:source')", ref); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE nodes SET state=1,kind='hole:source' WHERE ref=?", ref); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO inputs VALUES('source-configuration-stop','1')"); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return ErrSourceConfiguration
}
