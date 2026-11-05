package s4db

// Options configures a database.
type Options struct {
	// CacheBytes bounds the block cache of decoded run blocks, index
	// partitions and filters. The memtable has its own budget, a constant of
	// the file. Zero selects 192 MiB.
	CacheBytes int
	// Families are key prefixes whose values Stats counts apart. A key
	// counts under the first family it starts with.
	Families [][]byte
	// ReadOnly takes a reader slot and never the writer lock: no write
	// transactions and no background jobs. For tools beside a running daemon
	// and for checking a converted file.
	ReadOnly bool

	// memtable is the memtable budget written into a new file's header, a
	// constant of the file every process flushes at. Zero selects 64 MiB;
	// tests set it small.
	memtable int64
}

// minMemtable is the smallest memtable budget a file may carry, so a flush
// writes at least a few blocks.
const minMemtable = 64 << 10

// defaultMemtable is the memtable budget of a new file.
const defaultMemtable = 64 << 20

// withDefaults returns o with zero fields set to their defaults.
func (o Options) withDefaults() Options {
	if o.CacheBytes <= 0 {
		o.CacheBytes = 192 << 20
	}
	if o.memtable <= 0 {
		o.memtable = defaultMemtable
	}
	o.memtable = max(o.memtable, minMemtable)
	return o
}
