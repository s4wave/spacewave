package block_store_fillcache

import (
	"context"
	"os"

	"github.com/pkg/errors"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"github.com/sirupsen/logrus"
)

// cacheTable is the key/value table of the cache's database.
const cacheTable = "fill_cache"

// pageSize is the page size of a new cache file. Blocks are larger than the
// default page, and a larger page shortens the chain of pages a block spans.
const pageSize = 16 << 10

// walBytes bounds the cache's write-ahead log, so the log stays small enough
// that reads search it quickly even when reads never let SQLite restart it.
const walBytes = 64 << 20

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

// tryOpenStore opens the database at path, creating it when missing, and
// checks the structure of a file up to quickCheckBytes.
func tryOpenStore(ctx context.Context, path string) (store_kvtx_sqlite.Store, error) {
	// Open the database.
	store, err := store_kvtx_sqlite.OpenWithPragmas(ctx, path, cacheTable, store_kvtx_sqlite.Pragmas{
		WithoutRowID:       true,
		PageSize:           pageSize,
		WALCheckpointBytes: walBytes,
	})
	if err != nil {
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

// removeFiles deletes the database file and its log and shared-memory files.
func removeFiles(path string) error {
	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
