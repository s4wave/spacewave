package block

import (
	"fmt"

	"gonum.org/v1/gonum/graph"
	"gonum.org/v1/gonum/graph/encoding"
	"gonum.org/v1/gonum/graph/encoding/dot"
)

// handle contains working state for a block.
type handle struct {
	// nod is the base graph node
	graph.Node
	// parent is the parent node
	parent *refHandle
	// ref is the base block reference.
	// may be nil
	ref *BlockRef
	// isSubBlock indicates if this is a sub-block.
	isSubBlock bool
	// refHandles contains pointers to traversed references.
	// nil initially
	refHandles map[uint32]*refHandle
	// dirty indicates the block has been changed
	dirty bool

	// blk is the decoded block and/or sub-block pointer if known
	blk interface{}
	// blkPreWrite is the pre write callback
	blkPreWrite func(b interface{}) error
}

// Clone clones the handle object.
func (h *handle) Clone() *handle {
	return &handle{
		Node:        h.Node,
		parent:      h.parent,
		ref:         h.ref,
		isSubBlock:  h.isSubBlock,
		refHandles:  h.refHandles,
		dirty:       h.dirty,
		blk:         h.blk,
		blkPreWrite: h.blkPreWrite,
	}
}

// DOTID returns a DOT node ID.
//
// An ID is one of the following:
//
//  - a string of alphabetic ([a-zA-Z\x80-\xff]) characters, underscores ('_').
//    digits ([0-9]), not beginning with a digit.
//  - a numeral [-]?(.[0-9]+ | [0-9]+(.[0-9]*)?).
//  - a double-quoted string ("...") possibly containing escaped quotes (\").
//  - an HTML string (<...>).
func (h *handle) DOTID() string {
	if h.isSubBlock {
		// TODO: locking?
		var parentid string
		var subBlockId uint32
		if h.parent != nil && h.parent.src != nil {
			parentid = h.parent.src.DOTID()
			subBlockId = h.parent.id
		}
		return fmt.Sprintf("%s@%d", parentid, subBlockId)
	}

	return h.ref.MarshalString()
}

// Attributes returns the graph attributes
func (r *handle) Attributes() []encoding.Attribute {
	var res []encoding.Attribute
	if r.blk != nil {
		attrs, ok := r.blk.(BlockWithAttributes)
		if ok {
			res = append(res, attrs.GetBlockGraphAttributes()...)
		}
	}
	return res
}

var _ graph.Node = ((*handle)(nil))

// refHandle is a block ref handle.
type refHandle struct {
	// id is the ref identifier.
	// determined by code, usually ref field id
	id uint32
	// src is the block handle src
	src *handle
	// target is the block handle target
	target *handle
}

// From returns the from node of the edge.
func (r *refHandle) From() graph.Node {
	return r.src
}

// To returns the to node of the edge.
func (r *refHandle) To() graph.Node {
	return r.target
}

// ReversedEdge returns an edge that has
// the end points of the receiver swapped.
func (r *refHandle) ReversedEdge() graph.Edge {
	return &refHandle{src: r.target, target: r.src}
}

/*
func (r *refHandle) FromPort() (string, string) {
	return strconv.Itoa(int(r.id)), ""
}

func (r *refHandle) ToPort() (string, string) {
	return "parent", ""
}
*/

// _ is a type assertion
var _ graph.Edge = ((*refHandle)(nil))

// _ is a type assertion
var _ dot.Node = ((*handle)(nil))
