//go:build !js

package engine

import (
	"bytes"
	"context"
	"maps"
	"math/rand/v2"
	"slices"
	"strconv"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// crashKeys is the key space of the crash workload's key-value commits.
const crashKeys = 12

// crashModel tracks what a crash workload acknowledged and what the crash left
// uncertain.
type crashModel struct {
	// values holds the acknowledged key-value state.
	values map[string]string
	// pending holds the changes of the commit the crash interrupted; a nil
	// value deletes its key.
	pending map[string]*string
	// synced holds blocks an acknowledged Sync made durable, by ref string.
	synced map[string]crashBlock
	// unsynced holds blocks that may be absent or present, by ref string.
	unsynced map[string]crashBlock
	// removed holds blocks an acknowledged RmBlock deleted.
	removed []*block.BlockRef
	// appended holds edges of acknowledged journal appends.
	appended map[block_gc.RefEdge]struct{}
	// possible holds every edge an append attempted.
	possible map[block_gc.RefEdge]struct{}
	// blocks counts put blocks so each payload is unique.
	blocks int
	// edges counts journal edges so each is unique.
	edges int
}

// crashBlock is one put block and its payload.
type crashBlock struct {
	// ref identifies the block.
	ref *block.BlockRef
	// data is the payload the block must read back as.
	data []byte
}

// recordingGraph collects every edge a journal replay applies.
type recordingGraph struct {
	block_gc.CollectorGraph
	// mtx protects applied.
	mtx sync.Mutex
	// applied holds every edge applied as an addition.
	applied map[block_gc.RefEdge]struct{}
}

// ApplyRefBatch records the added edges.
func (g *recordingGraph) ApplyRefBatch(_ context.Context, adds, _ []block_gc.RefEdge) error {
	g.mtx.Lock()
	defer g.mtx.Unlock()
	for _, edge := range adds {
		g.applied[edge] = struct{}{}
	}
	return nil
}

// TestCrashRecoversAcknowledgedState crashes a randomized workload at every
// backend Write and Remove boundary and checks that the reopened volume holds
// exactly what the workload acknowledged: committed key-value state, synced
// blocks, acknowledged block removals, and journaled edges. The operation the
// crash interrupted may apply wholly or not at all.
func TestCrashRecoversAcknowledgedState(t *testing.T) {
	for _, seed := range []uint64{1, 2} {
		t.Run("seed-"+strconv.FormatUint(seed, 10), func(t *testing.T) {
			// Count the boundaries of an uninterrupted run, which must also recover.
			boundaries := runCrashWorkload(t, seed, -1)
			for crashAfter := range boundaries {
				t.Run(strconv.Itoa(crashAfter), func(t *testing.T) {
					t.Parallel()
					runCrashWorkload(t, seed, crashAfter)
				})
			}
		})
	}
}

// runCrashWorkload runs the seeded workload, crashing the backend after
// crashAfter successful mutations, then recovers and checks the volume. It
// returns the number of Write and Remove calls the workload made.
func runCrashWorkload(t *testing.T, seed uint64, crashAfter int) int {
	ctx := t.Context()
	d := newDiskBackend(t)
	d.unsynced = true
	target := openReplayTarget(t, d)
	graph := &recordingGraph{applied: make(map[block_gc.RefEdge]struct{})}
	m := &crashModel{
		values:   make(map[string]string),
		synced:   make(map[string]crashBlock),
		unsynced: make(map[string]crashBlock),
		appended: make(map[block_gc.RefEdge]struct{}),
		possible: make(map[block_gc.RefEdge]struct{}),
	}

	// Run until the first failed operation, which the crash interrupted.
	d.mtx.Lock()
	d.crashAfter = crashAfter
	d.mtx.Unlock()
	rng := rand.New(rand.NewPCG(seed, seed))
	for range 40 {
		if err := m.step(ctx, rng, target, graph); err != nil {
			break
		}
	}
	_ = target.BlockStore.Close()
	_ = target.Engine.Close()
	d.mtx.Lock()
	mutations := d.mutations
	d.mtx.Unlock()

	// Recover and check the acknowledged state.
	d.restart()
	recovered := openReplayTarget(t, d)
	if _, err := recovered.ReplayWAL(ctx, graph); err != nil {
		t.Fatalf("recover journal: %v", err)
	}
	m.check(t, recovered, graph)

	// The recovered volume accepts new writes.
	tx, err := recovered.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("after"), []byte("recovery")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("commit after recovery: %v", err)
	}
	if _, _, err := recovered.PutBlock(ctx, []byte("after recovery"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := recovered.Sync(ctx); err != nil {
		t.Fatalf("sync after recovery: %v", err)
	}
	return mutations
}

// step runs one random operation and records its acknowledged effect. An
// error leaves the operation's effect uncertain in the model.
func (m *crashModel) step(ctx context.Context, rng *rand.Rand, target ReplayTarget, graph *recordingGraph) error {
	switch n := rng.IntN(100); {
	case n < 30:
		return m.commit(ctx, rng, target)
	case n < 55:
		return m.put(ctx, rng, target)
	case n < 67:
		if _, err := target.Sync(ctx); err != nil {
			return err
		}
		maps.Copy(m.synced, m.unsynced)
		clear(m.unsynced)
		return nil
	case n < 75:
		return m.remove(ctx, rng, target)
	case n < 87:
		return m.append(ctx, target)
	case n < 92:
		_, err := target.ReplayWAL(ctx, graph)
		return err
	default:
		if _, err := target.CleanPack(ctx); err != nil {
			return err
		}
		_, err := target.Reclaim(ctx)
		return err
	}
}

// commit sets or deletes one to three keys in one transaction.
func (m *crashModel) commit(ctx context.Context, rng *rand.Rand, target ReplayTarget) error {
	tx, err := target.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	changes := make(map[string]*string)
	for range 1 + rng.IntN(3) {
		key := "k" + strconv.Itoa(rng.IntN(crashKeys))
		if rng.IntN(5) == 0 {
			changes[key] = nil
			if err := tx.Delete(ctx, []byte(key)); err != nil {
				return err
			}
			continue
		}
		value := strconv.FormatUint(rng.Uint64(), 36)
		changes[key] = &value
		if err := tx.Set(ctx, []byte(key), []byte(value)); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		m.pending = changes
		return err
	}
	for key, value := range changes {
		if value == nil {
			delete(m.values, key)
			continue
		}
		m.values[key] = *value
	}
	return nil
}

// put admits one unique block without syncing it.
func (m *crashModel) put(ctx context.Context, rng *rand.Rand, target ReplayTarget) error {
	m.blocks++
	data := bytes.Repeat([]byte("block "+strconv.Itoa(m.blocks)+" "), 8+rng.IntN(256))
	ref, _, err := target.PutBlock(ctx, data, nil)
	if err != nil {
		return err
	}
	m.unsynced[ref.MarshalString()] = crashBlock{ref: ref, data: data}
	return nil
}

// remove deletes one synced block, which RmBlock makes durable.
func (m *crashModel) remove(ctx context.Context, rng *rand.Rand, target ReplayTarget) error {
	if len(m.synced) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m.synced))
	for key := range m.synced {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	key := keys[rng.IntN(len(keys))]
	b := m.synced[key]
	delete(m.synced, key)
	if err := target.RmBlock(ctx, b.ref); err != nil {
		m.unsynced[key] = b
		return err
	}
	m.removed = append(m.removed, b.ref)
	return nil
}

// append journals one or two unique edges.
func (m *crashModel) append(ctx context.Context, target ReplayTarget) error {
	var adds []block_gc.RefEdge
	for range 2 {
		m.edges++
		edge := block_gc.RefEdge{Subject: "s" + strconv.Itoa(m.edges), Object: "o" + strconv.Itoa(m.edges)}
		adds = append(adds, edge)
		m.possible[edge] = struct{}{}
	}
	if err := target.AppendJournal(ctx, adds, nil); err != nil {
		return err
	}
	for _, edge := range adds {
		m.appended[edge] = struct{}{}
	}
	return nil
}

// check compares the recovered volume with the acknowledged state.
func (m *crashModel) check(t *testing.T, target ReplayTarget, graph *recordingGraph) {
	t.Helper()
	ctx := t.Context()

	// Key-value state is the acknowledged state with the interrupted commit
	// applied wholly or not at all.
	got := make(map[string]*string)
	for i := range crashKeys {
		key := "k" + strconv.Itoa(i)
		value, found, _, err := target.Get(ctx, []byte(key))
		if err != nil {
			t.Fatalf("get %s: %v", key, err)
		}
		if found {
			v := string(value)
			got[key] = &v
		}
	}
	matches := func(pending bool) bool {
		for i := range crashKeys {
			key := "k" + strconv.Itoa(i)
			want, ok := m.values[key]
			wantPtr := &want
			if !ok {
				wantPtr = nil
			}
			if change, changed := m.pending[key]; pending && changed {
				wantPtr = change
			}
			if (got[key] == nil) != (wantPtr == nil) || (wantPtr != nil && *got[key] != *wantPtr) {
				return false
			}
		}
		return true
	}
	if !matches(false) && !matches(true) {
		t.Fatalf("recovered key-value state %v is neither %v nor it with %v applied", deref(got), m.values, deref(m.pending))
	}

	// Synced blocks read back exactly, unsynced ones exactly or not at all,
	// and acknowledged removals stay removed.
	for _, b := range m.synced {
		data, found, err := target.GetBlock(ctx, b.ref)
		if err != nil || !found || !bytes.Equal(data, b.data) {
			t.Fatalf("synced block %s: found=%t error=%v", b.ref.MarshalString(), found, err)
		}
	}
	for _, b := range m.unsynced {
		data, found, err := target.GetBlock(ctx, b.ref)
		if err != nil || (found && !bytes.Equal(data, b.data)) {
			t.Fatalf("unsynced block %s: found=%t error=%v", b.ref.MarshalString(), found, err)
		}
	}
	for _, ref := range m.removed {
		if _, found, err := target.GetBlock(ctx, ref); err != nil || found {
			t.Fatalf("removed block %s: found=%t error=%v", ref.MarshalString(), found, err)
		}
	}

	// Every acknowledged edge was applied, and only attempted edges were.
	for edge := range m.appended {
		if _, ok := graph.applied[edge]; !ok {
			t.Fatalf("acknowledged edge %v was never applied", edge)
		}
	}
	for edge := range graph.applied {
		if _, ok := m.possible[edge]; !ok {
			t.Fatalf("applied edge %v was never appended", edge)
		}
	}
}

// deref formats optional values for failure messages.
func deref(values map[string]*string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = "<deleted>"
		if value != nil {
			out[key] = *value
		}
	}
	return out
}
