package kvtx_cayley_nodegraph

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/cayley/quad/pquads"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/util/extsort"
)

// Builder writes a whole node graph from its nodes and quads, given in any
// order, as the ascending key stream of a new tree.
//
// Each node keeps the ID its caller gives it. IDs need not be dense, since the
// tree's next ID is one past the largest added, so a caller converting another
// store keeps that store's IDs and needs no map from old IDs to new ones. A
// node a quad names as predicate or label is pinned, as ApplyDeltas pins it.
//
// The stream depends only on the nodes and quads added, not on their order or
// on where the sorter spilled, so two builds of the same graph stream the
// same keys and values.
type Builder struct {
	sorter *extsort.Sorter
	// maxID is the largest node ID added.
	maxID uint64
}

// NewBuilder returns a Builder that sorts through sorter. The caller closes
// sorter.
func NewBuilder(sorter *extsort.Sorter) *Builder {
	return &Builder{sorter: sorter}
}

// AddNode adds node id with value v.
func (b *Builder) AddNode(id uint64, v *pquads.Value) error {
	// Write the ID map entry and the unpinned meta; a pin marker merges in.
	nv := v.ToNative()
	if id == 0 || nv == nil {
		return errors.Errorf("node graph build: node %d has no ID or value", id)
	}
	meta, err := (&NodeMeta{Value: v}).MarshalVT()
	if err != nil {
		return err
	}
	if err := b.sorter.Add(valueKey(nv.String()), encodeID(id)); err != nil {
		return err
	}
	b.maxID = max(b.maxID, id)
	return b.sorter.Add(metaKey(id), meta)
}

// AddQuad adds the quad of node IDs s, p, o and l, with l zero for no label.
func (b *Builder) AddQuad(s, p, o, l uint64) error {
	// Write both edge keys, then an empty pin marker for the predicate and the
	// label. A meta value is never empty, so Write tells the two apart.
	if s == 0 || p == 0 || o == 0 {
		return errors.Errorf("node graph build: quad <%d %d %d> lacks a node", s, p, o)
	}
	out, in := edgeKeys(quadRef{s: s, p: p, o: o, l: l})
	for _, k := range [][]byte{out, in, metaKey(p)} {
		if err := b.sorter.Add(k, nil); err != nil {
			return err
		}
	}
	if l == 0 {
		return nil
	}
	return b.sorter.Add(metaKey(l), nil)
}

// Write streams the built tree's keys and values to fn in ascending key
// order, once. fn must not keep key or value after it returns. A quad added
// twice writes its edges once, since an old kv store can hold several
// physical copies of one quad. Write fails on any other key added twice, such
// as two nodes with one value, and on a pin of a node never added.
func (b *Builder) Write(ctx context.Context, fn func(key, value []byte) error) error {
	// Sort in the next ID, one past the largest node, when there is a node.
	if b.maxID != 0 {
		if err := b.sorter.Add(nextIDKey, encodeID(b.maxID+1)); err != nil {
			return err
		}
	}

	// Hold one key back until the next differs, so a meta key's records,
	// adjacent in the merge, combine into one value.
	var held heldKey
	err := b.sorter.Merge(ctx, nil, func(key, value []byte) error {
		if held.ok && bytes.Equal(held.key, key) {
			return held.merge(value)
		}
		if err := held.flush(fn); err != nil {
			return err
		}
		held.set(key, value)
		return nil
	})
	if err != nil {
		return err
	}
	return held.flush(fn)
}

// heldKey is the key Write holds until it has seen all its records.
type heldKey struct {
	key, value []byte
	// ok is whether a key is held.
	ok bool
	// meta is whether the key is a node's meta key.
	meta bool
	// pinned is whether a pin marker named the node.
	pinned bool
}

// set holds key with its first record, copying both out of the merge's
// buffers.
func (h *heldKey) set(key, value []byte) {
	// Copy into the held buffers, then classify the key.
	h.key = append(h.key[:0], key...)
	h.value = append(h.value[:0], value...)
	h.ok, h.pinned = true, false
	_, tag, ok := parseNodeKey(key)
	h.meta = ok && tag == tagMeta && len(key) == nodeKeyLen+1
	if h.meta && len(value) == 0 {
		h.pinned, h.value = true, h.value[:0]
	}
}

// merge adds another record of the held key: a pin marker, the meta of a
// node whose pin markers came first, or another copy of a quad's edge. Any
// other repeat is an error.
func (h *heldKey) merge(value []byte) error {
	switch {
	case !h.meta && len(value) == 0 && len(h.value) == 0:
		return nil
	case !h.meta:
		return errors.Errorf("node graph build: key %q added twice", h.key)
	case len(value) == 0:
		h.pinned = true
	case len(h.value) == 0:
		h.value = append(h.value, value...)
	default:
		return errors.Errorf("node graph build: node %x added twice", h.key[len(nodePrefix):nodeKeyLen])
	}
	return nil
}

// flush passes the held key to fn, pinning its meta when a marker named it.
func (h *heldKey) flush(fn func(key, value []byte) error) error {
	// Nothing is held before the first key.
	if !h.ok {
		return nil
	}
	h.ok = false
	if !h.meta || !h.pinned {
		return fn(h.key, h.value)
	}

	// A pinned node must have been added, and stores its pin in its meta.
	if len(h.value) == 0 {
		return errors.Errorf("node graph build: quad names node %x, never added", h.key[len(nodePrefix):nodeKeyLen])
	}
	meta := &NodeMeta{}
	if err := meta.UnmarshalVT(h.value); err != nil {
		return err
	}
	meta.Pinned = true
	value, err := meta.MarshalVT()
	if err != nil {
		return err
	}
	return fn(h.key, value)
}
