package world_block

import (
	"context"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/graph"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_cayley "github.com/s4wave/spacewave/db/kvtx/cayley"
	kvtx_cayley_nodegraph "github.com/s4wave/spacewave/db/kvtx/cayley/nodegraph"
)

// NewestWorldFormat is the format new Worlds are created with when their
// creator asks for the newest.
//
// A World's format is stored in the root's format field and in the
// implementation tags of both of its trees, and they must agree:
//
//   - A node graph World's object tree is tagged KV_IMPL_TYPE_IAVL_NODE_GRAPH
//     and its graph tree KV_IMPL_TYPE_OKRA_INLINE_NODE_GRAPH, both from
//     creation (NewWorld). A build that predates the node graph ignores the
//     format field, which proto3 allows, but opens the object tree as it
//     loads the root and rejects its unknown tag there, so it refuses the
//     World before any read or write. Tagging the graph tree alone would not
//     do: such a build opens the graph only on a graph call, so an object-only
//     write would commit a root whose unknown graph tag hides the graph
//     tree's blocks from that build's reference walk.
//   - A kv World's trees carry neither tag.
//   - checkTreeImpls refuses a root where the field and tags disagree, at
//     load, before any read or write.
//
// Within the node graph tree a quad is two keys, its out key under the subject
// and its in key under the object. The store writes both in the same
// transaction and a World commits its graph tree as one root, so a committed
// tree holds both or neither; nothing repairs a tree holding one.
const NewestWorldFormat = WorldFormat_WORLD_FORMAT_NODE_GRAPH

// newObjectTree returns the root of a new object tree of the format. A kv
// World's stays untagged, as every kv World's object tree has been.
func (f WorldFormat) newObjectTree() *kvtx_block.KeyValueStore {
	if f == WorldFormat_WORLD_FORMAT_NODE_GRAPH {
		return kvtx_block.NewKeyValueStore(kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL_NODE_GRAPH)
	}
	return &kvtx_block.KeyValueStore{}
}

// setFormat sets the World's format and tags its object tree as newObjectTree
// tags a new one. Both formats keep the object tree in the IAVL layout, so
// only the tag changes; the tree's root sits inside the World block, so the
// World's next write rewrites that block alone. The caller sets a graph tree
// of the format.
func (w *World) setFormat(f WorldFormat) error {
	// A World with no object tree yet gets an empty one to tag.
	kv := w.GetObjectKeyValue()
	if kv == nil {
		kv = &kvtx_block.KeyValueStore{}
		w.ObjectKeyValue = kv
	}

	// Only a tree in the IAVL layout can change format by its tag alone.
	switch impl := kv.GetImplType(); impl {
	case 0, kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL, kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL_NODE_GRAPH:
	default:
		return errors.Errorf("retag a %v object tree", impl)
	}

	// Tag the tree as a new one of the format and set the format.
	kv.ImplType = f.newObjectTree().GetImplType()
	w.Format = f
	return nil
}

// graphImpl returns the implementation a new graph tree of the format takes.
func (f WorldFormat) graphImpl() kvtx_block.KVImplType {
	if f == WorldFormat_WORLD_FORMAT_NODE_GRAPH {
		return kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA_INLINE_NODE_GRAPH
	}
	return kvtx_block.DefaultKeyValueStoreImplForWorkload(kvtx_block.WorkloadClassGraphPrefixRead)
}

// checkTreeImpls checks that a World root's trees carry its format's tags:
// both of a node graph World's trees are tagged as such, and neither of a kv
// World's is.
func (w *World) checkTreeImpls() error {
	objTagged := w.GetObjectKeyValue().GetImplType() == kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL_NODE_GRAPH
	graphTagged := w.GetGraphKeyValue().GetImplType() == kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA_INLINE_NODE_GRAPH
	switch w.GetFormat() {
	case WorldFormat_WORLD_FORMAT_CAYLEY_KV:
		if objTagged || graphTagged {
			return errors.New("kv World has a node graph tree")
		}
		return nil
	case WorldFormat_WORLD_FORMAT_NODE_GRAPH:
		if !objTagged || !graphTagged {
			return errors.New("node graph World's trees are not tagged as such")
		}
		return nil
	}
	return errors.Errorf("unknown World format %v", w.GetFormat())
}

// ValidateInitial checks the format a new World starts with, under the graph
// settings it starts with: the node graph has fixed edge ranges and takes no
// index settings.
func (f WorldFormat) ValidateInitial(settings *GraphSettings) error {
	switch f {
	case WorldFormat_WORLD_FORMAT_CAYLEY_KV:
		return settings.ValidateInitial()
	case WorldFormat_WORLD_FORMAT_NODE_GRAPH:
		if len(settings.GetIndexes()) != 0 {
			return errors.New("node graph World takes no graph settings")
		}
		return nil
	}
	return errors.Errorf("unknown World format %v", f)
}

// openGraph opens the graph store of a World of format over store. The handle
// keeps ctx for its internal transactions.
func openGraph(ctx context.Context, format WorldFormat, settings *GraphSettings, store kvtx.TxOps) (*cayley.Handle, error) {
	switch format {
	case WorldFormat_WORLD_FORMAT_CAYLEY_KV:
		// Open under the root's settings, so the store reads no layout.
		opts, err := settings.graphOptions()
		if err != nil {
			return nil, err
		}
		return kvtx_cayley.NewGraph(ctx, kvtx.NewTxStore(store), opts)
	case WorldFormat_WORLD_FORMAT_NODE_GRAPH:
		if len(settings.GetIndexes()) != 0 {
			return nil, errors.New("node graph World has graph settings")
		}
		return kvtx_cayley_nodegraph.NewGraph(store)
	}
	return nil, errors.Errorf("unknown World format %v", format)
}

// batchCollector returns the store's own collector for a lookup of n filters,
// or nil when a cached handle shared across the filters serves them better.
// The node graph's collector resolves each value once for the whole batch and
// seeks each filter's narrowed range; the kv collector resolves each filter
// alone, so a kv batch of more than one shares the cached handle instead.
func (f WorldFormat) batchCollector(qs graph.QuadStore, n int) graphQuadBatchCollector {
	collector, _ := qs.(graphQuadBatchCollector)
	if f == WorldFormat_WORLD_FORMAT_CAYLEY_KV && n != 1 {
		return nil
	}
	return collector
}
