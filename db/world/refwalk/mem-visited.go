package refwalk

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// MemVisited is a Visited in memory, the Walker's default. It holds one entry
// per finished block, which grows with the graph walked: a walk of a graph too
// large for memory passes a Visited that stores its keys.
type MemVisited struct {
	// types holds, by each finished block's marshaled ref, a bit per type the
	// block was read as, indexed by bits.
	types map[string]uint64
	// bits holds the bit of each type name, the bit of "", a scan, first.
	bits map[string]uint64
}

// NewMemVisited constructs an empty MemVisited.
func NewMemVisited() *MemVisited {
	return &MemVisited{types: make(map[string]uint64), bits: map[string]uint64{"": 1}}
}

// Finished reports, for each key, whether a walk finished it.
func (m *MemVisited) Finished(_ context.Context, keys []VisitKey) ([]bool, error) {
	out := make([]bool, len(keys))
	for i, key := range keys {
		bit := m.bits[key.Type]
		out[i] = bit != 0 && m.types[key.Ref.MarshalString()]&bit != 0
	}
	return out, nil
}

// Finish records that a walk finished key. It holds up to 64 type names,
// which no schema approaches.
func (m *MemVisited) Finish(_ context.Context, key VisitKey) error {
	bit, ok := m.bits[key.Type]
	if !ok {
		if len(m.bits) == 64 {
			return errors.Errorf("more than 64 block types, at %s", key.Type)
		}
		bit = 1 << len(m.bits)
		m.bits[key.Type] = bit
	}
	m.types[key.Ref.MarshalString()] |= bit
	return nil
}

// Reached reports whether a walk finished ref under a type, and whether one
// finished it by scanning.
func (m *MemVisited) Reached(_ context.Context, ref *block.BlockRef) (typed, scanned bool, err error) {
	typed, scanned = m.Seen(ref)
	return typed, scanned, nil
}

// Seen is Reached without a context, which memory does not need.
func (m *MemVisited) Seen(ref *block.BlockRef) (typed, scanned bool) {
	types := m.types[ref.MarshalString()]
	return types&^1 != 0, types&1 != 0
}

// ScannedOnly returns how many blocks walks finished by scanning alone and
// under no type.
func (m *MemVisited) ScannedOnly() uint64 {
	var n uint64
	for _, types := range m.types {
		if types == 1 {
			n++
		}
	}
	return n
}
