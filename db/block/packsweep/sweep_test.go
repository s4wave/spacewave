package packsweep

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

// fixtureBlock is one block of a fixture graph.
type fixtureBlock struct {
	// ref is the block's ref.
	ref *block.BlockRef
	// data is its bytes.
	data []byte
	// refs are its children.
	refs []*block.BlockRef
	// unknown stores it without its refs in every pack.
	unknown bool
	// backend makes it held only by the ref source, in no pack.
	backend bool
	// missing makes it held nowhere.
	missing bool
}

// fixture is a pack store over a random block graph.
type fixture struct {
	// store is the pack store holding the packs.
	store *block_store_s3.PackStore
	// blocks are the graph's blocks by marshaled ref.
	blocks map[string]*fixtureBlock
	// order lists the blocks in creation order.
	order []*fixtureBlock
	// roots are the walk's roots.
	roots []*block.BlockRef
	// packs maps each pack id to its blocks.
	packs map[string][]*fixtureBlock
}

// fixtureOptions shapes a fixture.
type fixtureOptions struct {
	// blocks is the blocks in the graph.
	blocks int
	// roots is the roots picked from the newest blocks.
	roots int
	// packBlocks is the most blocks per pack.
	packBlocks int
	// dupShare is the share of blocks written to a second pack.
	dupShare float64
	// unknown, backend and missing name blocks with those states by index.
	unknown, backend, missing []int
}

// newFixture builds the graph and writes its packs. Each block links up to
// three older blocks, so subtrees are shared; the roots are among the newest
// blocks, so the oldest blocks are mostly dead.
func newFixture(t *testing.T, seed uint64, opts fixtureOptions) *fixture {
	t.Helper()
	ctx := context.Background()
	rng := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	f := &fixture{blocks: make(map[string]*fixtureBlock), packs: make(map[string][]*fixtureBlock)}

	// Build the graph, oldest first.
	for i := range opts.blocks {
		data := fmt.Appendf(nil, "block %d seed %d pad %0*d", i, seed, rng.IntN(400), 0)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		b := &fixtureBlock{ref: ref, data: data}
		if i > 0 {
			for range rng.IntN(4) {
				child := f.order[rng.IntN(i)]
				if !slices.ContainsFunc(b.refs, child.ref.EqualsRef) {
					b.refs = append(b.refs, child.ref)
				}
			}
		}
		f.blocks[ref.MarshalString()] = b
		f.order = append(f.order, b)
	}
	for _, i := range opts.unknown {
		f.order[i].unknown = true
	}
	for _, i := range opts.backend {
		f.order[i].backend = true
	}
	for _, i := range opts.missing {
		f.order[i].missing = true
	}
	var rootBlocks []*fixtureBlock
	for i := range opts.roots {
		root := f.order[len(f.order)-1-i*3]
		rootBlocks = append(rootBlocks, root)
		f.roots = append(f.roots, root.ref)
	}

	// Reach every special block from the first root, and one middle block
	// from every root. A block's refs are stored beside its bytes, so
	// adding them changes no ref.
	link := func(parent, child *fixtureBlock) {
		if parent != child && !slices.ContainsFunc(parent.refs, child.ref.EqualsRef) {
			parent.refs = append(parent.refs, child.ref)
		}
	}
	for _, set := range [][]int{opts.unknown, opts.backend, opts.missing} {
		for _, i := range set {
			link(rootBlocks[0], f.order[i])
		}
	}
	for _, root := range rootBlocks {
		link(root, f.order[len(f.order)/2])
	}

	// Write the packs in creation order, then a second copy of some blocks.
	_, client := s3test.New(t)
	f.store = block_store_s3.NewPackStore(logrus.NewEntry(logrus.New()), client, s3test.BucketName, "v2/", &block_store_s3.PackStoreOptions{KeepPacks: true})
	t.Cleanup(f.store.Close)
	var stored []*fixtureBlock
	for _, b := range f.order {
		if !b.backend && !b.missing {
			stored = append(stored, b)
		}
	}
	var dups []*fixtureBlock
	for _, b := range stored {
		if rng.Float64() < opts.dupShare {
			dups = append(dups, b)
		}
	}
	rng.Shuffle(len(dups), func(i, j int) { dups[i], dups[j] = dups[j], dups[i] })
	for _, set := range [][]*fixtureBlock{stored, dups} {
		for len(set) != 0 {
			n := min(1+rng.IntN(opts.packBlocks), len(set))
			chunk := set[:n]
			set = set[n:]
			batch := make([]*block.PutBatchEntry, len(chunk))
			for i, b := range chunk {
				batch[i] = &block.PutBatchEntry{Ref: b.ref, Data: b.data, Refs: b.refs, RefsKnown: !b.unknown}
			}
			before := f.packIDs(t)
			if _, err := f.store.PutBlockBatch(ctx, batch); err != nil {
				t.Fatal(err)
			}
			for _, id := range f.packIDs(t) {
				if !slices.Contains(before, id) {
					f.packs[id] = chunk
				}
			}
		}
	}
	return f
}

// packIDs lists the store's packs.
func (f *fixture) packIDs(t *testing.T) []string {
	t.Helper()
	entries, err := f.store.Packs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.GetId()
	}
	return ids
}

// BlockRefs serves the backend blocks.
func (f *fixture) BlockRefs(_ context.Context, refs []*block.BlockRef) ([]RefsResult, error) {
	out := make([]RefsResult, len(refs))
	for i, ref := range refs {
		b := f.blocks[ref.MarshalString()]
		if b == nil || !b.backend {
			out[i].Reason = packcopy.ReasonMissing
			continue
		}
		out[i].Refs = b.refs
	}
	return out, nil
}

// expected is the independent computation of a cycle.
type expected struct {
	// live is the blocks the walk can reach, by marshaled ref.
	live map[string]bool
	// truth is every block the roots reach through the true refs.
	truth map[string]bool
	// holes is each hole's reason by marshaled ref.
	holes map[string]string
	// verdicts is each pack's verdict.
	verdicts map[string]Verdict
}

// expect computes the cycle's result in memory from the graph alone.
func (f *fixture) expect(t *testing.T, lowPercent int) *expected {
	t.Helper()
	e := &expected{live: map[string]bool{}, truth: map[string]bool{}, holes: map[string]string{}, verdicts: map[string]Verdict{}}

	// Reach from the roots, following only the refs some store holds.
	var visit func(ref *block.BlockRef, seen map[string]bool, honest bool)
	visit = func(ref *block.BlockRef, seen map[string]bool, honest bool) {
		key := ref.MarshalString()
		if seen[key] {
			return
		}
		seen[key] = true
		b := f.blocks[key]
		if honest {
			switch {
			case b.missing:
				e.holes[key] = packcopy.ReasonMissing
				return
			case b.unknown:
				e.holes[key] = packcopy.ReasonRefsUnknown
				return
			}
		}
		for _, child := range b.refs {
			visit(child, seen, honest)
		}
	}
	for _, root := range f.roots {
		visit(root, e.live, true)
		visit(root, e.truth, false)
	}

	// Judge each pack from its blocks' encoded sizes.
	for id, blocks := range f.packs {
		var value, live uint64
		for _, b := range blocks {
			data, err := block.EncodeBlockObject(b.data, b.refs, !b.unknown)
			if err != nil {
				t.Fatal(err)
			}
			value += uint64(len(data))
			if e.live[b.ref.MarshalString()] {
				live += uint64(len(data))
			}
		}
		switch {
		case live == 0:
			e.verdicts[id] = VerdictDead
		case live*100 <= value*uint64(lowPercent): //nolint:gosec // a test percent.
			e.verdicts[id] = VerdictLow
		default:
			e.verdicts[id] = VerdictKept
		}
	}
	return e
}

// cycle runs snapshot, walk and judge into store, with S two days after now.
func (f *fixture) cycle(t *testing.T, store kvtx.Store, src PackSource, batch int, afterBatch func() error) (*Head, error) {
	t.Helper()
	ctx := context.Background()
	if src == nil {
		src = f.store
	}
	now := func() time.Time { return time.Now().Add(48 * time.Hour) }
	if _, err := Snapshot(ctx, &SnapshotOptions{Store: store, Packs: src, Reads: 3, Now: now}); err != nil {
		return nil, err
	}
	head, err := Walk(ctx, &WalkOptions{Store: store, Roots: f.roots, Refs: f, Batch: batch, afterBatch: afterBatch})
	if err != nil {
		return nil, err
	}
	if _, err := Judge(ctx, &JudgeOptions{Store: store, LowLivePercent: 30}); err != nil {
		return nil, err
	}
	return head, nil
}

// readResult reads a cycle's judgments and holes.
func readResult(t *testing.T, store kvtx.Store) (map[string]Verdict, map[string]string) {
	t.Helper()
	ctx := context.Background()
	verdicts := map[string]Verdict{}
	if err := Judgments(ctx, store, func(j *Judgment) error {
		verdicts[j.ID] = j.Verdict
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	holes := map[string]string{}
	if err := Holes(ctx, store, func(h Hole) error {
		holes[h.Ref.MarshalString()] = h.Reason
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return verdicts, holes
}

// TestDryRunMatchesReachability checks the report against an in-memory
// reachability over the same graph, with duplicated keys, a block reached
// from two roots, refs-unknown, backend-only and missing blocks.
func TestDryRunMatchesReachability(t *testing.T) {
	for seed := range uint64(4) {
		t.Run(fmt.Sprint(seed), func(t *testing.T) {
			f := newFixture(t, seed, fixtureOptions{
				blocks: 300, roots: 3, packBlocks: 12, dupShare: 0.2,
				unknown: []int{150, 210}, backend: []int{260, 270}, missing: []int{240, 280},
			})
			want := f.expect(t, 30)
			store := store_kvtx_inmem.NewStore()
			head, err := f.cycle(t, store, nil, 0, nil)
			if err != nil {
				t.Fatal(err)
			}

			// The report matches the computation.
			verdicts, holes := readResult(t, store)
			if len(verdicts) != len(want.verdicts) {
				t.Fatalf("judged %d packs, want %d", len(verdicts), len(want.verdicts))
			}
			for id, v := range want.verdicts {
				if verdicts[id] != v {
					t.Errorf("pack %s: verdict %v, want %v", id, verdicts[id], v)
				}
			}
			if !mapsEqual(holes, want.holes) {
				t.Errorf("holes %v, want %v", holes, want.holes)
			}
			if head.Live != uint64(len(want.live)) {
				t.Errorf("live %d, want %d", head.Live, len(want.live))
			}

			// A dead pack holds no truly reached block unless a hole that
			// stops the cycle hides it.
			blocking := BlockingHoles(head, nil, nil)
			for id, v := range verdicts {
				if v != VerdictDead || len(blocking) != 0 {
					continue
				}
				for _, b := range f.packs[id] {
					if want.truth[b.ref.MarshalString()] {
						t.Errorf("dead pack %s holds reached block %s with no blocking hole", id, b.ref.MarshalString())
					}
				}
			}
		})
	}
}

// TestDryRunSafeWithoutHoles checks that with no holes every dead pack holds
// only unreached blocks, and that some packs are dead.
func TestDryRunSafeWithoutHoles(t *testing.T) {
	f := newFixture(t, 7, fixtureOptions{blocks: 400, roots: 2, packBlocks: 10, dupShare: 0.3, backend: []int{390}})
	want := f.expect(t, 30)
	store := store_kvtx_inmem.NewStore()
	head, err := f.cycle(t, store, nil, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if blocking := BlockingHoles(head, nil, nil); len(blocking) != 0 {
		t.Fatalf("holes %v", blocking)
	}
	verdicts, _ := readResult(t, store)
	var dead int
	for id, v := range verdicts {
		if v != VerdictDead {
			continue
		}
		dead++
		for _, b := range f.packs[id] {
			if want.truth[b.ref.MarshalString()] {
				t.Fatalf("dead pack %s holds reached block", id)
			}
		}
	}
	if dead == 0 {
		t.Fatal("no dead pack in a graph whose oldest blocks are unreached")
	}
}

// errKill stops a step as a crash would.
var errKill = errors.New("killed")

// failingPacks fails the read of the nth pack and every one after it.
type failingPacks struct {
	PackSource
	// left is the reads that still succeed.
	left atomic.Int64
}

// ReadPackFile fails once left is spent.
func (p *failingPacks) ReadPackFile(ctx context.Context, id string, size uint64) ([]byte, error) {
	if p.left.Add(-1) < 0 {
		return nil, errKill
	}
	return p.PackSource.ReadPackFile(ctx, id, size)
}

// TestDryRunResumes kills the snapshot after each pack and the walk after
// each batch, then resumes, and checks every resumed run reports the same.
func TestDryRunResumes(t *testing.T) {
	f := newFixture(t, 11, fixtureOptions{
		blocks: 120, roots: 2, packBlocks: 8, dupShare: 0.2,
		unknown: []int{60}, backend: []int{110}, missing: []int{100},
	})
	clean := store_kvtx_inmem.NewStore()
	if _, err := f.cycle(t, clean, nil, 5, nil); err != nil {
		t.Fatal(err)
	}
	wantVerdicts, wantHoles := readResult(t, clean)

	// Kill the snapshot after each number of pack reads.
	for k := range int64(len(f.packs)) {
		store := store_kvtx_inmem.NewStore()
		src := &failingPacks{PackSource: f.store}
		src.left.Store(k)
		if _, err := f.cycle(t, store, src, 5, nil); !errors.Is(err, errKill) {
			t.Fatalf("snapshot killed after %d packs: %v", k, err)
		}
		if _, err := f.cycle(t, store, nil, 5, nil); err != nil {
			t.Fatal(err)
		}
		verdicts, holes := readResult(t, store)
		if !mapsEqual(verdicts, wantVerdicts) || !mapsEqual(holes, wantHoles) {
			t.Fatalf("snapshot killed after %d packs resumed to another report", k)
		}
	}

	// Kill the walk after each batch.
	for k := 1; ; k++ {
		store := store_kvtx_inmem.NewStore()
		batches := 0
		_, err := f.cycle(t, store, nil, 5, func() error {
			if batches++; batches == k {
				return errKill
			}
			return nil
		})
		if err == nil {
			break
		}
		if !errors.Is(err, errKill) {
			t.Fatal(err)
		}
		if _, err := f.cycle(t, store, nil, 5, nil); err != nil {
			t.Fatal(err)
		}
		verdicts, holes := readResult(t, store)
		if !mapsEqual(verdicts, wantVerdicts) || !mapsEqual(holes, wantHoles) {
			t.Fatalf("walk killed after batch %d resumed to another report", k)
		}
	}
}

// TestCatalogKeptAcrossCycles checks a second cycle reads only new packs.
func TestCatalogKeptAcrossCycles(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, 3, fixtureOptions{blocks: 60, roots: 1, packBlocks: 6})
	store := store_kvtx_inmem.NewStore()
	if _, err := f.cycle(t, store, nil, 0, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := NewCycle(ctx, store); err != nil {
		t.Fatal(err)
	}
	src := &failingPacks{PackSource: f.store}
	progress, err := Snapshot(ctx, &SnapshotOptions{Store: store, Packs: src})
	if err != nil {
		t.Fatal(err)
	}
	if progress.Catalogued != 0 || progress.Skipped != len(f.packs) {
		t.Fatalf("second cycle read %d packs and skipped %d of %d", progress.Catalogued, progress.Skipped, len(f.packs))
	}
}

// TestDirPacksReadsCopies reads every pack from local copies but one held at
// the wrong size, which comes from the store, to the same report.
func TestDirPacksReadsCopies(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t, 5, fixtureOptions{blocks: 80, roots: 2, packBlocks: 6, dupShare: 0.2})
	clean := store_kvtx_inmem.NewStore()
	if _, err := f.cycle(t, clean, nil, 0, nil); err != nil {
		t.Fatal(err)
	}
	wantVerdicts, wantHoles := readResult(t, clean)

	// Copy every pack, truncating the first.
	dir := t.TempDir()
	entries, err := f.store.Packs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range entries {
		data, err := f.store.ReadPackFile(ctx, e.GetId(), e.GetSizeBytes())
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			data = data[:len(data)-1]
		}
		if err := os.WriteFile(filepath.Join(dir, e.GetId()), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// The store serves only the truncated one.
	src := &failingPacks{PackSource: f.store}
	src.left.Store(1)
	local := &DirPacks{PackSource: src, Dir: dir}
	store := store_kvtx_inmem.NewStore()
	if _, err := f.cycle(t, store, local, 0, nil); err != nil {
		t.Fatal(err)
	}
	if local.Local != len(entries)-1 {
		t.Fatalf("read %d of %d packs locally", local.Local, len(entries)-1)
	}
	verdicts, holes := readResult(t, store)
	if !mapsEqual(verdicts, wantVerdicts) || !mapsEqual(holes, wantHoles) {
		t.Fatal("local copies gave another report")
	}
	if _, err := local.ReadPackFile(ctx, "../escape", 1); err == nil {
		t.Fatal("read a pack id naming another directory")
	}
}

// TestKeyTailMatchesIndex checks the sweep keys blocks as the pack index does.
func TestKeyTailMatchesIndex(t *testing.T) {
	ref, err := block.BuildBlockRef([]byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	key := packfile.BlockKey(ref.GetHash())
	if got := RefTail(ref); string(got) != string(key[len(key)-TailBytes:]) {
		t.Fatal("tail differs from the key's end")
	}
}

// mapsEqual reports whether two maps hold the same entries.
func mapsEqual[K, V comparable](a, b map[K]V) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}
