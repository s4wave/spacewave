package block_store_nodepush

import (
	"context"
	"database/sql"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile/identity"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
)

// packListSchema is the list's one table: a row per read copy, with its size
// and when it was stored, indexed by age for pruning.
const packListSchema = `
CREATE TABLE IF NOT EXISTS packs (
	id TEXT PRIMARY KEY,
	bytes INTEGER NOT NULL,
	stored INTEGER NOT NULL
) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS packs_stored ON packs (stored, id);
`

// packListKVTable is the key/value table the store opener creates beside the
// list's own table. The list leaves it empty.
const packListKVTable = "kv"

// listScanBatch is how many directory entries open reads at a time.
const listScanBatch = 256

// packList is the small SQLite list of the read copies a PackFiles holds: one
// row per pack file, named by the pack's ID on the remote. It holds no blocks.
//
// The files are the truth and the list is their index: a copy is listed after
// its file is named, and unlisted after its file is removed, so a crash
// between the two leaves a row without a file or a file without a row, and
// open puts the list back in step with the directory. The list is what the
// metrics sum and what pruning walks oldest first, so neither reads the
// directory.
type packList struct {
	// store owns the database file.
	store store_kvtx_sqlite.Store
	// db is the list's database.
	db *sql.DB
	// count is the number of copies listed.
	count atomic.Int64
	// bytes is the summed size of the copies listed.
	bytes atomic.Int64
}

// openPackList opens the list at path and puts it in step with the read
// copies in dir, removing what an interrupted store left there.
func openPackList(ctx context.Context, path, dir string) (*packList, error) {
	// Open the database with the build's SQLite driver, and its table.
	store, err := store_kvtx_sqlite.Open(ctx, path, packListKVTable)
	if err != nil {
		return nil, errors.Wrap(err, "open the read copy list")
	}
	l := &packList{store: store, db: store.GetDB()}
	if _, err := l.db.ExecContext(ctx, packListSchema); err != nil {
		_ = store.Close()
		return nil, errors.Wrap(err, "create the read copy list")
	}

	// Match the rows to the files, then count them.
	if err := l.reconcile(ctx, dir); err != nil {
		_ = store.Close()
		return nil, err
	}
	var count, bytes int64
	if err := l.db.QueryRowContext(ctx, "SELECT COUNT(*), COALESCE(SUM(bytes), 0) FROM packs").Scan(&count, &bytes); err != nil {
		_ = store.Close()
		return nil, err
	}
	l.count.Store(count)
	l.bytes.Store(bytes)
	return l, nil
}

// reconcile drops the rows whose file is gone, lists the files with no row, and
// removes any file that is not a whole pack, such as the temporary file of a
// store a crash cut short. It runs in one transaction, so a first open over a
// directory of thousands of copies commits once.
func (l *packList) reconcile(ctx context.Context, dir string) error {
	// Open the transaction, rolled back unless it commits.
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Drop the rows whose file is gone, collecting their IDs first so the
	// walk of the rows holds no write.
	rows, err := tx.QueryContext(ctx, "SELECT id FROM packs")
	if err != nil {
		return err
	}
	var gone []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		if _, err := os.Stat(filepath.Join(dir, id)); errors.Is(err, os.ErrNotExist) {
			gone = append(gone, id)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, id := range gone {
		if _, err := tx.ExecContext(ctx, "DELETE FROM packs WHERE id = ?", id); err != nil {
			return err
		}
	}

	// List each whole pack with no row, a batch of the directory at a time.
	f, err := os.Open(dir) // #nosec G304 -- the node's own directory
	if err != nil {
		return err
	}
	defer f.Close()
	for {
		entries, err := f.ReadDir(listScanBatch)
		for _, entry := range entries {
			if err := reconcileFile(ctx, tx, dir, entry); err != nil {
				return err
			}
		}
		if errors.Is(err, io.EOF) {
			return tx.Commit()
		}
		if err != nil {
			return err
		}
	}
}

// reconcileFile lists one file of the read copy directory in tx, or removes
// it when its name is not a pack ID.
func reconcileFile(ctx context.Context, tx *sql.Tx, dir string, entry os.DirEntry) error {
	// A name that is not a pack ID is a temporary file a crash left.
	name := entry.Name()
	if identity.ValidatePackID(name) != nil {
		if entry.Type().IsRegular() {
			return os.Remove(filepath.Join(dir, name))
		}
		return nil
	}

	// List a whole pack under its own name, dated by its file.
	info, err := entry.Info()
	if err != nil {
		return nil
	}
	_, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO packs (id, bytes, stored) VALUES (?, ?, ?)", name, info.Size(), info.ModTime().UnixNano())
	return err
}

// add lists the copy of pack id, size bytes, just stored.
func (l *packList) add(ctx context.Context, id string, size int64) error {
	// Replace any row of an earlier copy, counting the change once.
	var old int64
	err := l.db.QueryRowContext(ctx, "SELECT bytes FROM packs WHERE id = ?", id).Scan(&old)
	known := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := l.db.ExecContext(ctx, "INSERT OR REPLACE INTO packs (id, bytes, stored) VALUES (?, ?, ?)", id, size, time.Now().UnixNano()); err != nil {
		return err
	}
	if !known {
		l.count.Add(1)
	}
	l.bytes.Add(size - old)
	return nil
}

// remove unlists pack id, whose file is gone.
func (l *packList) remove(ctx context.Context, id string, size int64) error {
	res, err := l.db.ExecContext(ctx, "DELETE FROM packs WHERE id = ?", id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 0 {
		l.count.Add(-1)
		l.bytes.Add(-size)
	}
	return nil
}

// oldest returns the copy stored first, or false with none listed.
func (l *packList) oldest(ctx context.Context) (string, int64, bool, error) {
	// Read the first row by age; none is no copy.
	var id string
	var size int64
	err := l.db.QueryRowContext(ctx, "SELECT id, bytes FROM packs ORDER BY stored, id LIMIT 1").Scan(&id, &size)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, false, nil
	}
	return id, size, err == nil, err
}

// close closes the database.
func (l *packList) close() error {
	return l.store.Close()
}
