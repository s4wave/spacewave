package common

import (
	"errors"
	"io/fs"
	"os"
)

// SyncLocalWAL makes durable every commit the WAL of the database file at
// dbPath holds, and the directory entries of the database and its WAL. The
// WAL is flushed through a handle of its own: a flush covers the file's
// written pages whichever handle wrote them. An empty path, or a database with
// no WAL file, has nothing to flush.
func SyncLocalWAL(dbPath string) error {
	// Skip a database with no file.
	if dbPath == "" {
		return nil
	}

	// Flush the WAL's data.
	f, err := os.OpenFile(dbPath+"-wal", os.O_RDWR, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := errors.Join(syncFileData(f), f.Close()); err != nil {
		return err
	}

	// Flush the directory so a newly created database or WAL survives.
	return syncDir(dbPath)
}
