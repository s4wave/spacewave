package world_block

import (
	"bytes"
	"context"
	"fmt"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/quad/pquads"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/util/extsort"
	"github.com/s4wave/spacewave/db/world"
)

// regenVerifyQuadBytes is the run bytes the comparison writes per quad: its
// encoded text, estimated at 120 bytes, and the side tag.
const regenVerifyQuadBytes = 128

// regenVerifyShown is the most differing quads a mismatch names.
const regenVerifyShown = 4

// verify proves built holds what base holds: the same World apart from the
// graph, and the same quads. The two are one content in two formats, held by
// name, so commits on the head since the swap do not enter the comparison.
// Both graphs stream every quad, tagged by side, into one external sort, and
// the merge must meet each quad from both sides. Memory holds cfg.Budget and
// the sort's read buffers; disk holds the runs, reserved before the first.
// A difference returns a wrapped ErrRegenMismatch.
func (r *Regenerator) verify(ctx context.Context, s regenState) error {
	// Trace the whole comparison.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/regen/verify")
	defer task.End()

	// Each side's trees must carry its format's tags.
	sides := [2]*block.BlockRef{s.base, s.built}
	var worlds [2]*World
	var cursors [2]*block.Cursor
	for i, ref := range sides {
		var err error
		if cursors[i], worlds[i], err = r.readWorld(ctx, r.a.store, ref); err != nil {
			return err
		}
		if err := worlds[i].checkTreeImpls(); err != nil {
			return errors.Wrap(ErrRegenMismatch, err.Error())
		}
	}

	// Compare the Worlds apart from their graphs, with the object tree's tag
	// normalised to a kv World's. The object tree and the changelog are
	// content addressed, so equal refs hold equal records.
	var stripped [2]*World
	for i, w := range worlds {
		w = w.CloneVT()
		w.GraphKeyValue, w.GraphSettings = nil, nil
		if err := w.setFormat(WorldFormat_WORLD_FORMAT_CAYLEY_KV); err != nil {
			return errors.Wrap(ErrRegenMismatch, err.Error())
		}
		stripped[i] = w
	}
	if !stripped[0].EqualVT(stripped[1]) {
		return errors.Wrap(ErrRegenMismatch, "Worlds differ outside the graph")
	}

	// Sort every quad of both graphs, tagged by side.
	sorter, err := r.cfg.newSorter()
	if err != nil {
		return err
	}
	defer sorter.Close()
	for i := range sides {
		if err := addVerifyQuads(ctx, cursors[i], worlds[i], byte(i), sorter, int64(r.cfg.Budget)); err != nil {
			return err
		}
	}

	// Each quad must show on both sides.
	var tally verifyTally
	err = sorter.Merge(ctx, nil, func(key, value []byte) error {
		tally.add(key, value[0])
		return nil
	})
	if err != nil {
		return err
	}
	tally.flush()
	if tally.differ != 0 {
		return errors.Wrapf(ErrRegenMismatch, "%d quads differ, such as %v", tally.differ, tally.shown)
	}
	return nil
}

// verifyTally counts the sides of each quad as the comparison's merge meets
// them in key order, so it holds one quad at a time.
type verifyTally struct {
	// key is the quad being counted, valid when ok.
	key []byte
	ok  bool
	// count is how many records of key each side gave.
	count [2]int
	// differ counts the quads one side lacks. A side may give a quad more than
	// once, as an old kv store's physical copies of it.
	differ int
	// shown names the first few of them.
	shown []string
}

// add counts a record of key from side, closing the previous quad when key
// differs from it.
func (t *verifyTally) add(key []byte, side byte) {
	if !t.ok || !bytes.Equal(t.key, key) {
		t.flush()
		t.key, t.ok, t.count = append(t.key[:0], key...), true, [2]int{}
	}
	t.count[side]++
}

// flush closes the quad being counted.
func (t *verifyTally) flush() {
	if !t.ok || (t.count[0] != 0 && t.count[1] != 0) {
		return
	}
	t.differ++
	if len(t.shown) < regenVerifyShown {
		t.shown = append(t.shown, describeVerifyQuad(t.key, t.count))
	}
}

// addVerifyQuads adds every quad of the graph of the decoded World w at
// cursor to sorter with value side, reserving the runs' disk from the quad
// count once the sort would spill.
func addVerifyQuads(ctx context.Context, cursor *block.Cursor, w *World, side byte, sorter *extsort.Sorter, budget int64) error {
	// Open the graph tree read-only.
	gtx, err := kvtx_block.BuildKvTransaction(ctx, cursor.FollowSubBlock(2), false)
	if err != nil {
		return errors.Wrap(err, "open graph tree")
	}
	defer gtx.Discard()
	handle, err := openGraph(ctx, w.GetFormat(), w.GetGraphSettings(), gtx)
	if err != nil {
		return errors.Wrap(err, "open graph")
	}
	defer handle.Close()

	// Reserve from the count, then stream every quad.
	stats, err := handle.Stats(ctx, false)
	if err != nil {
		return errors.Wrap(err, "graph counts")
	}
	if n := stats.Quads.Value * regenVerifyQuadBytes; n > budget {
		if err := sorter.Reserve(n); err != nil {
			return err
		}
	}
	tag := []byte{side}
	return world.IterateFilteredFullQuads(ctx, handle, quad.Quad{}, func(q quad.Quad) error {
		key, err := pquads.MakeQuad(q).MarshalVT()
		if err != nil {
			return err
		}
		return sorter.Add(key, tag)
	})
}

// describeVerifyQuad names a quad the two sides hold count times each.
func describeVerifyQuad(key []byte, count [2]int) string {
	q := &pquads.Quad{}
	if err := q.UnmarshalVT(key); err != nil {
		return err.Error()
	}
	return fmt.Sprintf("%v (old %d, new %d)", q.ToNative(), count[0], count[1])
}
