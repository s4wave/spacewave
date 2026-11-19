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
	regenPinName    = regenRootPrefix + "pin"
	regenBaseName   = regenRootPrefix + "base"
	regenBuiltName  = regenRootPrefix + "built"
)

// regenSwitchChanges bounds the quad changes a switch carries the pin's
// build forward by on the replay path. A graph settings step of this many
// links held a concurrent write 387 ms on a testbed Space, so the switch
// holds the writers no longer than a step does.
const regenSwitchChanges = maxGraphSettingsStep

// reasonRegenMoved refuses a switch whose head moved past regenSwitchChanges
// quad changes from the pin. The driver pins again and retries.
const reasonRegenMoved = "world regeneration: the World changed too much since the pin"

// reasonRegenPinDiffers refuses a switch whose pin another pin replaced. The
// driver pins again and retries.
const reasonRegenPinDiffers = "world regeneration: the pin differs"

// reasonReservedRoot refuses a SetRetainedRootOp that names a root a
// regeneration holds.
const reasonReservedRoot = "retained root name is reserved for world regeneration"

// defaultRegenBudget is the sort memory of a regeneration build when the
// adapter's options set none.
const defaultRegenBudget = 64 << 20

// A Space World regenerates through its agreed order in two operations, so
// the writers never wait for the build: a pin, then a switch. Every replica
// applies both, as SpaceWorldRegen.tla beside db/world/block's WorldRegen.tla
// models. Between them each replica's regenBuilder builds the pin off the
// replay path.
//
// Invariants of applyRegenerateWorld and applyReleaseWorldRegen:
//
//  1. The World after a switch holds exactly the object records and quads of
//     the head before it. The pin is the kv World plus the carried quads,
//     written as an ordinary commit, and stays the head; the switch builds
//     the pin, or takes this replica's prebuilt build of the same pin, and
//     carries it forward by every quad change from the pin to the head.
//  2. The World after an operation is a function of the World before and the
//     operation's bytes alone. The carried quads come from the list the
//     operation names. The switch refuses, alike on every replica, when the
//     pin it names is not the held one, when the changes from the pin to the
//     head exceed regenSwitchChanges, or when the pin's build is
//     world_block.ErrRegenInvalid; each is read from the state and the
//     content-addressed Worlds. A prebuilt root and an inline build of one
//     pin are the same root, since a build is a function of its input, so
//     whether this device built ahead changes only how long the switch takes.
//  3. Every root a regeneration needs stays held. The pin, and after the
//     switch the head before it as base and the installed head as built, are
//     retained roots of the state, which the apply holds under the retained
//     root before replay appends its position (Adapter.holdApplied); that root
//     holds every root the replay base, a position or the head retains, so
//     neither a cursor save, a checkpoint that covers the position before it
//     installs, nor a restart frees them. base and built stay held until a
//     switch back or an ordered release that names both, which a device
//     orders only after its comparison of the two passed. A prebuilt root is
//     held under the local regen-build root from before it is published until
//     the retained root holds the switch's head, which reaches its pages, or
//     the pin moves.
//  4. A replica that cannot complete the operation, for want of disk, a block
//     or its context, or for a failure of its store, stops replay before it
//     and retries; it never records a refusal. Replay refuses only on the
//     operation, the World and the held roots, which every replica reads
//     alike.
//  5. A switch back keeps every write ordered after the switch: base is the
//     head at the switch, not the pin, so the writes between the pin and the
//     switch are in it, and a write after the switch is carried back.
//  6. The quad list is a payload of the pin: the writer holds it before the
//     operation is queued, and every device holds it from the order's hold
//     until the checkpoint passes the operation, as it holds a transaction's
//     payloads.
//
// Producers into the apply: the replay of an ordered operation; a restored
// replay cursor or a checkpoint install, each a World another replica
// computed by this apply; no local writer's candidate, since the engine
// queues the operations with none, so every replica, the author included,
// replays them. Into the switch also the regenBuilder's prebuilt root, built
// from the pin the applied state names.

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

// Spill directories of the three sorters a Space runs. A sorter removes
// every run in its directory as it opens, so each directory has one sorter at
// a time: replay's inline builds run on the driver goroutine, the background
// builds on the regenBuilder's one routine, and the comparisons one at a time
// under the adapter's regenCompare.
const (
	regenSpillReplay  = "replay"
	regenSpillBuild   = "build"
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
func heldRegen(state *InnerState) (pin, base, built *block.BlockRef) {
	for _, root := range state.GetRetainedRoots() {
		switch root.GetName() {
		case regenPinName:
			pin = root.GetRootRef()
		case regenBaseName:
			base = root.GetRootRef()
		case regenBuiltName:
			built = root.GetRootRef()
		}
	}
	return pin, base, built
}

// applyRegenerateWorld pins, switches or switches back the World prev, as
// the invariants above require.
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
	pin, base, built := heldRegen(prev)
	target := regenOp.GetTarget()
	switch {
	case target == format:
		return nil, "world regeneration: the World is already " + format.String(), nil
	case target == world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH && format == world_block.WorldFormat_WORLD_FORMAT_CAYLEY_KV:
		if base != nil || built != nil {
			return nil, "world regeneration: a regeneration is already held", nil
		}
		if regenOp.GetBase().GetEmpty() {
			return r.regenPin(ctx, regenOp.GetQuads(), prev)
		}
		if pin == nil || !pin.EqualsRef(regenOp.GetBase()) {
			return nil, reasonRegenPinDiffers, nil
		}
		return r.regenSwitch(ctx, steps, pin, prev)
	case target == world_block.WorldFormat_WORLD_FORMAT_CAYLEY_KV && format == world_block.WorldFormat_WORLD_FORMAT_NODE_GRAPH:
		if base == nil || built == nil {
			return nil, "world regeneration: no old World is held", nil
		}
		next, err := steps.Carry(ctx, base, built, head)
		if err != nil {
			return nil, "", &regenGapError{err: err}
		}
		out := withRoot(prev, next)
		out.RetainedRoots = dropRegenRoots(out.GetRetainedRoots())
		return out, "", nil
	}
	return nil, "world regeneration: no step from " + format.String() + " to " + target.String(), nil
}

// regenPin adds the carried quads to the kv World prev and holds the result,
// which stays the head, as the pin; a pin while only a pin is held moves it.
func (r *spaceReplay) regenPin(ctx context.Context, quads *block.BlockRef, prev *InnerState) (*InnerState, string, error) {
	// Commit the carried quads on the World before.
	out := prev
	if !quads.GetEmpty() {
		var reason string
		var err error
		out, reason, err = r.commitOn(ctx, prev, "world regeneration failed", func(btx *world_block.EngineTx) (bool, error) {
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

	// Hold it as the pin.
	out, reason := setRetainedRoot(out, regenPinName, out.GetHeadRef().GetRootRef())
	if reason != "" {
		return nil, "world regeneration: " + reason, nil
	}
	return out, "", nil
}

// regenSwitch installs the node graph of the head of prev: this device's
// prebuilt build of pin, or one built inline when it holds none, carried
// forward by the changes from pin to the head. It holds the head before as
// base and the installed head as built, and releases the pin.
func (r *spaceReplay) regenSwitch(ctx context.Context, steps *world_block.OrderedRegen, pin *block.BlockRef, prev *InnerState) (*InnerState, string, error) {
	// Refuse a head that moved too far for the switch's bound.
	head := prev.GetHeadRef().GetRootRef()
	within, err := steps.ChangesWithin(ctx, pin, head, regenSwitchChanges)
	if err != nil {
		return nil, "", &regenGapError{err: err}
	}
	if !within {
		return nil, reasonRegenMoved, nil
	}

	// Take the prebuilt build of the pin, or build it inline. Both are the
	// same root, or the same refusal.
	built, invalid := r.a.regenBuild.lookup(pin)
	if built == nil && invalid == "" {
		built, err = steps.Build(ctx, pin, nil)
		if err != nil && errors.Is(err, world_block.ErrRegenInvalid) && !isReplayGap(err) {
			invalid = err.Error()
		} else if err != nil {
			return nil, "", &regenGapError{err: err}
		}
	}
	if invalid != "" {
		return nil, "world regeneration failed: " + invalid, nil
	}

	// Carry it forward to the head, install it, and hold both sides.
	next, err := steps.Carry(ctx, built, pin, head)
	if err != nil {
		return nil, "", &regenGapError{err: err}
	}
	out := withRoot(prev, next)
	for _, held := range []*RetainedRoot{{Name: regenPinName}, {Name: regenBaseName, RootRef: head}, {Name: regenBuiltName, RootRef: next}} {
		var reason string
		if out, reason = setRetainedRoot(out, held.GetName(), held.GetRootRef()); reason != "" {
			return nil, "world regeneration: " + reason, nil
		}
	}
	return out, "", nil
}

// applyReleaseWorldRegen releases the roots a regeneration holds when they are
// the ones the operation names: a pin, named as base with built empty, or base
// and built after a switch.
func applyReleaseWorldRegen(releaseOp *ReleaseWorldRegenOp, prev *InnerState) (*InnerState, string) {
	// Refuse a release of roots other than the held ones.
	pin, base, built := heldRegen(prev)
	if releaseOp.GetBuilt().GetEmpty() {
		if pin == nil || !pin.EqualsRef(releaseOp.GetBase()) {
			return nil, "world regeneration release: the held pin differs"
		}
	} else if base == nil || built == nil || !base.EqualsRef(releaseOp.GetBase()) || !built.EqualsRef(releaseOp.GetBuilt()) {
		return nil, "world regeneration release: the held roots differ"
	}

	// Release them.
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
