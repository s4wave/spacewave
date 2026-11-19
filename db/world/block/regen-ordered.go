package world_block

import (
	"context"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
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
// returns its root. It runs on the calling goroutine, sorting in the
// config's budget, and calls yield, when set, between short slices: a build
// off the replay path yields to the writers, and an inline build on the
// replay path, which the writers wait behind already, passes nil.
func (o *OrderedRegen) Build(ctx context.Context, base *block.BlockRef, yield func(context.Context) error) (*block.BlockRef, error) {
	return buildNodeGraphRoot(ctx, o.roots.root, o.store, base, o.cfg, yield)
}

// Carry returns target carried forward by the changes the World made from
// from to to, as to's World in target's format: a switch carries the prebuilt
// node graph of a pinned kv World to the kv head, and a switch back carries
// the held kv base to the node graph head. A change that does not apply
// exactly is skipped: with an exact build there is none, and after a build
// that lost a quad, skipping is what a switch back needs to restore it from
// base, and what every replica's switch does alike.
func (o *OrderedRegen) Carry(ctx context.Context, target, from, to *block.BlockRef) (*block.BlockRef, error) {
	ignore := graph.IgnoreOpts{IgnoreDup: true, IgnoreMissing: true}
	next, _, err := o.roots.applyDiff(ctx, o.store, target, from, to, -1, ignore, &regenPacer{})
	if err != nil {
		return nil, errors.Wrap(err, "carry World forward")
	}
	return next, nil
}

// ChangesWithin reports whether the quad changes from the World at from to
// the World at to, both in one format, number at most limit. It reads both
// graphs and writes nothing, stopping at the first change past limit, and
// counts as Carry applies: each quad once per pass. The count is a function
// of the two roots alone, so every replica decides alike.
func (o *OrderedRegen) ChangesWithin(ctx context.Context, from, to *block.BlockRef, limit int) (bool, error) {
	// An unmoved World has no changes.
	if from.EqualsRef(to) {
		return true, nil
	}

	// Open both Worlds so their graph sub-blocks resolve.
	var cursors [2]*block.Cursor
	var formats [2]WorldFormat
	for i, ref := range []*block.BlockRef{from, to} {
		cursor, w, err := o.roots.readWorld(ctx, o.store, ref)
		if err != nil {
			return false, err
		}
		cursors[i], formats[i] = cursor, w.GetFormat()
	}
	if formats[0] != formats[1] {
		return false, errors.Errorf("count changes from a %v World to a %v one", formats[0], formats[1])
	}

	// Count the deletes, then the adds, stopping past the limit.
	var changes int
	for _, action := range []graph.Procedure{graph.Delete, graph.Add} {
		err := walkQuadChanges(ctx, formats[0], cursors[0], cursors[1], action, func(quad.Quad) error {
			changes++
			if changes > limit {
				return errRegenDiffLarge
			}
			return nil
		})
		if errors.Is(err, errRegenDiffLarge) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
	return true, nil
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
	// CompareWorldRegen compares the held old World with the new one and
	// returns ErrRegenMismatch when they differ, releasing nothing, so a
	// caller can act on the proof while the old World is still held to go
	// back to.
	CompareWorldRegen(ctx context.Context) error
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

// CompareWorldRegen verifies a switched World, releasing nothing.
func (r *Regenerator) CompareWorldRegen(ctx context.Context) error {
	s, err := r.read(ctx)
	if err != nil {
		return err
	}
	if s.stage != RegenSwitched {
		return errors.Errorf("compare a World regeneration at stage %v", s.stage)
	}
	return r.verify(regenBackground(ctx), s)
}

// ReleaseWorldRegen verifies a switched World and releases its old root.
func (r *Regenerator) ReleaseWorldRegen(ctx context.Context) error {
	return r.Run(ctx, RegenDone, nil)
}

// _ is a type assertion
var _ WorldRegenerator = (*Regenerator)(nil)
