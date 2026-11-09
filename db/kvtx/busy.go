package kvtx

import "strings"

// IsBusy reports whether err is a store's report that another writer holds its
// lock, such as SQLite's "database is locked" or SQLITE_BUSY, which a later
// attempt of the same transaction may pass.
func IsBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "database is locked") || strings.Contains(msg, "SQLITE_BUSY")
}
