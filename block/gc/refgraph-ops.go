package block_gc

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
)

// RefEdge is a subject -> object edge in the GC ref graph.
type RefEdge struct {
	Subject, Object string
}

// RefGraphOps is the interface for GC reference graph operations.
type RefGraphOps interface {
	// AddRef adds a gc/ref edge from subject to object. Idempotent.
	AddRef(ctx context.Context, subject, object string) error
	// RemoveRef removes a single gc/ref edge from subject to object.
	RemoveRef(ctx context.Context, subject, object string) error
	// ApplyRefBatch applies ref graph edge additions followed by removals.
	// Implementations should batch these operations when possible.
	ApplyRefBatch(ctx context.Context, adds, removes []RefEdge) error
	// RemoveNodeRefs removes all outgoing gc/ref edges for a node.
	// Returns the list of target IRIs that lost an incoming edge.
	// If markOrphaned is true, targets with no remaining incoming
	// refs get an unreferenced edge.
	RemoveNodeRefs(ctx context.Context, node string, markOrphaned bool) ([]string, error)
	// HasIncomingRefs checks if a node has any incoming gc/ref edges.
	// Excludes edges from "unreferenced".
	HasIncomingRefs(ctx context.Context, node string) (bool, error)
	// GetOutgoingRefs returns all targets of gc/ref edges from a node.
	GetOutgoingRefs(ctx context.Context, node string) ([]string, error)
	// GetIncomingRefs returns all sources with gc/ref edges to a node.
	GetIncomingRefs(ctx context.Context, node string) ([]string, error)
	// GetUnreferencedNodes returns all nodes linked from "unreferenced".
	GetUnreferencedNodes(ctx context.Context) ([]string, error)
	// AddBlockRef adds gc/ref from source block to target block.
	AddBlockRef(ctx context.Context, source, target *block.BlockRef) error
	// AddObjectRoot adds gc/ref from object:{key} to block.
	AddObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error
	// RemoveObjectRoot removes gc/ref from object:{key} to block.
	RemoveObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error
	// Close closes the ref graph.
	Close() error
}

// _ is a type assertion
var _ RefGraphOps = ((*RefGraph)(nil))
