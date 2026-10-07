package world_block

// ChangeLogEntry is one linked-list entry plus its expanded change batch.
// An entry without changes marks where the retained history starts.
type ChangeLogEntry struct {
	// Seqno is the World seqno the entry recorded.
	Seqno uint64
	// ChangeType is the entry's change type.
	ChangeType WorldChangeType
	// TotalSize is the number of changes in the batch, including linked nodes.
	TotalSize uint32
	// Changes is the expanded change batch.
	Changes []*WorldChange
}
