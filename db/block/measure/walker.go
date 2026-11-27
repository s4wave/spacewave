// Package measure measures a frozen block DAG with a disk frontier and exact
// disk deduplication. No graph-sized collection is retained in memory.
package measure

import (
	"context"
	"database/sql"
	"io"
	"math"
	"strconv"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
)

// Walker owns a resumable scratch database, separate from every input store.
// A block's count and outgoing edges commit together. Repeating an interrupted
// range may download its bytes again, but never counts a block twice.
type Walker struct {
	// DB holds the local frontier, cache, counts and parent edges.
	DB *sql.DB
	// store owns the scratch connection lifecycle.
	store sqlite.Store
	// Locate returns the frozen index's pack number, offset and encoded length.
	Locate func(context.Context, *block.BlockRef) (uint32, int64, int, bool, error)
	// ReadRange reads a bounded range from the named pack, or stops the walk.
	ReadRange func(context.Context, string, int64, int) ([]byte, error)
	// Describe returns a type label and typed children, without storing state.
	Describe func(context.Context, *block.BlockRef, []byte, []byte) (string, []Child, error)
}

// Open creates or resumes a scratch database. Close releases it. Callers must
// bind immutable input identities with Bind before seeding or resuming.
func Open(path string) (*Walker, error) {
	// Bound SQLite's page cache and keep durable resume commits local.
	store, err := sqlite.OpenWithPragmas(context.Background(), path, "scratch", sqlite.Pragmas{FullSync: true, CacheSize: -4096})
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
 CREATE INDEX IF NOT EXISTS frontier ON nodes(state,depth,pack,off);
 CREATE TABLE IF NOT EXISTS edges(child TEXT NOT NULL,parent TEXT NOT NULL,PRIMARY KEY(child,parent));
 CREATE TABLE IF NOT EXISTS cache(ref TEXT PRIMARY KEY,data BLOB NOT NULL);`)
	if err != nil {
		store.Close()
		return nil, err
	}
	return &Walker{DB: db, store: store}, nil
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
	return w.enqueue(ctx, w.DB, Child{Ref: ref, Type: typ}, "root:"+name, 0)
}

// executor is the common write surface of a database and a transaction.
type executor interface {
	// ExecContext changes only the scratch database.
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

// enqueue deduplicates a block by its hash and keeps every parent on disk.
func (w *Walker) enqueue(ctx context.Context, db executor, c Child, parent string, depth int) error {
	// Locate before insertion; an absent extent is an unresolved index hole.
	if c.Ref.GetEmpty() {
		return nil
	}
	num, off, n, found, err := w.Locate(ctx, c.Ref)
	if err != nil {
		return err
	}
	if !found {
		n = 0
	}

	// Persist one identity and type, upgrading an earlier untyped sighting.
	typ := c.Type
	if typ == nil {
		typ = []byte{}
	}
	key := c.Ref.MarshalString()
	_, err = db.ExecContext(ctx, `INSERT INTO nodes(ref,type,depth,pack,off,n) VALUES(?,?,?,?,?,?)
 ON CONFLICT(ref) DO UPDATE SET type=excluded.type,state=0 WHERE length(nodes.type)=0 AND length(excluded.type)>0`, key, typ, depth, int64(num), off, n)
	if err != nil {
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

// next returns one bounded contiguous run at the lowest pending level. The
// disk index orders an entire frontier by pack and offset without collecting it.
func (w *Walker) next(ctx context.Context) ([]item, error) {
	// Take at most 4096 refs in pack order from the first pending level.
	rows, err := w.DB.QueryContext(ctx, `SELECT ref,type,depth,pack,off,n,
 EXISTS(SELECT 1 FROM cache WHERE cache.ref=nodes.ref)
 FROM nodes WHERE state=0 ORDER BY depth,pack,off LIMIT 4096`)
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
// leave the current run pending; no error is silently converted into a hole.
func (w *Walker) Run(ctx context.Context) error {
	for {
		// Read the next breadth-first run, outside any scratch transaction.
		run, err := w.next(ctx)
		if err != nil || len(run) == 0 {
			return err
		}
		if run[0].n == 0 {
			if _, err = w.DB.ExecContext(ctx, "UPDATE nodes SET state=1,kind='hole:index' WHERE ref=?", run[0].ref); err != nil {
				return err
			}
			continue
		}
		if run[0].cached {
			for _, it := range run {
				if err = w.visit(ctx, it); err != nil {
					return err
				}
			}
			continue
		}
		var id string
		var size int64
		if err = w.DB.QueryRowContext(ctx, "SELECT id,size FROM packs WHERE num=?", int64(run[0].pack)).Scan(&id, &size); err != nil {
			return errors.Wrap(err, "pack missing from frozen table")
		}
		end := run[len(run)-1].off + int64(run[len(run)-1].n)
		if end > size {
			return errors.New("extent runs past pack length")
		}
		data, err := w.ReadRange(ctx, id, run[0].off, int(end-run[0].off))
		if errors.Is(err, block.ErrNotFound) {
			for _, it := range run {
				if _, err = w.DB.ExecContext(ctx, "UPDATE nodes SET state=1,kind='hole:source' WHERE ref=?", it.ref); err != nil {
					return err
				}
			}
			continue
		}
		if err != nil {
			return err
		}

		// Cache each verified encoded block for type lookups and crash resume.
		for _, it := range run {
			at := it.off - run[0].off
			if at < 0 || at+int64(it.n) > int64(len(data)) {
				return errors.New("short measurement range")
			}
			ref, err := block.UnmarshalBlockRefB58(it.ref)
			if err != nil {
				return err
			}
			raw := data[at : at+int64(it.n)]
			if _, err = decode(ref, raw); err != nil {
				return err
			}
			if _, err = w.DB.ExecContext(ctx, "INSERT OR IGNORE INTO cache VALUES(?,?)", it.ref, raw); err != nil {
				return err
			}
		}

		// Commit a block's result together with its deduplicated children.
		for _, it := range run {
			if err = w.visit(ctx, it); err != nil {
				return err
			}
		}
	}
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

	// Read only the indexed extent and retain it on disk for the later walk.
	num, off, n, found, err := w.Locate(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	var id string
	if err = w.DB.QueryRowContext(ctx, "SELECT id FROM packs WHERE num=?", int64(num)).Scan(&id); err != nil {
		return nil, err
	}

	// Verify a newly downloaded extent before making it reusable.
	raw, err = w.ReadRange(ctx, id, off, n)
	if err != nil {
		return nil, err
	}
	stored, err := decode(ref, raw)
	if err != nil {
		return nil, err
	}
	_, err = w.DB.ExecContext(ctx, "INSERT OR IGNORE INTO cache VALUES(?,?)", ref.MarshalString(), raw)
	return stored, err
}

// visit describes one cached block and atomically commits its graph edges.
func (w *Walker) visit(ctx context.Context, it item) error {
	// Decode its type outside the write transaction: type lookup may read cache.
	ref, err := block.UnmarshalBlockRefB58(it.ref)
	if err != nil {
		return err
	}
	stored, err := w.GetStoredBlock(ctx, ref)
	if err != nil {
		return err
	}
	kind, kids, err := w.Describe(ctx, ref, stored.Data, it.typ)
	if err != nil {
		return err
	}
	if !stored.RefsKnown && kind == "untyped" {
		return errors.New("block has no recorded child list; measurement is incomplete: " + it.ref)
	}

	// Preserve the stored closure even when a type is unknown or its list differs.
	tx, err := w.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, kid := range kids {
		if err = w.enqueue(ctx, tx, kid, it.ref, it.depth+1); err != nil {
			return err
		}
	}
	for _, kid := range stored.Refs {
		if err = w.enqueue(ctx, tx, Child{Ref: kid}, it.ref, it.depth+1); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, "UPDATE nodes SET state=1,kind=?,bytes=? WHERE ref=?", kind, len(stored.Data), it.ref)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// Report streams stable tab-separated totals, histograms, pack live fractions
// and unresolved holes with all parents. Index holes are not confirmed B2 loss.
func (w *Walker) Report(ctx context.Context, out io.Writer) error {
	queries := []string{
		`SELECT name,value FROM inputs WHERE name IN ('reserved-bytes','reserved-requests')`,
		`SELECT 'total',count(*),coalesce(sum(bytes),0) FROM nodes WHERE state=1 AND kind NOT LIKE 'hole:%'`,
		`SELECT 'pending',count(*),0 FROM nodes WHERE state=0`,
		`SELECT 'holes:index-unresolved',count(*),0 FROM nodes WHERE kind='hole:index'`,
		`SELECT 'holes:source-unavailable',count(*),0 FROM nodes WHERE kind='hole:source'`,
		`SELECT 'histogram',kind,bin,count(*),sum(bytes) FROM
 (SELECT kind,bytes,CASE WHEN bytes<1024 THEN 0 WHEN bytes<4096 THEN 1024
 WHEN bytes<16384 THEN 4096 WHEN bytes<65536 THEN 16384
 WHEN bytes<1048576 THEN 65536 ELSE 1048576 END AS bin
 FROM nodes WHERE state=1 AND kind NOT LIKE 'hole:%') GROUP BY kind,bin`,
		`SELECT 'pack',p.id,p.size,count(*),sum(n.n),1.0*sum(n.n)/p.size
 FROM nodes n JOIN packs p ON p.num=n.pack
 WHERE n.state=1 AND n.kind NOT LIKE 'hole:%' GROUP BY p.num`,
		`SELECT CASE WHEN n.kind='hole:index' THEN 'hole:index-unresolved' ELSE 'hole:source-unavailable' END,n.ref,e.parent
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
