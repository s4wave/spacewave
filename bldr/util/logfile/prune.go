package logfile

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PruneOldLogs deletes regular .log files under dir whose mtime is older
// than maxAge relative to now. Returns the number of files removed and
// the first error encountered (other failures are still attempted).
//
// Non-existent dir is treated as success with zero removed files.
// Subdirectories and non-.log entries are skipped. Symlinks are not
// followed; their own metadata is consulted via os.Lstat semantics from
// os.ReadDir's DirEntry.
func PruneOldLogs(dir string, maxAge time.Duration, now time.Time) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	cutoff := now.Add(-maxAge)
	var removed int
	var firstErr error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		// Skip non-regular files (symlinks, sockets, etc.).
		if !info.Mode().IsRegular() {
			continue
		}
		if !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		removed++
	}
	return removed, firstErr
}
