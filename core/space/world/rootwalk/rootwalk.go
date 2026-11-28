// Package rootwalk walks every block a Space retains, from its named roots
// down, and derives each block's children from its decoded content.
//
// A Space holds its blocks under the named roots roots.go in space_world
// lists. Each root holds a root block of the Space's own structure, such as a
// HeldRoots chain of Worlds, a RetainedRootSet or the payload roots of the
// operations above the checkpoint, stored as plain protobuf. Below those the
// blocks are Worlds and operation payloads, encoded with the World's
// transform, which a refwalk.Walker decodes by type. Walk reads the root
// blocks itself and hands every World and payload to the Walker, so a caller
// sees each block the Space retains once with the refs its content holds: the
// refs a write records as the block's child list.
//
// The walk streams: it passes each root block and each batch the Walker reads
// to the Visitor as it goes, and keeps only the Worlds and payload roots the
// root blocks name, the Walker's stack, and the Walker's set of visited
// blocks, which a graph with shared children needs to visit each block once.
package rootwalk

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"

	space_world "github.com/s4wave/spacewave/core/space/world"
)

// LegacyReplaySpanRoot is the name a Space engine before the Adapter held its
// replay span under, as a ReplaySpan chain of Worlds and payload roots.
const LegacyReplaySpanRoot = "replay-span"

// RootNames are the names a Space holds roots under. The payloads root holds
// operation payloads, the replay base a World, the retained root a
// RetainedRootSet and the legacy span a ReplaySpan chain. Every other root
// holds a HeldRoots chain of Worlds.
var RootNames = []string{
	space_world.RootReplayBase,
	space_world.RootCheckpoint,
	space_world.RootImport,
	space_world.RootReplayCursor,
	space_world.RootReplayPayloads,
	space_world.RootRetained,
	space_world.RootPending,
	space_world.RootBackfill,
	LegacyReplaySpanRoot,
}

// Space is what Walk reads one Space's retained blocks from.
type Space struct {
	// World is the bucket and transform of the Space's Worlds, which decode
	// every World and payload below the root blocks.
	World *bucket.ObjectRef
	// GetRoot reads the ref the Space holds under a root name, empty when it
	// holds none.
	GetRoot func(ctx context.Context, name string) (*block.BlockRef, error)
	// RootBlocks reads the root blocks, which are stored without a
	// transform.
	RootBlocks block.StoreOps
	// Cursor is the Space's saved replay, nil when the walk has none. Walk
	// also walks its base and head Worlds with their retained roots, and the
	// World and payloads of each outcome.
	Cursor *space_world.ReplayCursor
	// PublishedRoot names the root holding the Space's published graph, as
	// space_world.PublishedGraph builds it, empty when the Space has none.
	PublishedRoot string
	// ReleaseLegacySpan releases the legacy replay span root, which holds
	// ref, in place of walking the span. Nil walks the span.
	ReleaseLegacySpan func(ctx context.Context, ref *block.BlockRef) error
}
