package block_store_fillcache

import (
	"context"
	"os"
	"strings"

	"github.com/pkg/errors"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"github.com/sirupsen/logrus"
)

// cacheTable is the key/value table of the cache's database.
const cacheTable = "fill_cache"

// filePragmas are the cache file's layout and settings. The table keeps
// rowids, so a block's row holds most of a page before it spills to overflow
// pages; a WITHOUT ROWID row keeps only about a quarter of one, and reading
// such a table cost three to four times as much per block at 300,000 blocks
// of 1 to 16 KiB. Its page is 4 KiB, which read as fast as 16 KiB and keeps
// the file a fifth smaller. Its write-ahead log is bounded at 64 MiB, so the
// log stays small enough that reads search it quickly even when reads never
// let SQLite restart it. The read benchmark replaces it to compare layouts;
// nothing else writes it.
var filePragmas = store_kvtx_sqlite.Pragmas{
	PageSize:           4 << 10,
	WALCheckpointBytes: 64 << 20,
}

// quickCheckBytes is the file size up to which Open checks the file's
// structure. A larger file opens unchecked, since scanning it would hold up the
// start for the whole scan; its blocks are hash-checked as they are read.
const quickCheckBytes = 1 << 30

// openStore opens the cache's database at path. A file that fails to open or to
// pass the structure check is deleted with its log and recreated empty, as the
// cache holds only copies.
func openStore(ctx context.Context, le *logrus.Entry, path string) (store_kvtx_sqlite.Store, error) {
	// Open the file as it is.
	store, err := tryOpenStore(ctx, path)
	if err == nil {
		return store, nil
	}

	// Delete the unusable file and open a new one.
	le.WithError(err).Warn("the block fill cache file is unusable, recreating it")
	if err := removeFiles(path); err != nil {
		return nil, err
	}
	return tryOpenStore(ctx, path)
}

// tryOpenStore opens the database at path, creating it when missing, checks
// that its table and page are in filePragmas' layout, and checks the
// structure of a file up to quickCheckBytes. A file in another layout, as an
// older release wrote, fails, so openStore recreates it.
func tryOpenStore(ctx context.Context, path string) (store_kvtx_sqlite.Store, error) {
	// Open the database.
	store, err := store_kvtx_sqlite.OpenWithPragmas(ctx, path, cacheTable, filePragmas)
	if err != nil {
		return nil, err
	}

	// Check the file's layout, which only a new file takes from filePragmas.
	if err := checkLayout(ctx, store); err != nil {
		_ = store.Close()
		return nil, err
	}

	// Check the structure of a file small enough to scan at once.
	if info, err := os.Stat(path); err == nil && info.Size() <= quickCheckBytes {
		var result string
		if err := store.GetDB().QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&result); err != nil {
			_ = store.Close()
			return nil, err
		}
		if result != "ok" {
			_ = store.Close()
			return nil, errors.Errorf("quick check: %s", result)
		}
	}
	return store, nil
}

// checkLayout returns an error when the cache table's rowids or the file's page
// size differ from filePragmas.
func checkLayout(ctx context.Context, store store_kvtx_sqlite.Store) error {
	// Read the table's definition and the file's page size.
	db := store.GetDB()
	var def string
	if err := db.QueryRowContext(ctx, "SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?", cacheTable).Scan(&def); err != nil {
		return err
	}
	var page int32
	if err := db.QueryRowContext(ctx, "PRAGMA page_size").Scan(&page); err != nil {
		return err
	}

	// Compare them with the layout.
	withoutRowID := strings.Contains(strings.ToUpper(def), "WITHOUT ROWID")
	if withoutRowID != filePragmas.WithoutRowID || (filePragmas.PageSize != 0 && page != filePragmas.PageSize) {
		return errors.Errorf("layout: without rowid %v, page %d", withoutRowID, page)
	}
	return nil
}

// removeFiles deletes the database file and its log and shared-memory files.
func removeFiles(path string) error {
	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
