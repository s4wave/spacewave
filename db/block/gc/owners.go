package block_gc

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// OwnerCount counts the owners of one kind a mark seeded from.
type OwnerCount struct {
	// Nodes is the number of owners of the kind.
	Nodes int
	// Edges is the number of their forward records.
	Edges int
}

// MarkOption configures a mark.
type MarkOption func(*BlockMark)

// KeepOwnerEdges keeps the forward records of the owners of kind, the name
// before an owner's colon, in BlockMark.OwnerEdges. The mark reads them while
// seeding, so keeping them costs no read.
func KeepOwnerEdges(kind string) MarkOption {
	return func(m *BlockMark) { m.keepKind = kind }
}

// MarkLike makes BlockMark.Live a sibling of set, so set.Overlap can compare
// the two by hash.
func MarkLike(set *NodeSet) MarkOption {
	return func(m *BlockMark) { m.Live = set.Sibling() }
}

// newBlockMark returns an empty mark configured by opts.
func newBlockMark(opts []MarkOption) *BlockMark {
	m := &BlockMark{Live: NewNodeSet()}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// countOwner counts the forward record from owner to object, and keeps it when
// the mark keeps owner's kind. Forward records arrive in key order, grouped by
// subject, so an owner is new when it differs from the last.
func (m *BlockMark) countOwner(owner, object, last string) {
	// Count the record under the owner's kind, the name before its colon.
	if m.Owners == nil {
		m.Owners = make(map[string]OwnerCount)
	}
	kind, _, _ := strings.Cut(owner, ":")
	c := m.Owners[kind]
	c.Edges++
	if owner != last {
		c.Nodes++
	}
	m.Owners[kind] = c

	// Keep the record of a kept kind.
	if m.keepKind != "" && kind == m.keepKind {
		m.OwnerEdges = append(m.OwnerEdges, RefEdge{Subject: owner, Object: object})
	}
}

// FormatOwners spells owner counts as kind=nodes/edges, sorted by kind.
func FormatOwners(owners map[string]OwnerCount) string {
	var b strings.Builder
	for _, kind := range slices.Sorted(maps.Keys(owners)) {
		if b.Len() != 0 {
			b.WriteByte(' ')
		}
		c := owners[kind]
		fmt.Fprintf(&b, "%s=%d/%d", kind, c.Nodes, c.Edges)
	}
	return b.String()
}
