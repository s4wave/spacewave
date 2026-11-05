package common

// Pragmas configures tunable SQLite pragmas applied during Open, and the shape
// of a table Open creates. A zero value leaves SQLite's compiled default in
// place.
type Pragmas struct {
	// FullSync selects synchronous=FULL; the existing default is NORMAL.
	// Connectors that open several physical connections must configure each one.
	FullSync bool
	// CacheSize sets the SQLite cache_size pragma. Positive = pages,
	// negative = KiB. 0 leaves the SQLite default.
	CacheSize int32
	// MmapSize sets the SQLite mmap_size pragma in bytes. 0 leaves the
	// SQLite default (mmap disabled).
	MmapSize int64
	// TempStore sets the SQLite temp_store pragma. Valid values are 0
	// (DEFAULT), 1 (FILE), 2 (MEMORY). 0 leaves the SQLite default.
	TempStore int32
	// PageSize sets the SQLite page_size pragma in bytes. Must be a power
	// of two between 512 and 65536. 0 leaves the SQLite default. Only
	// effective on a fresh database.
	PageSize int32
	// WithoutRowID creates a missing table as WITHOUT ROWID, so each row is
	// stored once, in the key's B-tree, instead of in a rowid table plus a key
	// index. It suits many small rows; SQLite advises against it for rows
	// larger than about a twentieth of a page. An existing table keeps its
	// shape.
	WithoutRowID bool
	// WALCheckpointBytes bounds the write-ahead log: past it, a write that
	// commits starts a background checkpoint, and journal_size_limit truncates
	// the log to it whenever the log restarts. SQLite's own checkpoint after
	// a commit copies only what no reader needs and never shrinks the file,
	// so under constant reads and heavy writes the log can grow without
	// bound. 0 leaves both to SQLite.
	WALCheckpointBytes int64
}
