package volume_scoped

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// refGraph serves the GC ref graph of a scoped volume view.
//
// Object and bucket nodes are the view's own: each gains the prefix on the way
// in and loses it on the way out, and nodes of other owners stay hidden. Block
// nodes are shared by content address: a plugin may add edges from a block, and
// may not remove them, since that could make another owner's data collectable.
// The permanent roots belong to the volume's collector, with two exceptions
// that GC tracking of the view's buckets needs: the view may root its own
// bucket nodes, and may mark or unmark a block as unreferenced, a staging mark
// that retains nothing.
type refGraph struct {
	// inner is the ref graph of the underlying volume.
	inner block_gc.RefGraphOps
	// prefix is the view prefix.
	prefix string
}

// AddRef adds a gc/ref edge from subject to object.
func (g *refGraph) AddRef(ctx context.Context, subject, object string) error {
	edge, err := g.scopeEdge(block_gc.RefEdge{Subject: subject, Object: object}, true)
	if err != nil {
		return err
	}
	return g.inner.AddRef(ctx, edge.Subject, edge.Object)
}

// RemoveRef removes a single gc/ref edge from subject to object.
func (g *refGraph) RemoveRef(ctx context.Context, subject, object string) error {
	edge, err := g.scopeEdge(block_gc.RefEdge{Subject: subject, Object: object}, false)
	if err != nil {
		return err
	}
	return g.inner.RemoveRef(ctx, edge.Subject, edge.Object)
}

// ApplyRefBatch applies one ownership transition. The view refuses the whole
// batch when any edge is outside it.
func (g *refGraph) ApplyRefBatch(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	// Map every edge into the volume before applying any.
	scopedAdds, err := g.scopeEdges(adds, true)
	if err != nil {
		return err
	}
	scopedRemoves, err := g.scopeEdges(removes, false)
	if err != nil {
		return err
	}

	// Apply the batch to the volume.
	err = g.inner.ApplyRefBatch(ctx, scopedAdds, scopedRemoves)
	if err == nil {
		return nil
	}

	// Name the uncommitted suffix in the view's own nodes.
	remainderAdds, remainderRemoves, ok := block_gc.RefBatchRemainder(err)
	if !ok {
		return err
	}
	return &batchError{
		err:     err,
		adds:    g.unscopeEdges(remainderAdds),
		removes: g.unscopeEdges(remainderRemoves),
	}
}

// RemoveNodeRefs removes all outgoing gc/ref edges of an object or bucket node.
func (g *refGraph) RemoveNodeRefs(ctx context.Context, node string, markOrphaned bool) ([]string, error) {
	// Map the owner node into the volume.
	scoped, err := g.scopeOwner(node)
	if err != nil {
		return nil, err
	}

	// Remove the owner's edges and report only the targets in the view.
	targets, err := g.inner.RemoveNodeRefs(ctx, scoped, markOrphaned)
	if err != nil {
		return nil, err
	}
	return g.unscopeNodes(targets), nil
}

// HasIncomingRefs checks if a node has any incoming gc/ref edges.
//
// The answer counts the edges of every owner in the volume, since a block
// stays live while any owner holds it.
func (g *refGraph) HasIncomingRefs(ctx context.Context, node string) (bool, error) {
	scoped, err := g.scopeNode(node)
	if err != nil {
		return false, err
	}
	return g.inner.HasIncomingRefs(ctx, scoped)
}

// HasIncomingRefsExcluding checks if a node has any incoming gc/ref edges
// excluding the source nodes in the view. The permanent roots cannot be named
// in the view, and the underlying graph already ignores the unreferenced node.
func (g *refGraph) HasIncomingRefsExcluding(ctx context.Context, node string, excluded ...string) (bool, error) {
	// Map the node into the volume.
	scoped, err := g.scopeNode(node)
	if err != nil {
		return false, err
	}

	// Map the excluded owners, skipping the nodes the view cannot name.
	scopedExcluded := make([]string, 0, len(excluded))
	for _, ex := range excluded {
		if scopedEx, err := g.scopeNode(ex); err == nil {
			scopedExcluded = append(scopedExcluded, scopedEx)
		}
	}
	return g.inner.HasIncomingRefsExcluding(ctx, scoped, scopedExcluded...)
}

// GetOutgoingRefs returns the targets of gc/ref edges from a node in the view.
func (g *refGraph) GetOutgoingRefs(ctx context.Context, node string) ([]string, error) {
	// Map the node into the volume.
	scoped, err := g.scopeNode(node)
	if err != nil {
		return nil, err
	}

	// Report only the targets in the view.
	targets, err := g.inner.GetOutgoingRefs(ctx, scoped)
	if err != nil {
		return nil, err
	}
	return g.unscopeNodes(targets), nil
}

// GetIncomingRefs returns the sources in the view with gc/ref edges to a node.
func (g *refGraph) GetIncomingRefs(ctx context.Context, node string) ([]string, error) {
	// Map the node into the volume.
	scoped, err := g.scopeNode(node)
	if err != nil {
		return nil, err
	}

	// Report only the sources in the view.
	sources, err := g.inner.GetIncomingRefs(ctx, scoped)
	if err != nil {
		return nil, err
	}
	return g.unscopeNodes(sources), nil
}

// GetUnreferencedNodes refuses to list the unreferenced nodes of every owner.
func (g *refGraph) GetUnreferencedNodes(ctx context.Context) ([]string, error) {
	return nil, errors.Wrap(ErrRefused, "list unreferenced nodes")
}

// AddBlockRef adds gc/ref from source block to target block.
func (g *refGraph) AddBlockRef(ctx context.Context, source, target *block.BlockRef) error {
	return g.inner.AddBlockRef(ctx, source, target)
}

// AddObjectRoot adds gc/ref from object:{key} in the view to block.
func (g *refGraph) AddObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	return g.inner.AddObjectRoot(ctx, g.prefix+objectKey, ref)
}

// RemoveObjectRoot removes gc/ref from object:{key} in the view to block.
func (g *refGraph) RemoveObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	return g.inner.RemoveObjectRoot(ctx, g.prefix+objectKey, ref)
}

// Close leaves the ref graph open: the underlying volume owns it.
func (g *refGraph) Close() error {
	return nil
}

// scopeNode maps a node of the view to its node in the volume. It refuses the
// permanent roots and any other node that is not an object, bucket or block.
func (g *refGraph) scopeNode(node string) (string, error) {
	if key, ok := block_gc.ParseObjectIRI(node); ok {
		return block_gc.ObjectIRI(g.prefix + key), nil
	}
	if id, ok := block_gc.ParseBucketIRI(node); ok {
		return block_gc.BucketIRI(g.prefix + id), nil
	}
	if _, ok := block_gc.ParseBlockIRI(node); ok {
		return node, nil
	}
	return "", errors.Wrapf(ErrRefused, "ref graph node %q", node)
}

// scopeOwner maps an object or bucket node of the view to its node in the
// volume and refuses block nodes.
func (g *refGraph) scopeOwner(node string) (string, error) {
	scoped, err := g.scopeNode(node)
	if err != nil {
		return "", err
	}
	if _, ok := block_gc.ParseBlockIRI(node); ok {
		return "", errors.Wrapf(ErrRefused, "ref graph owner %q", node)
	}
	return scoped, nil
}

// scopeEdge maps an edge of the view to its edge in the volume. A removal must
// start at an object or bucket node of the view, or at the unreferenced node.
func (g *refGraph) scopeEdge(edge block_gc.RefEdge, add bool) (block_gc.RefEdge, error) {
	// Map a staging mark, which names a block, and a bucket root, which names a
	// bucket of the view.
	switch {
	case edge.Subject == block_gc.NodeUnreferenced:
		if _, ok := block_gc.ParseBlockIRI(edge.Object); !ok {
			return block_gc.RefEdge{}, errors.Wrapf(ErrRefused, "unreferenced mark of %q", edge.Object)
		}
		return edge, nil
	case edge.Subject == block_gc.NodeGCRoot && add:
		id, ok := block_gc.ParseBucketIRI(edge.Object)
		if !ok {
			return block_gc.RefEdge{}, errors.Wrapf(ErrRefused, "gcroot edge to %q", edge.Object)
		}
		return block_gc.RefEdge{Subject: edge.Subject, Object: block_gc.BucketIRI(g.prefix + id)}, nil
	}

	// Map the subject, which may only be an owner when removing.
	var scoped block_gc.RefEdge
	var err error
	if add {
		scoped.Subject, err = g.scopeNode(edge.Subject)
	} else {
		scoped.Subject, err = g.scopeOwner(edge.Subject)
	}
	if err != nil {
		return block_gc.RefEdge{}, err
	}

	// Map the object.
	scoped.Object, err = g.scopeNode(edge.Object)
	if err != nil {
		return block_gc.RefEdge{}, err
	}
	return scoped, nil
}

// scopeEdges maps the edges of the view to edges in the volume.
func (g *refGraph) scopeEdges(edges []block_gc.RefEdge, add bool) ([]block_gc.RefEdge, error) {
	scoped := make([]block_gc.RefEdge, len(edges))
	for i, edge := range edges {
		var err error
		scoped[i], err = g.scopeEdge(edge, add)
		if err != nil {
			return nil, err
		}
	}
	return scoped, nil
}

// unscopeNode maps a node of the volume to its node in the view. It reports
// false for the nodes outside the view.
func (g *refGraph) unscopeNode(node string) (string, bool) {
	if key, ok := block_gc.ParseObjectIRI(node); ok {
		key, ok = strings.CutPrefix(key, g.prefix)
		return block_gc.ObjectIRI(key), ok
	}
	if id, ok := block_gc.ParseBucketIRI(node); ok {
		id, ok = strings.CutPrefix(id, g.prefix)
		return block_gc.BucketIRI(id), ok
	}
	_, ok := block_gc.ParseBlockIRI(node)
	return node, ok
}

// unscopeNodes maps the nodes of the volume to the view and drops the nodes
// outside it.
func (g *refGraph) unscopeNodes(nodes []string) []string {
	unscoped := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if mapped, ok := g.unscopeNode(node); ok {
			unscoped = append(unscoped, mapped)
		}
	}
	return unscoped
}

// unscopeEdges maps the edges of the volume to the view and drops the edges
// outside it.
func (g *refGraph) unscopeEdges(edges []block_gc.RefEdge) []block_gc.RefEdge {
	unscoped := make([]block_gc.RefEdge, 0, len(edges))
	for _, edge := range edges {
		// A permanent root stays as it is: the view named it as it is.
		subject, subjectOK := edge.Subject, true
		if edge.Subject != block_gc.NodeUnreferenced && edge.Subject != block_gc.NodeGCRoot {
			subject, subjectOK = g.unscopeNode(edge.Subject)
		}
		object, objectOK := g.unscopeNode(edge.Object)
		if subjectOK && objectOK {
			unscoped = append(unscoped, block_gc.RefEdge{Subject: subject, Object: object})
		}
	}
	return unscoped
}

// _ is a type assertion
var _ block_gc.RefGraphOps = (*refGraph)(nil)
