package common

import "strings"

// queries are the statements a transaction runs on one table, built once per
// store.
type queries struct {
	// get reads one key's value.
	get string
	// size counts the keys.
	size string
	// set writes one key's value.
	set string
	// scanAll reads every key in order.
	scanAll string
	// scanPrefix reads the keys between a prefix and its successor.
	scanPrefix string
	// scanPrefixUnbounded reads the keys from a prefix with no successor.
	scanPrefixUnbounded string
	// delete removes one key.
	delete string
	// exists checks one key.
	exists string
	// table is the table the statements read and write.
	table string
}

// newQueries builds the statements for table.
func newQueries(table string) *queries {
	q := func(parts ...string) string { return strings.Join(parts, " ") }
	return &queries{
		get:                 q("SELECT value FROM", table, "WHERE key = ?"),
		size:                q("SELECT COUNT(*) FROM", table),
		set:                 q("INSERT OR REPLACE INTO", table, "(key, value) VALUES (?, ?)"),
		scanAll:             q("SELECT key, value FROM", table, "ORDER BY key"),
		scanPrefix:          q("SELECT key, value FROM", table, "WHERE key >= ? AND key < ? ORDER BY key"),
		scanPrefixUnbounded: q("SELECT key, value FROM", table, "WHERE key >= ? ORDER BY key"),
		delete:              q("DELETE FROM", table, "WHERE key = ?"),
		exists:              q("SELECT 1 FROM", table, "WHERE key = ? LIMIT 1"),
		table:               table,
	}
}
