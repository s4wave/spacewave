package writer

import (
	"io"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
)

// PackGraph packs the blocks iter yields as PackBlocks does, laid out in graph
// order: the structure blocks, those with refs, in depth-first pre-order from
// the pack's roots, then the leaves in the same order. Each structure block's
// index entry records its reach, the end of the last structure block placed
// under it, so the range from a structure block to its reach holds the
// structure of its subtree in this pack and one range read fetches it.
//
// The roots are the blocks no other block of the pack references, in the
// order iter yields them, so a caller streaming blocks in walk order keeps its
// trees in that order. A block two parents share is placed under the first, so
// for the second parent its reach is a hint. Refs to blocks outside the pack
// are ignored.
//
// Invariants: the pack holds every block iter yields exactly once, as
// PackBlocks does. A duplicate block key fails the pack, as kvfile does for
// PackBlocks. Every reach lies within the pack's values. PackGraph holds the
// blocks iter yields until the pack is written, as a pack is built in memory.
func PackGraph(w io.Writer, iter BlockIterator) (*PackResult, error) {
	// Collect the blocks in the order iter yields them, keyed by block key.
	type node struct {
		h        *hash.Hash
		blk      *block.StoredBlock
		children []int
	}
	var nodes []node
	at := make(map[string]int)
	for {
		h, blk, err := iter()
		if err != nil {
			return nil, errors.Wrap(err, "iterating blocks")
		}
		if h == nil {
			break
		}
		key := string(packfile.BlockKey(h))
		if _, ok := at[key]; ok {
			return nil, errors.Errorf("duplicate block %s", h.MarshalString())
		}
		at[key] = len(nodes)
		nodes = append(nodes, node{h: h, blk: blk})
	}

	// Link each block to its children in the pack, in ref order.
	referenced := make([]bool, len(nodes))
	for i := range nodes {
		for _, ref := range nodes[i].blk.Refs {
			if ref.GetEmpty() || ref.GetHash() == nil {
				continue
			}
			if j, ok := at[string(packfile.BlockKey(ref.GetHash()))]; ok && j != i {
				nodes[i].children = append(nodes[i].children, j)
				referenced[j] = true
			}
		}
	}

	// Lay the blocks out depth first from the roots, recording for each the
	// layout position one past its subtree. A last pass over every block
	// places any a root does not reach, so none is dropped.
	layout := make([]int, 0, len(nodes))
	end := make([]int, len(nodes))
	placed := make([]bool, len(nodes))
	type frame struct{ i, next int }
	var stack []frame
	visit := func(start int) {
		// Place the start once, then walk its children depth first.
		if placed[start] {
			return
		}
		placed[start] = true
		layout = append(layout, start)
		stack = append(stack, frame{i: start})
		for len(stack) != 0 {
			top := len(stack) - 1
			i, next := stack[top].i, stack[top].next
			if next == len(nodes[i].children) {
				end[i] = len(layout)
				stack = stack[:top]
				continue
			}
			stack[top].next++
			if c := nodes[i].children[next]; !placed[c] {
				placed[c] = true
				layout = append(layout, c)
				stack = append(stack, frame{i: c})
			}
		}
	}
	for i := range nodes {
		if !referenced[i] {
			visit(i)
		}
	}
	for i := range nodes {
		visit(i)
	}

	// Write the structure blocks, then the leaves, each in layout order.
	structure := func(i int) bool { return len(nodes[i].blk.Refs) != 0 }
	p := newPacker(w)
	value := make([]int, len(nodes))
	valueEnd := make([]uint64, len(nodes))
	for _, leaves := range []bool{false, true} {
		for _, i := range layout {
			if structure(i) == leaves {
				continue
			}
			value[i] = p.kvw.Len()
			if err := p.add(nodes[i].h, nodes[i].blk); err != nil {
				return nil, err
			}
			valueEnd[i] = p.kvw.GetPos()
		}
	}

	// A structure block reaches to the end of the last structure block in its
	// subtree's layout range, which holds at least the block itself.
	lastStructure := make([]int, len(layout))
	last := -1
	for k, i := range layout {
		if structure(i) {
			last = i
		}
		lastStructure[k] = last
	}
	for _, i := range layout {
		if !structure(i) {
			continue
		}
		if err := p.kvw.SetReach(value[i], valueEnd[lastStructure[end[i]-1]]); err != nil {
			return nil, err
		}
	}
	return p.finish(ValueOrderGraph)
}
