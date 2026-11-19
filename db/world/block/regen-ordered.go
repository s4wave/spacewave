package world_block

import (
	"context"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

// OrderedRegen runs the steps of a regeneration that an ordered log decides,
// as a Space World's replay applies RegenerateWorldOp on every replica. The
// log, not this type, holds the roots: each step reads the roots it is given
// and returns the root it wrote, publishing nothing. SpaceWorldRegen.tla
// models the lifecycle.
//
// Each step is a function of its input roots alone, invariant 4 of the
// build, so every replica that runs it writes the same root.
type OrderedRegen struct {
	// roots opens the World roots of the bucket.
	roots regenRoots
	// store is where the steps write, or nil for the cursor's own store.
	store block.StoreOps
	// cfg sets where the build and the comparison sort.
	cfg RegenConfig
}

// NewOrderedRegen returns the regeneration steps for the World roots of
// bucketCursor's bucket, written through store, or bucketCursor's transaction
// store when store is nil.
func NewOrderedRegen(bucketCursor *bucket_lookup.Cursor, store block.StoreOps, cfg RegenConfig) *OrderedRegen {
	return &OrderedRegen{roots: regenRoots{root: bucketCursor}, store: store, cfg: cfg}
}

// Format returns the stored format of the World at root.
func (o *OrderedRegen) Format(ctx context.Context, root *block.BlockRef) (WorldFormat, error) {
	_, w, err := o.roots.readWorld(ctx, o.store, root)
	return w.GetFormat(), err
}

// Build writes the node graph World of the cayley kv World at base and
// returns its root. It does not yield: the caller holds the World still.
func (o *OrderedRegen) Build(ctx context.Context, base *block.BlockRef) (*block.BlockRef, error) {
	return buildNodeGraphRoot(ctx, o.roots.root, o.store, base, o.cfg, nil)
}

// SwitchBack returns base, a cayley kv World, carried forward by the changes
// the node graph World made from built to head, as head's World in base's
// format. A change that does not apply exactly is skipped: with an exact
// build there is none, and after a build that lost a quad, skipping is what
// restores it from base.
func (o *OrderedRegen) SwitchBack(ctx context.Context, base, built, head *block.BlockRef) (*block.BlockRef, error) {
	ignore := graph.IgnoreOpts{IgnoreDup: true, IgnoreMissing: true}
	next, _, err := o.roots.applyDiff(ctx, o.store, base, built, head, -1, ignore, &regenPacer{})
	if err != nil {
		return nil, errors.Wrap(err, "switch back")
	}
	return next, nil
}

// Compare proves the node graph World at built holds every object record and
// quad of the World at base, and nothing else. A difference returns a wrapped
// ErrRegenMismatch.
func (o *OrderedRegen) Compare(ctx context.Context, base, built *block.BlockRef) error {
	return o.roots.compare(ctx, o.store, o.cfg, base, built)
}

// WorldRegenerator moves a World between stored formats with the old World
// held until a comparison releases it: the local Regenerator, or a Space World
// engine, which orders each step for every replica.
type WorldRegenerator interface {
	// RegenerateWorld moves the World to target: to the node graph, after
	// adding the quads of the quad list at quads, which may be empty; back to
	// cayley kv while the old World is held.
	RegenerateWorld(ctx context.Context, target WorldFormat, quads *block.BlockRef) error
	// ReleaseWorldRegen compares the held old World with the new one and
	// releases the old one when they are equal, or returns ErrRegenMismatch.
	ReleaseWorldRegen(ctx context.Context) error
}

// RegenerateWorld moves the World to target, as WorldRegenerator says. A
// local World carries no quads.
func (r *Regenerator) RegenerateWorld(ctx context.Context, target WorldFormat, quads *block.BlockRef) error {
	if !quads.GetEmpty() {
		return errors.New("a local World regeneration carries no quads")
	}
	switch target {
	case WorldFormat_WORLD_FORMAT_NODE_GRAPH:
		return r.Run(ctx, RegenSwitched, nil)
	case WorldFormat_WORLD_FORMAT_CAYLEY_KV:
		return r.SwitchBack(ctx)
	}
	return errors.Errorf("regenerate a World into %v", target)
}

// ReleaseWorldRegen verifies a switched World and releases its old root.
func (r *Regenerator) ReleaseWorldRegen(ctx context.Context) error {
	return r.Run(ctx, RegenDone, nil)
}

// _ is a type assertion
var _ WorldRegenerator = (*Regenerator)(nil)
