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
	// WALCheckpointBytes bounds the write-ahead log: journal_size_limit
	// truncates the log to it whenever SQLite restarts it, and once the
	// store's background checkpoint has copied all of a log past it, a
	// truncating checkpoint resets it. Under constant reads and heavy writes
	// SQLite may never restart the log, which can then grow without bound.
	// 0 uses DefaultWALCheckpointBytes.
	WALCheckpointBytes int64
}
