package volume_rpc

import (
	"github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
)

// NewNamedRoot returns the wire form of root. A lease belongs to the process
// of the Volume that holds it, so the wire carries none.
func NewNamedRoot(root block.NamedRoot) *NamedRoot {
	return &NamedRoot{Name: root.Name, Ref: root.Ref, Previous: root.Previous, RequireHeld: root.RequireHeld}
}

// ToNamedRoot returns the root the wire form records.
func (r *NamedRoot) ToNamedRoot() block.NamedRoot {
	return block.NamedRoot{Name: r.GetName(), Ref: r.GetRef(), Previous: r.GetPrevious(), RequireHeld: r.GetRequireHeld()}
}

// NewPutBatchEntries returns the wire form of entries.
func NewPutBatchEntries(entries []*block.PutBatchEntry) []*block_rpc.PutBlockBatchEntry {
	out := make([]*block_rpc.PutBlockBatchEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, &block_rpc.PutBlockBatchEntry{
			Ref:       entry.Ref,
			Data:      entry.Data,
			Refs:      entry.Refs,
			Tombstone: entry.Tombstone,
			RefsKnown: entry.RefsKnown,
		})
	}
	return out
}

// ToPutBatchEntries returns the entries the wire forms carry.
func ToPutBatchEntries(entries []*block_rpc.PutBlockBatchEntry) []*block.PutBatchEntry {
	out := make([]*block.PutBatchEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, &block.PutBatchEntry{
			Ref:       entry.GetRef(),
			Data:      entry.GetData(),
			Refs:      entry.GetRefs(),
			RefsKnown: entry.GetRefsKnown(),
			Tombstone: entry.GetTombstone(),
		})
	}
	return out
}
