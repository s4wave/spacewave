package volume_rpc

import (
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_rpc "github.com/s4wave/spacewave/db/block/rpc"
)

// NewNamedRoot returns the wire form of root.
func NewNamedRoot(root block.NamedRoot) *NamedRoot {
	out := &NamedRoot{Name: root.Name, Ref: root.Ref, Previous: root.Previous}
	if !root.Expires.IsZero() {
		out.ExpiresUnixMilli = root.Expires.UnixMilli()
	}
	return out
}

// ToNamedRoot returns the root the wire form names.
func (r *NamedRoot) ToNamedRoot() block.NamedRoot {
	out := block.NamedRoot{Name: r.GetName(), Ref: r.GetRef(), Previous: r.GetPrevious()}
	if ms := r.GetExpiresUnixMilli(); ms != 0 {
		out.Expires = time.UnixMilli(ms)
	}
	return out
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
