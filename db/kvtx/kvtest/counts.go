package kvtx_kvtest

// Counts are the operations a CountingStore saw, for a scaling test that runs
// one call at two data sizes and compares what it did.
type Counts struct {
	// Txns counts the transactions opened.
	Txns uint64
	// WriteTxns counts the write transactions among them.
	WriteTxns uint64
	// Reads counts the point reads: Get and Exists.
	Reads uint64
	// Steps counts the keys a scan or an iterator visited, and its seeks.
	Steps uint64
	// Writes counts Set and Delete.
	Writes uint64
}

// Total returns the reads, steps and writes.
func (c Counts) Total() uint64 {
	return c.Reads + c.Steps + c.Writes
}

// Sub returns the operations c counted beyond base.
func (c Counts) Sub(base Counts) Counts {
	return Counts{
		Txns:      c.Txns - base.Txns,
		WriteTxns: c.WriteTxns - base.WriteTxns,
		Reads:     c.Reads - base.Reads,
		Steps:     c.Steps - base.Steps,
		Writes:    c.Writes - base.Writes,
	}
}
