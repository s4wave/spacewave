package world_block

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_deferred "github.com/s4wave/spacewave/db/kvtx/deferred"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
)

// A regeneration keeps its state in two named roots beside the World root, so
// each step lands atomically with the roots it moves and a restart derives the
// stage from the roots alone:
//
//   - base holds the old-format root the built World reflects: the pin, then
//     the head each catch-up round reached. After the swap it is the old root,
//     held for the comparison and the switch back.
//   - built holds the new-format World: the build of base, then each round's
//     catch-up. After the swap it is the root the swap installed.
//
// Neither name holding a root is idle; base alone is building, restarted from
// the pin after a crash since the sort's runs are disposable; both are
// catching up while the head is in the old format and switched once it is in
// the new.

// regenChunkKeys is the most changed log records a catch-up reads, and the
// most quad changes it applies, before it writes the new World.
const regenChunkKeys = 1024

// errRegenDiffLarge stops a swap whose last diff exceeds the bound.
var errRegenDiffLarge = errors.New("regeneration diff exceeds the swap bound")

// RegenStage is the step a World's regeneration has reached.
type RegenStage int

const (
	// RegenIdle is no regeneration in progress.
	RegenIdle RegenStage = iota
	// RegenBuilding is a pinned root not yet built in the new format.
	RegenBuilding
	// RegenCatchingUp is a built World behind the old-format head.
	RegenCatchingUp
	// RegenSwitched is the new World serving as the head, with the old root
	// held.
	RegenSwitched
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
		return s, errors.New("regeneration holds a built root without its base")
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

// Run carries the World's regeneration from its current stage until the new
// World is the head, and returns at once for a World already in the node
// graph format. Each step lands in the roots before the next starts, so a Run
// after a crash resumes from the last step.
func (r *Regenerator) Run(ctx context.Context) error {
	// Trace the whole regeneration.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/regen/run")
	defer task.End()

	// Take the step each stage calls for until the swap.
	for {
		s, err := r.read(ctx)
		if err != nil {
			return err
		}
		switch s.stage {
		case RegenIdle:
			if s.format == WorldFormat_WORLD_FORMAT_NODE_GRAPH {
				return nil
			}
			err = r.pin(ctx, s)
		case RegenBuilding:
			err = r.build(ctx, s)
		case RegenCatchingUp:
			err = r.catchUp(ctx, s)
		case RegenSwitched:
			return nil
		}
		if err != nil {
			return err
		}
	}
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

// catchUp applies the diff from base to the head off the writer turn, then
// swaps when the diff was small enough that the last one should be too.
func (r *Regenerator) catchUp(ctx context.Context, s regenState) error {
	// Hold the head while the round reads it.
	release, err := block.PinRoot(ctx, r.a.store, s.head.Root)
	if err != nil {
		return err
	}
	_, changes, err := r.advance(ctx, s, -1, false)
	release()
	if err != nil || changes > r.cfg.SwapChanges {
		return err
	}
	return r.swap(ctx)
}

// swap applies the last diff inside the writer turn and installs the new World
// as the head, moving the World root and both regeneration roots at once. A
// last diff past the bound ends the turn and leaves the World catching up.
func (r *Regenerator) swap(ctx context.Context) error {
	// Take the turn, so the head holds still, and read the roots under it.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/regen/swap")
	defer task.End()
	turn, err := r.a.Begin(ctx)
	if err != nil {
		return err
	}
	defer turn.Release()
	s, err := r.read(ctx)
	if err != nil || s.stage != RegenCatchingUp {
		return err
	}

	// Apply the last diff and swap. The new World carries the head's
	// changelog, so the head keeps its seqno.
	next, _, err := r.advance(ctx, s, r.cfg.SwapChanges, true)
	if errors.Is(err, errRegenDiffLarge) {
		return nil
	}
	if err != nil {
		return err
	}
	turn.(*localTurn).published(ctx, world.Head{Root: next.Clone(), Seqno: s.head.Seqno, Durable: true})
	return nil
}

// advance applies the diff from base to the head to the built World under a
// stage, then moves base to the head and built to the result in one change,
// with the World root too when install is set. limit bounds the changed log
// records, or is negative for no bound. Returns the new built root and the
// number of changed log records.
func (r *Regenerator) advance(ctx context.Context, s regenState, limit int, install bool) (*block.BlockRef, int, error) {
	// Write the catch-up under a stage, held until the roots hold it.
	store, release, err := block.OpenStage(ctx, r.a.store)
	if err != nil {
		return nil, 0, err
	}
	defer release()
	next, changes, err := r.applyDiff(ctx, store, s, limit)
	if err != nil {
		return nil, 0, err
	}

	// Move the roots.
	roots := []block.NamedRoot{
		{Name: r.baseName, Ref: s.head.Root, Previous: s.base},
		{Name: r.builtName, Ref: next, Previous: s.built},
	}
	if install {
		roots = append(roots, block.NamedRoot{Name: r.a.name, Ref: next, Previous: s.head.Root})
	}
	if err := r.a.store.SetRoots(ctx, nil, roots); err != nil {
		return nil, 0, err
	}
	return next, changes, nil
}

// applyDiff writes the built World advanced from base to the head: the head's
// World with the built graph, then the quads the diff of the two old graphs'
// logs deletes, then those it adds. The deletes go first since a quad deleted
// and added again has a new log record, which can sort before the old one.
// Memory holds one chunk of keys and changes.
func (r *Regenerator) applyDiff(ctx context.Context, store block.StoreOps, s regenState, limit int) (*block.BlockRef, int, error) {
	// An unmoved head needs no catch-up.
	if s.head.Root.EqualsRef(s.base) {
		return s.built, 0, nil
	}

	// Graft the built graph onto the head's World.
	g, err := r.graft(ctx, store, s.head.Root, s.built)
	if err != nil {
		return nil, 0, err
	}

	// Open both old Worlds so their graph sub-blocks resolve.
	var cursors [2]*block.Cursor
	for i, ref := range []*block.BlockRef{s.base, s.head.Root} {
		if _, cursors[i], _, err = r.openWorld(ctx, store, ref); err != nil {
			return nil, 0, err
		}
	}

	// Delete the quads of the changed records in base, then add those of the
	// head, counting the records on the first pass.
	var changes int
	for i, action := range []graph.Procedure{graph.Delete, graph.Add} {
		err := walkChangedLinks(ctx, cursors[0], cursors[1], func(keys [][]byte) error {
			// Stop a diff past the bound before it applies.
			if i == 0 {
				changes += len(keys)
				if limit >= 0 && changes > limit {
					return errRegenDiffLarge
				}
			}

			// Apply the chunk's quads in the World that holds them.
			return walkGraphLinks(ctx, cursors[i].FollowSubBlock(2), keys, func(q quad.Quad) error {
				return g.apply(ctx, graph.Delta{Quad: q, Action: action})
			})
		})
		if err == nil {
			_, err = g.write(ctx)
		}
		if err != nil {
			return nil, 0, err
		}
	}
	return g.ref, changes, nil
}

// graft writes the head's World with the built World's graph tree, in the
// node graph format, and returns the delta writer on it.
func (r *Regenerator) graft(ctx context.Context, store block.StoreOps, head, built *block.BlockRef) (*regenGraft, error) {
	// Read the built World's graph tree and the head's World.
	_, _, bw, err := r.openWorld(ctx, store, built)
	if err != nil {
		return nil, errors.Wrap(err, "load built World")
	}
	tx, worldCursor, w, err := r.openWorld(ctx, store, head)
	if err != nil {
		return nil, errors.Wrap(err, "load head World")
	}

	// Replace the head World's graph with the built one.
	w.GraphKeyValue = bw.GetGraphKeyValue().CloneVT()
	w.Format = WorldFormat_WORLD_FORMAT_NODE_GRAPH
	w.GraphSettings = nil
	worldCursor.SetBlock(w, true)
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "write grafted World")
	}
	return &regenGraft{r: r, store: store, ref: ref}, nil
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

// walkChangedLinks calls fn with the changed primitive log keys of the graphs
// of the decoded Worlds before and after, in ascending chunks of at most
// regenChunkKeys. fn must not keep keys.
func walkChangedLinks(ctx context.Context, before, after *block.Cursor, fn func(keys [][]byte) error) error {
	// Stream the changed log keys, passing each full chunk on.
	ranges := []kvtx_block.KeyRange{kvtx_block.PrefixKeyRange(graphLogPrefix)}
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

// regenGraft applies quad changes to a node graph World in chunks, writing the
// World after each, so its memory holds one chunk.
type regenGraft struct {
	// r is the regenerator.
	r *Regenerator
	// store is the stage the World is written to.
	store block.StoreOps
	// ref is the World's last written root.
	ref *block.BlockRef
	// deltas are the changes not yet applied.
	deltas []graph.Delta
}

// apply adds a change, writing the World once a chunk is held.
func (g *regenGraft) apply(ctx context.Context, delta graph.Delta) error {
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
	handle, err := openGraph(ctx, WorldFormat_WORLD_FORMAT_NODE_GRAPH, nil, ops, ktx)
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
