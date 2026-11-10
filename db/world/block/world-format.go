package world_block

import (
	"context"

	"github.com/aperturerobotics/cayley"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_cayley "github.com/s4wave/spacewave/db/kvtx/cayley"
	kvtx_cayley_nodegraph "github.com/s4wave/spacewave/db/kvtx/cayley/nodegraph"
)

// NewestWorldFormat is the format new Worlds are created with when their
// creator asks for the newest.
const NewestWorldFormat = WorldFormat_WORLD_FORMAT_NODE_GRAPH

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

// openGraph opens the graph store of a World of format over store. committed
// is the transaction under store's write buffer, or store itself when it
// buffers nothing. The handle keeps ctx for its internal transactions.
func openGraph(ctx context.Context, format WorldFormat, settings *GraphSettings, store, committed kvtx.TxOps) (*cayley.Handle, error) {
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
		return kvtx_cayley_nodegraph.NewGraph(store, committed)
	}
	return nil, errors.Errorf("unknown World format %v", format)
}
