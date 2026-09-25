package logfile

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// DefaultKeepLogs is the number of newest prior logs kept regardless of age.
// Each process start writes a new log, so frequent restarts would otherwise
// accumulate logs for the whole retention window.
const DefaultKeepLogs = 8

// PruneOldLogs deletes regular .log files under dir whose mtime is older
// than maxAge relative to now, and all but the keep newest of the rest.
// A non-positive keep disables the count limit. Returns the number of files
// removed and the first error encountered (other failures are still
// attempted).
//
// Non-existent dir is treated as success with zero removed files.
// Subdirectories and non-.log entries are skipped. Symlinks are not
// followed; their own metadata is consulted via os.Lstat semantics from
// os.ReadDir's DirEntry.
func PruneOldLogs(dir string, maxAge time.Duration, keep int, now time.Time) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	// Collect regular log files; a failed stat is reported but skipped.
	var logs []os.FileInfo
	var firstErr error
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if info.Mode().IsRegular() {
			logs = append(logs, info)
		}
	}

	// Newest first: keep the first keep logs younger than the cutoff.
	slices.SortFunc(logs, func(a, b os.FileInfo) int {
		return b.ModTime().Compare(a.ModTime())
	})
	cutoff := now.Add(-maxAge)
	var removed int
	for i, info := range logs {
		if !info.ModTime().Before(cutoff) && (keep <= 0 || i < keep) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, info.Name())); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		removed++
	}
	return removed, firstErr
}
