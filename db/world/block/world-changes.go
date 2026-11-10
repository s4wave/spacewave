package world_block

import (
	"bytes"
	"context"
	fmt "fmt"
	"slices"

	"github.com/aperturerobotics/cayley/graph"
	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	cayley_proto "github.com/aperturerobotics/cayley/graph/proto"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_cayley "github.com/s4wave/spacewave/db/kvtx/cayley"
	kvtx_cayley_nodegraph "github.com/s4wave/spacewave/db/kvtx/cayley/nodegraph"
	"github.com/s4wave/spacewave/db/world"
)

// graphLogPrefix prefixes the records of the Cayley primitive log in the graph
// tree, one per node and link by ID. It mirrors the log index of the Cayley kv
// quad store, which adds a link record when a quad is set and removes it when
// the quad is deleted.
var graphLogPrefix = []byte("l/")

// WorldChanges lists the objects and graph quads that differ between two World
// roots. It compares the object and graph trees block by block and skips every
// subtree the roots share, so its cost follows the size of the change rather
// than the size of the World. Each tree comparison lists at most maxKeys tree
// keys; a larger change yields an unknown ChangeSet. A changed quad does not
// change the records of the objects it names, so only the quads report it.
func WorldChanges(ctx context.Context, before, after *block.Cursor, maxKeys int) (*world.ChangeSet, error) {
	// List the changed objects.
	keys, complete, err := ChangedObjectKeys(ctx, before, after, maxKeys)
	if err != nil || !complete {
		return world.NewUnknownChangeSet(), err
	}
	return worldChangesWithKeys(ctx, before, after, keys, maxKeys)
}

// worldChangesWithKeys completes the ChangeSet between two World roots that
// ChangedObjectKeys has decoded and compared, from the changed object keys it
// listed. It reuses the keys slice.
func worldChangesWithKeys(ctx context.Context, before, after *block.Cursor, keys []string, maxKeys int) (*world.ChangeSet, error) {
	// List the quads set or deleted between the graphs.
	quads, complete, err := changedGraphQuads(ctx, before, after, maxKeys)
	if err != nil || !complete {
		return world.NewUnknownChangeSet(), err
	}
	return &world.ChangeSet{Keys: keys, Quads: quads}, nil
}

// readObjectRecord reads the record of the object at key in an object tree.
// It returns nil when the tree holds no such object.
func readObjectRecord(ctx context.Context, tree kvtx.BlockTx, key string) (*Object, error) {
	bcs, err := tree.GetCursorAtKey(ctx, []byte(objectKeyPrefix+key))
	if err != nil || bcs == nil {
		return nil, err
	}
	return UnmarshalObject(ctx, bcs)
}

// ChangedObjectKeys lists the objects whose records differ between two World
// roots. It compares the object trees block by block and skips every subtree
// the roots share, so its cost follows the size of the change rather than the
// size of the World. complete is false when the change exceeds maxKeys or the
// comparison bound; the caller must then treat every object as changed.
func ChangedObjectKeys(ctx context.Context, before, after *block.Cursor, maxKeys int) ([]string, bool, error) {
	// Decode both World roots so their object tree sub-blocks resolve. A
	// missing root must fail rather than compare as an empty World.
	for _, bcs := range []*block.Cursor{before, after} {
		w, err := UnmarshalWorld(ctx, bcs)
		if err != nil {
			return nil, false, err
		}
		if w == nil {
			return nil, false, block.ErrNotFound
		}
	}

	// Compare the object trees, which hold one record per object key.
	keys, complete, err := kvtx_block.ChangedKeys(ctx, before.FollowSubBlock(1), after.FollowSubBlock(1), maxKeys)
	if err != nil || !complete {
		return nil, false, err
	}

	// Strip the storage prefix to recover the object keys.
	objKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if objKey, ok := bytes.CutPrefix(key, []byte(objectKeyPrefix)); ok {
			objKeys = append(objKeys, string(objKey))
		}
	}
	return objKeys, true, nil
}

// ChangedObjectKeysIn lists which of keys name objects whose records differ
// between two World roots. It compares only the parts of the object trees that
// can hold those keys, so it lists a change of any size.
func ChangedObjectKeysIn(ctx context.Context, before, after *block.Cursor, keys []string) ([]string, error) {
	// Decode both World roots so their object tree sub-blocks resolve.
	for _, bcs := range []*block.Cursor{before, after} {
		w, err := UnmarshalWorld(ctx, bcs)
		if err != nil {
			return nil, err
		}
		if w == nil {
			return nil, fmt.Errorf("changed keys World root: %w", block.NotFoundError(bcs.GetRef()))
		}
	}

	// Compare the object records of the listed keys.
	ranges := make([]kvtx_block.KeyRange, len(keys))
	for i, key := range keys {
		ranges[i] = kvtx_block.ExactKeyRange([]byte(objectKeyPrefix + key))
	}
	changed, err := kvtx_block.ChangedKeysIn(ctx, before.FollowSubBlock(1), after.FollowSubBlock(1), ranges)
	if err != nil {
		return nil, err
	}

	// Strip the storage prefix to recover the object keys.
	objKeys := make([]string, len(changed))
	for i, key := range changed {
		objKeys[i] = string(key[len(objectKeyPrefix):])
	}
	return objKeys, nil
}

// changedGraphQuads lists the quads set or deleted between the graphs of two
// decoded World roots, each quad in the version of each graph that holds it.
// complete is false when the change exceeds maxKeys or the comparison bound,
// or when the roots differ in format, since then every key differs.
func changedGraphQuads(ctx context.Context, before, after *block.Cursor, maxKeys int) ([]world.GraphQuad, bool, error) {
	// Compare the graph trees of two roots in one format.
	format, same, err := sharedFormat(ctx, before, after)
	if err != nil || !same {
		return nil, false, err
	}
	keys, complete, err := kvtx_block.ChangedKeys(ctx, before.FollowSubBlock(2), after.FollowSubBlock(2), maxKeys)
	if err != nil || !complete {
		return nil, false, err
	}
	if format == WorldFormat_WORLD_FORMAT_NODE_GRAPH {
		return changedNodeGraphQuads(ctx, before.FollowSubBlock(2), after.FollowSubBlock(2), keys, maxKeys)
	}

	// Resolve each changed link record in the graph that holds it. A record
	// present in only one graph names a quad set or deleted; one rewritten in
	// place names the quads of both versions.
	keys = slices.DeleteFunc(keys, func(key []byte) bool {
		return !bytes.HasPrefix(key, graphLogPrefix)
	})
	var quads []world.GraphQuad
	for _, bcs := range []*block.Cursor{before, after} {
		graphQuads, err := readGraphLinks(ctx, bcs.FollowSubBlock(2), keys)
		if err != nil {
			return nil, false, err
		}
		quads = append(quads, graphQuads...)
	}
	return quads, true, nil
}

// sharedFormat returns the format of two decoded World roots, with same false
// when they differ.
func sharedFormat(ctx context.Context, before, after *block.Cursor) (WorldFormat, bool, error) {
	var formats [2]WorldFormat
	for i, bcs := range []*block.Cursor{before, after} {
		w, err := UnmarshalWorld(ctx, bcs)
		if err != nil {
			return 0, false, err
		}
		if w == nil {
			return 0, false, block.ErrNotFound
		}
		formats[i] = w.GetFormat()
	}
	return formats[0], formats[0] == formats[1], nil
}

// changedNodeGraphQuads resolves the changed keys of two node graph trees to
// the quads they name, at most maxKeys of them.
func changedNodeGraphQuads(ctx context.Context, before, after *block.Cursor, keys [][]byte, maxKeys int) ([]world.GraphQuad, bool, error) {
	// Open both graph trees for reading.
	var txs [2]kvtx.BlockTx
	for i, bcs := range []*block.Cursor{before, after} {
		ktx, err := kvtx_block.BuildKvTransaction(ctx, bcs, false)
		if err != nil {
			return nil, false, err
		}
		defer ktx.Discard()
		txs[i] = ktx
	}

	// Resolve the changed keys to quads in each tree.
	cquads, complete, err := kvtx_cayley_nodegraph.ChangedQuads(ctx, txs[0], txs[1], keys, maxKeys)
	if err != nil || !complete {
		return nil, false, err
	}
	quads := make([]world.GraphQuad, len(cquads))
	for i, q := range cquads {
		quads[i] = world.CayleyQuadToGraphQuad(q)
	}
	return quads, true, nil
}

// readGraphLinks resolves the quads of the link records among the primitive log
// keys in the graph tree at bcs. A key the tree lacks, a node record, and a
// record marked deleted yield nothing.
func readGraphLinks(ctx context.Context, bcs *block.Cursor, keys [][]byte) ([]world.GraphQuad, error) {
	var quads []world.GraphQuad
	err := walkGraphLinks(ctx, bcs, keys, func(q quad.Quad) error {
		quads = append(quads, world.CayleyQuadToGraphQuad(q))
		return nil
	})
	return quads, err
}

// walkGraphLinks calls fn with the quad of each live link record among the
// primitive log keys in the graph tree at bcs, in keys' order. It reads each
// key once and resolves each link's nodes through the quad store.
func walkGraphLinks(ctx context.Context, bcs *block.Cursor, keys [][]byte, fn func(quad.Quad) error) error {
	// Open the graph tree for reading.
	if len(keys) == 0 {
		return nil
	}
	ktx, err := kvtx_block.BuildKvTransaction(ctx, bcs, false)
	if err != nil {
		return err
	}
	defer ktx.Discard()

	// Decode the live link records the tree holds.
	var links []*cayley_proto.Primitive
	for _, key := range keys {
		data, found, err := ktx.Get(ctx, key)
		if err != nil {
			return err
		}
		if !found {
			continue
		}
		p := &cayley_proto.Primitive{}
		if err := p.UnmarshalVT(data); err != nil {
			return err
		}
		if !p.IsNode() && !p.Deleted {
			links = append(links, p)
		}
	}
	if len(links) == 0 {
		return nil
	}

	// Resolve the node values of each link through the quad store.
	graphHd, err := kvtx_cayley.NewGraph(ctx, kvtx.NewTxStore(ktx), graph.Options{cayley_kv.OptAssumeDefaultIdx: true})
	if err != nil {
		return err
	}
	defer graphHd.Close()
	for _, p := range links {
		q, err := graphHd.Quad(ctx, p)
		if err != nil {
			return err
		}
		if err := fn(q); err != nil {
			return err
		}
	}
	return nil
}
