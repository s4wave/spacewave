package space_world

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	bquad "github.com/s4wave/spacewave/db/block/quad"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// The retained roots a regeneration holds. Only RegenerateWorldOp and
// ReleaseWorldRegenOp move them; SetRetainedRootOp refuses every name under
// regenRootPrefix.
const (
	regenRootPrefix = "world-regen/"
	regenBaseName   = regenRootPrefix + "base"
	regenBuiltName  = regenRootPrefix + "built"
)

// reasonReservedRoot refuses a SetRetainedRootOp that names a root a
// regeneration holds.
const reasonReservedRoot = "retained root name is reserved for world regeneration"

// defaultRegenBudget is the sort memory of a regeneration build when the
// adapter's options set none.
const defaultRegenBudget = 64 << 20

// A Space World regenerates through its agreed order: every replica applies
// RegenerateWorldOp to the World before it, as SpaceWorldRegen.tla beside
// db/world/block's WorldRegen.tla models.
//
// Invariants of applyRegenerateWorld and applyReleaseWorldRegen:
//
//  1. The World after a regeneration to the node graph holds exactly the
//     object records and quads of the World before it, plus the carried
//     quads. The carried quads are written first, as an ordinary commit on
//     the World before, and that commit's World is base, which the build
//     reads whole; so the build adds nothing a comparison of base and built
//     cannot see.
//  2. The World after is a function of the World before and the operation's
//     bytes alone: the carried quads come from the list the operation names,
//     the commit runs as every replica runs it, and the build's output does
//     not depend on the device's sort budget or spill directory.
//  3. base stays held under world-regen/base, beside built under
//     world-regen/built, until a switch back or an ordered release that
//     names both roots. A release applies only while both names hold the
//     roots it names, and a device orders one only after its comparison of
//     the two passed. From the apply until the head that retains them
//     installs, the position's state retains them, and the collector counts
//     every position's retained roots as live.
//  4. A replica that cannot complete the operation, for want of disk, a block
//     or its context, or for a failure of its store, stops replay before it
//     and retries; it never records a refusal. Replay refuses only on the
//     operation, the World and the held roots, which every replica reads
//     alike: a target equal to the current format, a regeneration already
//     held, a switch back with nothing held, a quad list that does not
//     decode, a carried quad the World refuses, or a base the build refuses
//     as world_block.ErrRegenInvalid.
//  5. A switch back keeps every write ordered after the regeneration: base
//     holds the carried quads, so a write that set one of them, a no-op on the
//     node graph, is not lost when base comes back.
//  6. The quad list is a payload of the operation: the writer holds it before
//     the operation is queued, and every device holds it from the order's
//     hold until the checkpoint passes the operation, as it holds a
//     transaction's payloads.
//
// Producers into the apply: the replay of an ordered operation; a restored
// replay cursor or a checkpoint install, each a World another replica
// computed by this apply; no local writer's candidate, since the engine
// queues the operation with none, so every replica, the author included,
// replays it.

// regenGapError is a regeneration step this device could not complete. Replay
// stops before the operation and retries, by invariant 4.
type regenGapError struct {
	err error
}

// Error returns the step's failure.
func (e *regenGapError) Error() string {
	return "world regeneration stopped: " + e.err.Error()
}

// Unwrap returns the step's failure.
func (e *regenGapError) Unwrap() error {
	return e.err
}

// Spill directories of the two sorters a Space runs. A sorter removes every
// run in its directory as it opens, so each directory has one sorter at a
// time: replay's builds and switch backs run on the driver goroutine, and
// the comparisons run one at a time under the adapter's regenCompare.
const (
	regenSpillReplay  = "replay"
	regenSpillCompare = "compare"
)

// regenConfig returns the adapter's build configuration for the sorter of
// role, with a spill directory of its own and the default budget where its
// options set none.
func (a *Adapter) regenConfig(role string) world_block.RegenConfig {
	// Default the spill directory to one per Space under the temp dir.
	cfg := a.opts.Regen
	if cfg.SpillDir == "" {
		dir := url.PathEscape(a.bucketID)
		if a.opts.transient {
			dir += "-transient"
		}
		cfg.SpillDir = filepath.Join(os.TempDir(), "spacewave-world-regen", dir)
	}

	// Give each sorter role its own directory, since a sorter clears its
	// directory's runs when it starts.
	cfg.SpillDir = filepath.Join(cfg.SpillDir, role)
	if cfg.Budget == 0 {
		cfg.Budget = defaultRegenBudget
	}
	return cfg
}

// heldRegen returns the roots a regeneration holds in state, each nil when
// its name holds none.
func heldRegen(state *InnerState) (base, built *block.BlockRef) {
	for _, root := range state.GetRetainedRoots() {
		switch root.GetName() {
		case regenBaseName:
			base = root.GetRootRef()
		case regenBuiltName:
			built = root.GetRootRef()
		}
	}
	return base, built
}

// applyRegenerateWorld rebuilds the World prev in the operation's target
// format, as the invariants above require.
func (r *spaceReplay) applyRegenerateWorld(ctx context.Context, regenOp *RegenerateWorldOp, prev *InnerState) (*InnerState, string, error) {
	// A regeneration needs a World.
	if prev.GetHeadRef().GetEmpty() {
		return nil, "world is not initialized", nil
	}

	// Bind the replay cursor at the World and read its format.
	if _, err := r.bind(ctx, prev.GetHeadRef()); err != nil {
		return nil, "", err
	}
	steps := world_block.NewOrderedRegen(r.cursor, r.store(), r.a.regenConfig(regenSpillReplay))
	head := prev.GetHeadRef().GetRootRef()
	format, err := steps.Format(ctx, head)
	if err != nil {
		return nil, "", &regenGapError{err: err}
	}

	// Refuse what this World cannot take, then take the step.
	base, built := heldRegen(prev)
	target := regenOp.GetTarget()
	switch {
	case target == format:
		return nil, "world regeneration: the World is already " + format.String(), nil
	case target == world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH && format == world_block.WorldFormat_WORLD_FORMAT_CAYLEY_KV:
		if base != nil || built != nil {
			return nil, "world regeneration: a regeneration is already held", nil
		}
		return r.regenerateForward(ctx, steps, regenOp.GetQuads(), prev)
	case target == world_block.WorldFormat_WORLD_FORMAT_CAYLEY_KV && format == world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH:
		if base == nil || built == nil {
			return nil, "world regeneration: no old World is held", nil
		}
		next, err := steps.SwitchBack(ctx, base, built, head)
		if err != nil {
			return nil, "", &regenGapError{err: err}
		}
		out := withRoot(prev, next)
		out.RetainedRoots = dropRegenRoots(out.GetRetainedRoots())
		return out, "", nil
	}
	return nil, "world regeneration: no step from " + format.String() + " to " + target.String(), nil
}

// regenerateForward adds the carried quads to the kv World prev as base, builds
// base as a node graph, installs the build, and holds both.
func (r *spaceReplay) regenerateForward(ctx context.Context, steps *world_block.OrderedRegen, quads *block.BlockRef, prev *InnerState) (*InnerState, string, error) {
	// Commit the carried quads on the World before, which becomes base.
	baseState := prev
	if !quads.GetEmpty() {
		var reason string
		var err error
		baseState, reason, err = r.commitOn(ctx, prev, "world regeneration failed", func(btx *world_block.EngineTx) (bool, error) {
			_, bcs := r.cursor.BuildTransactionAtRefWithStore(nil, quads, r.store())
			err := RangeQuadList(ctx, bcs, func(q *bquad.Quad) error {
				return btx.SetGraphQuad(ctx, world.QuadToGraphQuad(q))
			})
			return err == nil, err
		})
		if err != nil || reason != "" {
			return nil, reason, err
		}
	}
	base := baseState.GetHeadRef().GetRootRef()

	// Build base as a node graph. A base every replica fails to build alike
	// refuses the operation; any other failure waits at it.
	built, err := steps.Build(ctx, base)
	if err != nil && errors.Is(err, world_block.ErrRegenInvalid) && !isReplayGap(err) {
		return nil, "world regeneration failed: " + err.Error(), nil
	}
	if err != nil {
		return nil, "", &regenGapError{err: err}
	}

	// Install the build and hold both roots.
	out := withRoot(baseState, built)
	for _, held := range []*RetainedRoot{{Name: regenBaseName, RootRef: base}, {Name: regenBuiltName, RootRef: built}} {
		var reason string
		if out, reason = setRetainedRoot(out, held.GetName(), held.GetRootRef()); reason != "" {
			return nil, "world regeneration: " + reason, nil
		}
	}
	return out, "", nil
}

// applyReleaseWorldRegen releases the roots a regeneration holds when they are
// the ones the operation names.
func applyReleaseWorldRegen(releaseOp *ReleaseWorldRegenOp, prev *InnerState) (*InnerState, string) {
	// Refuse a release of roots other than the held ones.
	base, built := heldRegen(prev)
	if base == nil || built == nil || !base.EqualsRef(releaseOp.GetBase()) || !built.EqualsRef(releaseOp.GetBuilt()) {
		return nil, "world regeneration release: the held roots differ"
	}

	// Release both.
	out := prev.CloneVT()
	out.RetainedRoots = dropRegenRoots(out.GetRetainedRoots())
	return out, ""
}

// withRoot returns the head of state with its World root at root.
func withRoot(state *InnerState, root *block.BlockRef) *InnerState {
	out := state.CloneVT()
	out.HeadRef.RootRef = root.Clone()
	return out
}

// dropRegenRoots returns roots without a regeneration's, keeping the order.
func dropRegenRoots(roots []*RetainedRoot) []*RetainedRoot {
	out := make([]*RetainedRoot, 0, len(roots))
	for _, root := range roots {
		if !strings.HasPrefix(root.GetName(), regenRootPrefix) {
			out = append(out, root)
		}
	}
	return out
}
