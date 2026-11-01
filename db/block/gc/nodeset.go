package block_gc

import "hash/maphash"

// nodeSetMinBits is the base-2 logarithm of an empty NodeSet's table size.
const nodeSetMinBits = 10

// NodeSet is a set of graph nodes that keeps 16 bytes a node, so a mark of
// millions of nodes fits a small memory budget. It stores a 128-bit hash of
// each node under two random seeds chosen per set: two nodes share a hash with
// probability about n²/2¹²⁹, and no one can choose nodes that collide without
// the seeds. A collision would make Add report a new node as present.
type NodeSet struct {
	// seeds key the two halves of the hash of each node that is not a
	// block.
	seeds [2]maphash.Seed
	// blockSeeds key the two halves of the hash of each block's encoding.
	// Separate seeds keep a block and another node from sharing a hash by
	// spelling.
	blockSeeds [2]maphash.Seed
	// slots is an open-addressed table of 1<<bits hashes, in which the zero
	// hash marks an empty slot.
	slots [][2]uint64
	// bits is the base-2 logarithm of the table size.
	bits uint
	// count is the number of nodes in the set.
	count int
}

// NewNodeSet returns an empty set.
func NewNodeSet() *NodeSet {
	return &NodeSet{
		seeds:      [2]maphash.Seed{maphash.MakeSeed(), maphash.MakeSeed()},
		blockSeeds: [2]maphash.Seed{maphash.MakeSeed(), maphash.MakeSeed()},
		slots:      make([][2]uint64, 1<<nodeSetMinBits),
		bits:       nodeSetMinBits,
	}
}

// Sibling returns an empty set that hashes nodes as s does, so Overlap can
// compare the two by hash.
func (s *NodeSet) Sibling() *NodeSet {
	return &NodeSet{
		seeds:      s.seeds,
		blockSeeds: s.blockSeeds,
		slots:      make([][2]uint64, 1<<nodeSetMinBits),
		bits:       nodeSetMinBits,
	}
}

// Overlap returns the number of nodes of s that o holds. o must be s or a
// sibling of it.
func (s *NodeSet) Overlap(o *NodeSet) int {
	n := 0
	for _, h := range s.slots {
		if h == ([2]uint64{}) {
			continue
		}
		if _, found := o.find(h); found {
			n++
		}
	}
	return n
}

// Len returns the number of nodes in the set.
func (s *NodeSet) Len() int {
	return s.count
}

// Has reports whether node is in the set.
func (s *NodeSet) Has(node string) bool {
	_, found := s.find(s.hash(node))
	return found
}

// Add inserts node and reports whether it was absent.
func (s *NodeSet) Add(node string) bool {
	return s.addHash(s.hash(node))
}

// addHash inserts the node of hash h and reports whether it was absent.
func (s *NodeSet) addHash(h [2]uint64) bool {
	// Find the node's slot, or the empty slot it would take.
	i, found := s.find(h)
	if found {
		return false
	}

	// Store it, doubling the table past a load of 0.8.
	s.slots[i] = h
	s.count++
	if s.count*5 > len(s.slots)*4 {
		s.grow()
	}
	return true
}

// hash returns the 128-bit hash of node. The zero hash is reserved for empty
// slots, so it is moved to another value.
func (s *NodeSet) hash(node string) [2]uint64 {
	// Hash a block by its encoding.
	if isBlockNode(node) {
		enc := node[len(prefixBlock):]
		h := [2]uint64{maphash.String(s.blockSeeds[0], enc), maphash.String(s.blockSeeds[1], enc)}
		if h == ([2]uint64{}) {
			h[1] = 1
		}
		return h
	}

	// Hash any other node by its IRI.
	h := [2]uint64{maphash.String(s.seeds[0], node), maphash.String(s.seeds[1], node)}
	if h == ([2]uint64{}) {
		h[1] = 1
	}
	return h
}

// find returns the slot holding h, or the empty slot that ends its probe.
func (s *NodeSet) find(h [2]uint64) (uint64, bool) {
	mask := uint64(1)<<s.bits - 1
	for i := h[0] & mask; ; i = (i + 1) & mask {
		switch s.slots[i] {
		case h:
			return i, true
		case [2]uint64{}:
			return i, false
		}
	}
}

// grow doubles the table and reinserts every hash.
func (s *NodeSet) grow() {
	old := s.slots
	s.bits++
	s.slots = make([][2]uint64, 1<<s.bits)
	for _, h := range old {
		if h != ([2]uint64{}) {
			i, _ := s.find(h)
			s.slots[i] = h
		}
	}
}
