package block

// DiscardDetached releases an abandoned cursor position from its transaction.
// The transaction root and positions still referenced by another parent survive.
// It returns the traversed references of a discarded position without discarding
// their targets. Positions cleared by an earlier write are already released.
// The caller must no longer use the discarded cursor position.
func (c *Cursor) DiscardDetached() map[uint32]*Cursor {
	if c == nil || c.t == nil || c.pos == nil {
		return nil
	}
	c.t.mtx.Lock()
	defer c.t.mtx.Unlock()

	// ClearTree may have reused the numeric ID for a different position.
	if c.t.blockGraph.Node(c.pos.ID()) != c.pos || c.pos == c.t.root || len(c.pos.parents) != 0 {
		return nil
	}
	refs := make(map[uint32]*Cursor, len(c.pos.refHandles))
	for id, ref := range c.pos.refHandles {
		refs[id] = newCursor(c.t, ref.target, c.store)
	}
	c.clearRefHandles()
	c.t.blockGraph.RemoveNode(c.pos.ID())
	return refs
}

// DiscardDetachedTree releases an abandoned cursor position and every
// descendant left without another parent. Descendants still referenced from
// outside the discarded tree survive. The caller must no longer use any
// discarded position.
func (c *Cursor) DiscardDetachedTree() {
	for _, child := range c.DiscardDetached() {
		child.DiscardDetachedTree()
	}
}
