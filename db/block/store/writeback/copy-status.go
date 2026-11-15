package block_store_writeback

// CopyStatus reports a copy of the stored blocks to a remote.
type CopyStatus struct {
	// Target is the remote the copy writes to, empty before a copy starts.
	Target string
	// Running reports a copy in progress.
	Running bool
	// Done reports every block stored before the copy started is on the
	// remote.
	Done bool
	// Position is the share of the key space the copy has passed, from 0 to 1.
	// Block keys are hashes, so it tracks the share of blocks.
	Position float64
	// Scanned is the number of blocks the copy listed since it started.
	Scanned int64
	// Present is the number of listed blocks the remote already held.
	Present int64
	// Copied is the number of blocks the copy wrote to the remote.
	Copied int64
	// CopiedBytes is the size of the blocks the copy wrote.
	CopiedBytes int64
	// Gone is the number of listed blocks the local store no longer held when
	// the copy read them.
	Gone int64
	// RefsUnknown is the number of blocks the copy left out because the local
	// store does not know their refs. The remote would keep them as leaves, so
	// they stay local, and a copy with any is not complete for dropping the
	// local store.
	RefsUnknown int64
	// Corrupt is the number of blocks the copy left out because their local
	// bytes fail their hash. They stay local, the copy goes on past them, and
	// a copy with any is not done: its saved cursor stays before the first
	// step that left one out, so the next call reads it again.
	Corrupt int64
}

// add adds the step counts of o to c.
func (c *CopyStatus) add(o CopyStatus) {
	// Sum each count.
	c.Scanned += o.Scanned
	c.Present += o.Present
	c.Copied += o.Copied
	c.CopiedBytes += o.CopiedBytes
	c.Gone += o.Gone
	c.RefsUnknown += o.RefsUnknown
	c.Corrupt += o.Corrupt
}
