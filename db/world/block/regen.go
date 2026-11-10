package world_block

import (
	"bytes"
	"context"
	"time"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/quad/pquads"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_cayley_nodegraph "github.com/s4wave/spacewave/db/kvtx/cayley/nodegraph"
	kvtx_deferred "github.com/s4wave/spacewave/db/kvtx/deferred"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
)

// A regeneration keeps its state in two named roots beside the World root, so
// each step lands atomically with the roots it moves and a restart derives the
// stage from the roots alone. WorldRegen.tla models the lifecycle; here one
// SetRoots moves the World root with both names, so the crash windows the
// model has between the Authority's decision and its record cannot occur.
//
//   - base holds the old-format World: the pin, then the head each catch-up
//     round reached. After the swap it is the old root, held for the
//     comparison and the switch back, which carries it forward by the new
//     format's commits.
//   - built holds the new-format World that reflects base: the build of the
//     pin, then each round's catch-up. After the swap it is the root the swap
//     installed, then the head each switch back round reached.
//
// The two always hold one content in two formats. Neither name holding a root
// is idle; base alone is building, restarted from the pin after a crash since
// the sort's runs are disposable; both are catching up while the head is in
// the old format and switched once it is in the new. built alone is switched
// back: the head is the old format again, and built keeps the new-format root
// it left for inspection until Release.

// regenChunkKeys is the most changed log records a catch-up reads, and the
// most quad changes it applies, before it writes the new World.
const regenChunkKeys = 1024

// errRegenDiffLarge stops a swap whose last diff exceeds the bound.
var errRegenDiffLarge = errors.New("regeneration diff exceeds the swap bound")

// ErrRegenMismatch reports a switched World whose new-format root does not
// hold what its old root holds. Run switches it back.
var ErrRegenMismatch = errors.New("regenerated World differs from the old root")

// ErrRegenSwitchedBack reports a regeneration that switched back and holds the
// new-format root it left, until Release.
var ErrRegenSwitchedBack = errors.New("regeneration switched back")

// regenWay is a direction through the catch-up.
type regenWay bool

const (
	// regenForward carries built forward by the old format's commits to base.
	regenForward regenWay = false
	// regenBack carries base forward by the new format's commits to built.
	regenBack regenWay = true
)

// RegenStage is the step a World's regeneration has reached.
type RegenStage int

const (
	// RegenIdle is an old-format World with no regeneration in progress.
	RegenIdle RegenStage = iota
	// RegenBuilding is a pinned root not yet built in the new format.
	RegenBuilding
	// RegenCatchingUp is a built World behind the old-format head.
	RegenCatchingUp
	// RegenSwitched is the new World serving as the head, with the old root
	// held.
	RegenSwitched
	// RegenSwitchedBack is the old format serving again, with the new-format
	// root it left held.
	RegenSwitchedBack
	// RegenDone is a World in the newest format with no regeneration roots.
	RegenDone
)

// String returns the stage's name.
func (s RegenStage) String() string {
	switch s {
	case RegenIdle:
		return "idle"
	case RegenBuilding:
		return "building"
	case RegenCatchingUp:
		return "catching up"
	case RegenSwitched:
		return "switched"
	case RegenSwitchedBack:
		return "switched back"
	case RegenDone:
		return "done"
	}
	return "unknown"
}

// Regenerator moves a World on a LocalAuthority from the cayley kv format to
// the node graph while writers commit. Only one Regenerator runs per World.
type Regenerator struct {
	// a is the World's authority.
	a *LocalAuthority
	// cfg is the build and swap configuration.
	cfg RegenConfig
	// baseName and builtName are the regeneration's roots.
	baseName, builtName string
}

// NewRegenerator returns the Regenerator of the World a governs.
func NewRegenerator(a *LocalAuthority, cfg RegenConfig) *Regenerator {
	return &Regenerator{
		a:         a,
		cfg:       cfg,
		baseName:  a.name + "/regen/base",
		builtName: a.name + "/regen/built",
	}
}

// regenState is what a regeneration reads to choose its next step.
type regenState struct {
	// stage is the step derived from the roots.
	stage RegenStage
	// head is the World's head.
	head world.Head
	// format is the head's format.
	format WorldFormat
	// base and built are the regeneration's roots, empty when free.
	base, built *block.BlockRef
}

// Stage returns the step the World's regeneration has reached.
func (r *Regenerator) Stage(ctx context.Context) (RegenStage, error) {
	s, err := r.read(ctx)
	return s.stage, err
}

// read reads the head and the regeneration's roots and derives the stage.
func (r *Regenerator) read(ctx context.Context) (regenState, error) {
	// Read the head.
	var s regenState
	var err error
	s.head, err = r.a.Head(ctx)
	if err != nil {
		return s, err
	}
	if s.head.Root.GetEmpty() {
		return s, errors.New("regenerate a World with no head")
	}

	// Read its format.
	_, _, w, err := r.openWorld(ctx, nil, s.head.Root)
	if err != nil {
		return s, err
	}
	s.format = w.GetFormat()

	// Read the regeneration's roots.
	if s.base, err = r.a.store.GetRoot(ctx, r.baseName); err != nil {
		return s, err
	}
	if s.built, err = r.a.store.GetRoot(ctx, r.builtName); err != nil {
		return s, err
	}

	// Derive the stage.
	switch {
	case s.base.GetEmpty() && !s.built.GetEmpty():
		s.stage = RegenSwitchedBack
	case s.base.GetEmpty() && s.format == WorldFormat_WORLD_FORMAT_NODE_GRAPH:
		s.stage = RegenDone
	case s.base.GetEmpty():
		s.stage = RegenIdle
	case s.built.GetEmpty():
		s.stage = RegenBuilding
	case s.format == WorldFormat_WORLD_FORMAT_NODE_GRAPH:
		s.stage = RegenSwitched
	default:
		s.stage = RegenCatchingUp
	}
	return s, nil
}

// Run carries the World's regeneration from its current stage to until,
// RegenSwitched or RegenDone, calling step, when set, with the stage each step
// reached and the time it took. Done releases the old root once the new World
// is proven equal to it. Each step lands in the roots before the next starts,
// so a Run after a crash resumes from the last step. A new World that differs
// from the old root is switched back and Run returns ErrRegenMismatch; a
// regeneration switched back returns ErrRegenSwitchedBack until Release.
func (r *Regenerator) Run(ctx context.Context, until RegenStage, step func(RegenStage, time.Duration)) error {
	// Trace the whole regeneration.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/regen/run")
	defer task.End()

	// Step until the stage asked for, switching back from a mismatch.
	stage, err := r.Stage(ctx)
	for err == nil && stage != until && stage != RegenDone {
		start := time.Now()
		stage, err = r.Step(ctx)
		if errors.Is(err, ErrRegenMismatch) {
			if backErr := r.SwitchBack(ctx); backErr != nil {
				return errors.Wrapf(backErr, "switch back after %v", err)
			}
		}
		if err == nil && step != nil {
			step(stage, time.Since(start))
		}
	}
	return err
}

// Step takes the one step the current stage calls for and returns the stage
// it reached: the pin from idle, the build, a catch-up round and the swap it
// may lead to, or the comparison and the release of the old root from
// switched. A comparison that finds a difference returns ErrRegenMismatch and
// stays switched; a switch back returns ErrRegenSwitchedBack; done takes no
// step.
func (r *Regenerator) Step(ctx context.Context) (RegenStage, error) {
	// Take the stage's step.
	s, err := r.read(ctx)
	if err != nil {
		return s.stage, err
	}
	switch s.stage {
	case RegenIdle:
		err = r.pin(ctx, s)
	case RegenBuilding:
		err = r.build(ctx, s)
	case RegenCatchingUp:
		err = r.catchUp(ctx, s, regenForward)
	case RegenSwitched:
		if err = r.verify(ctx, s); err == nil {
			err = r.release(ctx, s)
		}
	case RegenSwitchedBack:
		err = ErrRegenSwitchedBack
	}
	if err != nil {
		return s.stage, err
	}

	// Read the stage the step reached.
	s, err = r.read(ctx)
	return s.stage, err
}

// SwitchBack returns a switched World to the old format, the runtime switch
// back to it. Rounds off the writer turn carry the held old root forward by
// the new format's commits, then one bounded last diff and the switch run
// under the turn, as the swap does in the other direction. The new-format
// root stays held until Release.
func (r *Regenerator) SwitchBack(ctx context.Context) error {
	// Trace the switch back.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/regen/switch-back")
	defer task.End()

	// Run rounds until the switch lands.
	for {
		s, err := r.read(ctx)
		if err != nil {
			return err
		}
		switch s.stage {
		case RegenSwitched:
			err = r.catchUp(ctx, s, regenBack)
		case RegenSwitchedBack:
			return nil
		default:
			return errors.Errorf("switch back a regeneration %v", s.stage)
		}
		if err != nil {
			return err
		}
	}
}

// Release drops the regeneration's roots, ending it where it stands: a build
// or catch-up is abandoned while the old-format head keeps serving, and a
// switch back lets go of the new-format root it left. A switched World is
// refused, since its old root is the only proof of its content until Run
// verifies it or SwitchBack returns to it.
func (r *Regenerator) Release(ctx context.Context) error {
	s, err := r.read(ctx)
	if err != nil {
		return err
	}
	if s.stage == RegenSwitched {
		return errors.New("release a switched regeneration before it is verified")
	}
	return r.release(ctx, s)
}

// release frees both regeneration roots.
func (r *Regenerator) release(ctx context.Context, s regenState) error {
	// Free each name that holds a root.
	var roots []block.NamedRoot
	for _, root := range []block.NamedRoot{{Name: r.baseName, Previous: s.base}, {Name: r.builtName, Previous: s.built}} {
		if !root.Previous.GetEmpty() {
			roots = append(roots, root)
		}
	}
	if len(roots) == 0 {
		return nil
	}
	return r.a.store.SetRoots(ctx, nil, roots)
}

// pin holds the head as the base the build reads. The head was read just now,
// so the collector's grace keeps it until the root holds it.
func (r *Regenerator) pin(ctx context.Context, s regenState) error {
	return r.a.store.SetRoots(ctx, nil, []block.NamedRoot{{Name: r.baseName, Ref: s.head.Root}})
}

// build writes the new-format World of base under a stage and holds it as
// built.
func (r *Regenerator) build(ctx context.Context, s regenState) error {
	// Write the build under a stage, held until the root holds it.
	store, release, err := block.OpenStage(ctx, r.a.store)
	if err != nil {
		return err
	}
	defer release()
	built, err := buildNodeGraphRoot(ctx, r.a.root, store, s.base, r.cfg)
	if err != nil {
		return err
	}
	return r.a.store.SetRoots(ctx, nil, []block.NamedRoot{{Name: r.builtName, Ref: built}})
}

// catchUp runs a round off the writer turn in way, then swaps when the round
// was small enough that the last one should be too.
func (r *Regenerator) catchUp(ctx context.Context, s regenState, way regenWay) error {
	// Hold the head while the round reads it.
	release, err := block.PinRoot(ctx, r.a.store, s.head.Root)
	if err != nil {
		return err
	}
	_, changes, err := r.advance(ctx, s, way, -1, false)
	release()
	if err != nil || changes > r.cfg.SwapChanges {
		return err
	}
	return r.swap(ctx, way)
}

// swap applies the last diff in way inside the writer turn and installs the
// result as the head, moving the World root and both regeneration roots at
// once. A last diff past the bound ends the turn and leaves the stage as it
// was, for another round.
func (r *Regenerator) swap(ctx context.Context, way regenWay) error {
	// Trace the turn.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/regen/swap")
	defer task.End()

	// Take the turn, so the head holds still, and read the roots under it.
	turn, err := r.a.Begin(ctx)
	if err != nil {
		return err
	}
	defer turn.Release()
	s, err := r.read(ctx)
	want := RegenCatchingUp
	if way == regenBack {
		want = RegenSwitched
	}
	if err != nil || s.stage != want {
		return err
	}

	// Apply the last diff and swap. The new World carries the head's
	// changelog, so the head keeps its seqno.
	next, _, err := r.advance(ctx, s, way, r.cfg.SwapChanges, true)
	if errors.Is(err, errRegenDiffLarge) {
		return nil
	}
	if err != nil {
		return err
	}
	turn.(*localTurn).published(ctx, world.Head{Root: next.Clone(), Seqno: s.head.Seqno, Durable: true})
	return nil
}

// advance applies the diff from the root the trailing World reflects to the
// head onto the trailing World under a stage: built going forward, base going
// back. One change then moves the leading name to the head and the trailing
// one to the result, and with install the World root to the result, freeing
// base on a switch back. limit bounds the changes applied, or is negative for
// no bound. Returns the result and the number of changes applied.
func (r *Regenerator) advance(ctx context.Context, s regenState, way regenWay, limit int, install bool) (*block.BlockRef, int, error) {
	// Pick the trailing World and the root it reflects.
	trail, lead := block.NamedRoot{Name: r.builtName, Previous: s.built}, block.NamedRoot{Name: r.baseName, Previous: s.base}
	if way == regenBack {
		trail, lead = lead, trail
	}

	// Write the catch-up under a stage, held until the roots hold it.
	store, release, err := block.OpenStage(ctx, r.a.store)
	if err != nil {
		return nil, 0, err
	}
	defer release()
	next, changes, err := r.applyDiff(ctx, store, trail.Previous, lead.Previous, s.head.Root, limit)
	if err != nil {
		return nil, 0, err
	}

	// Move the roots.
	lead.Ref, trail.Ref = s.head.Root, next
	roots := []block.NamedRoot{lead, trail}
	if install {
		roots = append(roots, block.NamedRoot{Name: r.a.name, Ref: next, Previous: s.head.Root})
		if way == regenBack {
			roots[1].Ref = nil
		}
	}
	if err := r.a.store.SetRoots(ctx, nil, roots); err != nil {
		return nil, 0, err
	}
	return next, changes, nil
}

// applyDiff writes target advanced from the World from to the World to, both
// in the other format: to's World with target's graph, then the quads the
// diff of from's and to's graphs deletes, then those it adds. The deletes go
// first since a quad deleted and added again can show in the diff as an add
// that sorts before its delete. Memory holds one chunk of keys and changes.
func (r *Regenerator) applyDiff(ctx context.Context, store block.StoreOps, target, from, to *block.BlockRef, limit int) (*block.BlockRef, int, error) {
	// An unmoved head needs no catch-up.
	if to.EqualsRef(from) {
		return target, 0, nil
	}

	// Graft target's graph onto to's World.
	g, err := r.graft(ctx, store, to, target)
	if err != nil {
		return nil, 0, err
	}
	g.limit = limit

	// Open both source Worlds so their graph sub-blocks resolve.
	var cursors [2]*block.Cursor
	var format WorldFormat
	for i, ref := range []*block.BlockRef{from, to} {
		var w *World
		if _, cursors[i], w, err = r.openWorld(ctx, store, ref); err != nil {
			return nil, 0, err
		}
		format = w.GetFormat()
	}

	// Delete the quads only from holds, then add those only to holds.
	for _, action := range []graph.Procedure{graph.Delete, graph.Add} {
		err := walkQuadChanges(ctx, format, cursors[0], cursors[1], action, func(q quad.Quad) error {
			return g.apply(ctx, graph.Delta{Quad: q, Action: action})
		})
		if err == nil {
			_, err = g.write(ctx)
		}
		if err != nil {
			return nil, 0, err
		}
	}
	return g.ref, g.changes, nil
}

// walkQuadChanges calls fn with the quads of one pass of a catch-up between
// the graphs of the decoded Worlds before and after, both in format: on the
// delete pass those only before holds, on the add pass those only after
// holds. It streams the tree diff in chunks of regenChunkKeys.
func walkQuadChanges(ctx context.Context, format WorldFormat, before, after *block.Cursor, action graph.Procedure, fn func(quad.Quad) error) error {
	switch format {
	case WorldFormat_WORLD_FORMAT_CAYLEY_KV:
		// A changed log record names its quad in the World that holds it. An
		// old store can hold several physical copies of one quad, and one
		// logical write changes them all in the same commit, so the pass
		// passes each quad once; seen grows with the round's changes.
		side := before
		if action == graph.Add {
			side = after
		}
		seen := make(map[string]struct{})
		return walkChangedKeys(ctx, before, after, graphLogPrefix, func(keys [][]byte) error {
			return walkGraphLinks(ctx, side.FollowSubBlock(2), keys, func(q quad.Quad) error {
				// Pass the quad on at its first copy only.
				key, err := pquads.MakeQuad(q).MarshalVT()
				if err != nil {
					return err
				}
				if _, ok := seen[string(key)]; ok {
					return nil
				}
				seen[string(key)] = struct{}{}
				return fn(q)
			})
		})
	case WorldFormat_WORLD_FORMAT_NODE_GRAPH:
		// A changed out key names its quad in the tree that holds it.
		var txs [2]kvtx.BlockTx
		for i, c := range []*block.Cursor{before, after} {
			tx, err := kvtx_block.BuildKvTransaction(ctx, c.FollowSubBlock(2), false)
			if err != nil {
				return err
			}
			defer tx.Discard()
			txs[i] = tx
		}
		return walkChangedKeys(ctx, before, after, kvtx_cayley_nodegraph.NodeKeyPrefix(), func(keys [][]byte) error {
			return kvtx_cayley_nodegraph.WalkChangedQuads(ctx, txs[0], txs[1], keys, func(q quad.Quad, added bool) error {
				if added != (action == graph.Add) {
					return nil
				}
				return fn(q)
			})
		})
	}
	return errors.Errorf("catch up from a %v World", format)
}

// graft writes the head's World with target's graph tree, format and graph
// settings, and returns the delta writer on it.
func (r *Regenerator) graft(ctx context.Context, store block.StoreOps, head, target *block.BlockRef) (*regenGraft, error) {
	// Read target's graph tree and the head's World.
	_, _, tw, err := r.openWorld(ctx, store, target)
	if err != nil {
		return nil, errors.Wrap(err, "load trailing World")
	}
	tx, worldCursor, w, err := r.openWorld(ctx, store, head)
	if err != nil {
		return nil, errors.Wrap(err, "load head World")
	}

	// Replace the head World's graph with target's.
	w.GraphKeyValue = tw.GetGraphKeyValue().CloneVT()
	w.Format = tw.GetFormat()
	w.GraphSettings = tw.GetGraphSettings().CloneVT()
	worldCursor.SetBlock(w, true)
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "write grafted World")
	}
	return &regenGraft{r: r, store: store, ref: ref, format: w.Format, settings: w.GraphSettings}, nil
}

// openWorld opens a transaction on the World root at ref, writing through
// store, or the bucket's transaction store when store is nil, and decodes it.
func (r *Regenerator) openWorld(ctx context.Context, store block.StoreOps, ref *block.BlockRef) (*block.Transaction, *block.Cursor, *World, error) {
	tx, cursor := r.a.root.BuildTransactionAtRefWithStore(nil, ref, store)
	w, err := UnmarshalWorld(ctx, cursor)
	if err == nil && w == nil {
		err = block.NotFoundError(ref)
	}
	return tx, cursor, w, err
}

// walkChangedKeys calls fn with the changed keys under prefix of the graph
// trees of the decoded Worlds before and after, in ascending chunks of at most
// regenChunkKeys. fn must not keep keys.
func walkChangedKeys(ctx context.Context, before, after *block.Cursor, prefix []byte, fn func(keys [][]byte) error) error {
	// Stream the changed keys, passing each full chunk on.
	ranges := []kvtx_block.KeyRange{kvtx_block.PrefixKeyRange(prefix)}
	keys := make([][]byte, 0, regenChunkKeys)
	err := kvtx_block.WalkChangedKeysIn(ctx, before.FollowSubBlock(2), after.FollowSubBlock(2), ranges, func(key []byte) error {
		// Hold the key until its chunk is full.
		keys = append(keys, bytes.Clone(key))
		if len(keys) < regenChunkKeys {
			return nil
		}
		err := fn(keys)
		keys = keys[:0]
		return err
	})

	// Pass on the last chunk.
	if err == nil && len(keys) != 0 {
		err = fn(keys)
	}
	return err
}

// regenGraft applies quad changes to a World in chunks, writing the World
// after each, so its memory holds one chunk.
type regenGraft struct {
	// r is the regenerator.
	r *Regenerator
	// store is the stage the World is written to.
	store block.StoreOps
	// ref is the World's last written root.
	ref *block.BlockRef
	// format and settings open the World's graph.
	format   WorldFormat
	settings *GraphSettings
	// limit is the most changes to apply, or negative for no bound.
	limit int
	// changes counts the changes applied or held.
	changes int
	// deltas are the changes not yet applied.
	deltas []graph.Delta
}

// apply adds a change, writing the World once a chunk is held. A change past
// the limit stops the catch-up before it applies.
func (g *regenGraft) apply(ctx context.Context, delta graph.Delta) error {
	// Count the change against the bound, then hold it.
	g.changes++
	if g.limit >= 0 && g.changes > g.limit {
		return errRegenDiffLarge
	}
	g.deltas = append(g.deltas, delta)
	if len(g.deltas) < regenChunkKeys {
		return nil
	}
	_, err := g.write(ctx)
	return err
}

// write applies the held changes and writes the World, returning its root.
// The changes must apply exactly: a quad deleted that the graph lacks, or
// added that it holds, is a fault in the catch-up and fails it.
func (g *regenGraft) write(ctx context.Context) (*block.BlockRef, error) {
	// Nothing held leaves the root as written.
	if len(g.deltas) == 0 {
		return g.ref, nil
	}

	// Open the graph tree through deferred writes, so the tree rehashes once.
	tx, worldCursor, _, err := g.r.openWorld(ctx, g.store, g.ref)
	if err != nil {
		return nil, err
	}
	ktx, err := kvtx_block.BuildKvTransaction(ctx, worldCursor.FollowSubBlock(2), true)
	if err != nil {
		return nil, err
	}
	defer ktx.Discard()
	var ops kvtx.TxOps = ktx
	var deferred *kvtx_deferred.Ops
	if batch, ok := ktx.(kvtx.WriteBatchTxOps); ok {
		deferred = kvtx_deferred.NewOps(ktx, batch, 0, 0)
		ops = deferred
	}

	// Apply the changes to the tree.
	handle, err := openGraph(ctx, g.format, g.settings, ops)
	if err != nil {
		return nil, err
	}
	err = handle.ApplyDeltas(ctx, g.deltas, graph.IgnoreOpts{})
	_ = handle.Close()
	if err == nil && deferred != nil {
		err = deferred.Flush(ctx)
	}
	if err != nil {
		return nil, errors.Wrap(err, "apply catch-up")
	}

	// Write the World.
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "write caught-up World")
	}
	g.ref, g.deltas = ref, g.deltas[:0]
	return ref, nil
}
