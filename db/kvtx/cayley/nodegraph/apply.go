package kvtx_cayley_nodegraph

import (
	"context"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
)

// quadText identifies a quad by its values' text, with "" for no label.
type quadText [4]string

// textOf returns the text of a quad's values.
func textOf(q quad.Quad) quadText {
	var t quadText
	for i, d := range quad.Directions {
		if v := q.Get(d); v != nil {
			t[i] = valueText(v)
		}
	}
	return t
}

// ApplyDeltas adds and removes quads.
//
// Every delta is checked before any key is written, so a rejected call
// changes nothing. A write that fails later in the call is undone with the
// call's other writes, all but the next ID key, which only grows, so the
// transaction holds no quad with one of its two keys and no node without its
// ID map entry even when its owner commits it after the error. A node left
// without edges is deleted with its ID map entry in the same call, unless it
// is pinned as a predicate or label.
func (qs *QuadStore) ApplyDeltas(ctx context.Context, in []graph.Delta, opts graph.IgnoreOpts) error {
	// An empty call writes nothing. Otherwise one batch resolves each value
	// once.
	if len(in) == 0 {
		return nil
	}
	b := &deltaBatch{nodes: &qs.nodes, ids: make(map[string]uint64), present: make(map[quadText]bool)}

	// Check every delta against the store and the deltas before it.
	apply := make([]bool, len(in))
	for i, d := range in {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !d.Quad.IsValid() {
			return &graph.DeltaError{Delta: d, Err: ErrInvalidQuad}
		}
		exists, err := b.exists(ctx, d.Quad)
		if err != nil {
			return err
		}
		switch d.Action {
		case graph.Add:
			if exists {
				if opts.IgnoreDup {
					continue
				}
				return &graph.DeltaError{Delta: d, Err: graph.ErrQuadExists}
			}
		case graph.Delete:
			if !exists {
				if opts.IgnoreMissing {
					continue
				}
				return &graph.DeltaError{Delta: d, Err: graph.ErrQuadNotExist}
			}
		default:
			return &graph.DeltaError{Delta: d, Err: graph.ErrInvalidAction}
		}
		b.present[textOf(d.Quad)] = d.Action == graph.Add
		apply[i] = true
	}

	// Write the accepted deltas, undoing the call's writes when one fails.
	err := b.write(ctx, in, apply)
	if err != nil {
		if rerr := qs.nodes.rollback(context.WithoutCancel(ctx)); rerr != nil {
			return errors.Wrapf(err, "node graph: undo failed: %v", rerr)
		}
	}
	qs.nodes.clearUndo()
	return err
}

// write writes the accepted deltas, then deletes the nodes they left empty.
func (b *deltaBatch) write(ctx context.Context, in []graph.Delta, apply []bool) error {
	emptied := make(map[uint64]struct{})
	for i, d := range in {
		if !apply[i] {
			continue
		}
		var err error
		if d.Action == graph.Add {
			err = b.add(ctx, d.Quad)
		} else {
			err = b.remove(ctx, d.Quad, emptied)
		}
		if err != nil {
			return err
		}
	}
	for id := range emptied {
		if err := b.nodes.deleteIfEmpty(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// deltaBatch resolves each distinct value once within one ApplyDeltas call.
type deltaBatch struct {
	// nodes owns node identity, metadata and reversible writes.
	nodes *nodeStore
	// ids maps value text to node ID, with 0 for a value that has no node.
	ids map[string]uint64
	// present is whether each quad checked in this call exists after the
	// deltas before it.
	present map[quadText]bool
}

// lookup returns the node ID of a value, or 0 when it has no node.
func (b *deltaBatch) lookup(ctx context.Context, v quad.Value) (uint64, error) {
	// A missing value has no node, and a value seen in this call is known.
	if v == nil {
		return 0, nil
	}
	text := valueText(v)
	if id, ok := b.ids[text]; ok {
		return id, nil
	}

	// Read the ID map once and remember the answer, a miss included.
	id, err := b.nodes.lookupID(ctx, text)
	if err != nil {
		return 0, err
	}
	b.ids[text] = id
	return id, nil
}

// resolve returns the quad's node IDs, with ok false when a value has no node.
func (b *deltaBatch) resolve(ctx context.Context, q quad.Quad) (quadRef, bool, error) {
	var ids [4]uint64
	for i, d := range quad.Directions {
		v := q.Get(d)
		if v == nil {
			continue
		}
		id, err := b.lookup(ctx, v)
		if err != nil || id == 0 {
			return quadRef{}, false, err
		}
		ids[i] = id
	}
	return quadRef{s: ids[0], p: ids[1], o: ids[2], l: ids[3]}, true, nil
}

// exists reports whether q exists after the deltas checked before it.
func (b *deltaBatch) exists(ctx context.Context, q quad.Quad) (bool, error) {
	// A quad an earlier delta of this call checked exists as that delta left it.
	if present, ok := b.present[textOf(q)]; ok {
		return present, nil
	}

	// Otherwise the quad exists when its values have nodes and its out key is set.
	qr, ok, err := b.resolve(ctx, q)
	if err != nil || !ok {
		return false, err
	}
	return b.nodes.containsQuad(ctx, qr)
}

// node returns the ID of a value, creating its node when it has none, and
// pins it when pin is set.
func (b *deltaBatch) node(ctx context.Context, v quad.Value, pin bool) (uint64, error) {
	// Find the value's node, if it has one.
	if v == nil {
		return 0, nil
	}
	id, err := b.lookup(ctx, v)
	if err != nil {
		return 0, err
	}

	// Create a missing node, or pin an existing one.
	if id == 0 {
		id, err = b.nodes.createNode(ctx, v, pin)
		if err != nil {
			return 0, err
		}
		b.ids[valueText(v)] = id
		return id, nil
	}
	if pin {
		err = b.nodes.pin(ctx, id)
	}
	return id, err
}

// add writes the out and in keys of a quad that does not exist.
func (b *deltaBatch) add(ctx context.Context, q quad.Quad) error {
	// Resolve the endpoints to their nodes, creating any that are new.
	var qr quadRef
	var err error
	if qr.s, err = b.node(ctx, q.Subject, false); err != nil {
		return err
	}
	if qr.o, err = b.node(ctx, q.Object, false); err != nil {
		return err
	}

	// Resolve the predicate and label the same way, pinning them.
	if qr.p, err = b.node(ctx, q.Predicate, true); err != nil {
		return err
	}
	if qr.l, err = b.node(ctx, q.Label, true); err != nil {
		return err
	}

	// Write the edge under both endpoints.
	out, in := edgeKeys(qr)
	if err := b.nodes.set(ctx, out, []byte{}, nil, false); err != nil {
		return err
	}
	return b.nodes.set(ctx, in, []byte{}, nil, false)
}

// remove deletes the out and in keys of a quad that exists, and adds its
// endpoints to emptied for the liveness check.
func (b *deltaBatch) remove(ctx context.Context, q quad.Quad, emptied map[uint64]struct{}) error {
	// Resolve the quad, which the check found, to its node IDs.
	qr, ok, err := b.resolve(ctx, q)
	if err != nil {
		return err
	}
	if !ok {
		return errors.Errorf("node graph: removed quad %v has no node", q)
	}

	// Delete the edge under both endpoints.
	out, in := edgeKeys(qr)
	if err := b.nodes.del(ctx, out, []byte{}); err != nil {
		return err
	}
	if err := b.nodes.del(ctx, in, []byte{}); err != nil {
		return err
	}

	// Queue each endpoint once for the liveness check.
	emptied[qr.s] = struct{}{}
	emptied[qr.o] = struct{}{}
	return nil
}
