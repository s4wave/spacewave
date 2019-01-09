package block

import (
	"github.com/aperturerobotics/hydra/cid"
	"gonum.org/v1/gonum/graph"
)

// handle contains working state for a block.
type handle struct {
	// nod is the graph node
	nod graph.Node
	// parent is the parent node
	parent *refHandle
	// ref is the base block reference.
	ref *cid.BlockRef
	// refHandles contains pointers to traversed references.
	// nil initially
	refHandles map[uint32]*refHandle
	// dirty indicates the block has been changed
	dirty bool
	// blk is the decoded block if attached
	blk Block
}

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
