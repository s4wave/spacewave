package world_block_test

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

func BenchmarkWorldStateSetGraphQuadWrite(b *testing.B) {
	// Open a write transaction with the two objects every quad links.
	ctx := context.Background()
	eng, cleanup := setupWorldWriteBench(ctx, b)
	defer cleanup()
	ws := newWorldWriteBenchTx(ctx, b, eng)
	defer ws.Discard()
	createWorldWriteBenchObject(ctx, b, ws, "bench/set-graph/source")
	createWorldWriteBenchObject(ctx, b, ws, "bench/set-graph/target")

	// Prepare one quad with a distinct predicate per iteration.
	quads := make([]world.GraphQuad, b.N)
	for i := range quads {
		quads[i] = world.NewGraphQuadWithKeys(
			"bench/set-graph/source",
			"<bench/set-graph/predicate/"+strconv.Itoa(i)+">",
			"bench/set-graph/target",
			"",
		)
	}

	// Set one quad per iteration, counting block traffic.
	b.ReportAllocs()
	b.ResetTimer()
	var counts worldWriteBenchCounts
	for i := range b.N {
		opCtx, readCounter, writeCounter := withWorldWriteBenchCounters(ctx)
		if err := ws.SetGraphQuad(opCtx, quads[i]); err != nil {
			b.Fatal(err.Error())
		}
		counts.add(readCounter, writeCounter)
	}

	// Report the per-iteration results.
	b.ReportMetric(1, "graph_quads/op")
	counts.report(b)
}

func BenchmarkWorldStateCreateObjectWrite(b *testing.B) {
	// Open a write transaction and prepare one object key per iteration.
	ctx := context.Background()
	eng, cleanup := setupWorldWriteBench(ctx, b)
	defer cleanup()
	ws := newWorldWriteBenchTx(ctx, b, eng)
	defer ws.Discard()
	keys := make([]string, b.N)
	for i := range keys {
		keys[i] = "bench/create-object/" + strconv.Itoa(i)
	}

	// Create one object per iteration, counting block traffic.
	b.ReportAllocs()
	b.ResetTimer()
	var counts worldWriteBenchCounts
	for i := range b.N {
		opCtx, readCounter, writeCounter := withWorldWriteBenchCounters(ctx)
		createWorldWriteBenchObject(opCtx, b, ws, keys[i])
		counts.add(readCounter, writeCounter)
	}

	// Report the per-iteration results.
	b.ReportMetric(1, "objects/op")
	counts.report(b)
}

func BenchmarkWorldStateMultiOpWriteTransaction(b *testing.B) {
	// Build an empty World.
	ctx := context.Background()
	eng, cleanup := setupWorldWriteBench(ctx, b)
	defer cleanup()

	// Prepare two object keys and the quad linking them per iteration.
	subjectKeys := make([]string, b.N)
	objectKeys := make([]string, b.N)
	quads := make([]world.GraphQuad, b.N)
	for i := range subjectKeys {
		subjectKeys[i] = "bench/multi-op/subject/" + strconv.Itoa(i)
		objectKeys[i] = "bench/multi-op/object/" + strconv.Itoa(i)
		quads[i] = world.NewGraphQuadWithKeys(
			subjectKeys[i],
			"<bench/multi-op/relates-to>",
			objectKeys[i],
			"",
		)
	}

	// Create both objects, link them, and commit one transaction per iteration.
	b.ReportAllocs()
	b.ResetTimer()
	var counts worldWriteBenchCounts
	for i := range b.N {
		opCtx, readCounter, writeCounter := withWorldWriteBenchCounters(ctx)
		ws := newWorldWriteBenchTx(opCtx, b, eng)
		createWorldWriteBenchObject(opCtx, b, ws, subjectKeys[i])
		createWorldWriteBenchObject(opCtx, b, ws, objectKeys[i])
		if err := ws.SetGraphQuad(opCtx, quads[i]); err != nil {
			b.Fatal(err.Error())
		}
		if err := ws.Commit(opCtx); err != nil {
			b.Fatal(err.Error())
		}
		ws.Discard()
		counts.add(readCounter, writeCounter)
	}

	// Report the per-iteration results.
	b.ReportMetric(2, "objects/op")
	b.ReportMetric(1, "graph_quads/op")
	b.ReportMetric(1, "world_commits/op")
	counts.report(b)
}

// Seeded World shape: every object has a type edge to one of a few hub nodes
// and a parent edge to an earlier object, so the World holds
// worldSeedObjects objects, twice as many edges, and hubs with thousands of
// incoming edges.
const (
	worldSeedObjects      = 20000
	worldSeedTypes        = 4
	worldSeedObjectsPerTx = 500
	worldSeedTypePred     = "<bench/seed/type>"
	worldSeedParentPred   = "<bench/seed/parent>"
)

// worldSeed is the seeded World shared by every seeded benchmark run in the
// process. Each run opens a fresh Engine on its root.
var worldSeed struct {
	once    sync.Once
	testbed *testbed.Testbed
	rootRef *bucket.ObjectRef
	err     error
}

// worldSeededWriteCase is one kind of commit the seeded benchmarks apply.
type worldSeededWriteCase struct {
	// name names the case's sub-benchmark.
	name string
	// objects is the number of objects one commit creates, reported when
	// nonzero.
	objects float64
	// op applies commit i's changes to ws.
	op func(ctx context.Context, tb testing.TB, ws world.WorldState, i int) error
}

// worldSeededWriteCases are the commits the seeded benchmarks apply.
var worldSeededWriteCases = []worldSeededWriteCase{{
	name: "create-typed-object",
	op: func(ctx context.Context, tb testing.TB, ws world.WorldState, i int) error {
		// Create the object.
		key := "bench/seeded/new/" + strconv.Itoa(i)
		createWorldWriteBenchObject(ctx, tb, ws, key)

		// Link the object to its type hub and parent.
		typeQuad := world.NewGraphQuadWithKeys(key, worldSeedTypePred, worldSeedTypeKey(i), "")
		if err := ws.SetGraphQuad(ctx, typeQuad); err != nil {
			return err
		}
		parentQuad := world.NewGraphQuadWithKeys(key, worldSeedParentPred, worldSeedObjectKey(worldSeedPick(i)), "")
		return ws.SetGraphQuad(ctx, parentQuad)
	},
}, {
	name: "increment-object-rev",
	op: func(ctx context.Context, _ testing.TB, ws world.WorldState, i int) error {
		// Bump the revision of an existing object.
		return bumpObjectRev(ctx, ws, worldSeedObjectKey(worldSeedPick(i)))
	},
}, {
	name: "link-existing-objects",
	op: func(ctx context.Context, _ testing.TB, ws world.WorldState, i int) error {
		// Link two existing objects with a new edge.
		quad := world.NewGraphQuadWithKeys(
			worldSeedObjectKey(worldSeedPick(i)),
			"<bench/seeded/link>",
			worldSeedObjectKey(worldSeedPick(i+worldSeedObjects/2)),
			"",
		)
		return ws.SetGraphQuad(ctx, quad)
	},
}, {
	name:    "create-32-results",
	objects: worldSeedResultBatch,
	op: func(ctx context.Context, tb testing.TB, ws world.WorldState, i int) error {
		// Create a batch of result-shaped objects, as an app accepting a few
		// hosts' results does: each result sits under its host's key and
		// links to its type, its host and a player.
		quads := make([]world.GraphQuad, 0, 3*worldSeedResultBatch)
		for j := range worldSeedResultBatch {
			n := i*worldSeedResultBatch + j
			host := worldSeedObjectKey(worldSeedPick(1000 + j%4 + i/2))
			key := host + "/attempts/" + strconv.Itoa(n)
			createWorldWriteBenchObject(ctx, tb, ws, key)
			quads = append(quads,
				world.NewGraphQuadWithKeys(key, worldSeedTypePred, worldSeedTypeKey(0), ""),
				world.NewGraphQuadWithKeys(host, "<bench/host-result>", key, ""),
				world.NewGraphQuadWithKeys(key, "<bench/result-player>", worldSeedObjectKey(worldSeedPick(5000+n)), ""),
			)
		}
		for _, q := range quads {
			if err := ws.SetGraphQuad(ctx, q); err != nil {
				return err
			}
		}
		return nil
	},
}}

func BenchmarkWorldSeededWrite(b *testing.B) {
	// Each case commits one transaction per iteration on the seeded World.
	for _, c := range worldSeededWriteCases {
		b.Run(c.name, func(b *testing.B) {
			benchmarkWorldSeededWrite(b, c)
		})
	}
}

// worldStoredPageWindows are the numbers of commits whose kept pages
// BenchmarkWorldSeededStoredPages stores together.
var worldStoredPageWindows = []int{1, 8, 32, 64}

// BenchmarkWorldSeededStoredPages commits each seeded case through a
// PendingStore, as a Space's kept pages do, and stores the kept pages the
// World reaches once every window commits, dropping the rest: a page a later
// commit in the window replaced is never stored. A window of one stores what
// every commit stores. It reports the stored pages and bytes per commit, by
// tree, and logs them by level.
func BenchmarkWorldSeededStoredPages(b *testing.B) {
	for _, c := range worldSeededWriteCases {
		for _, window := range worldStoredPageWindows {
			b.Run(c.name+"/window-"+strconv.Itoa(window), func(b *testing.B) {
				benchmarkWorldSeededStoredPages(b, c, window)
			})
		}
	}
}

// worldSeedResultBatch is the number of results one create-32-results commit
// writes.
const worldSeedResultBatch = 32

// benchmarkWorldSeededWrite applies c in one committed transaction per
// iteration on a fresh Engine over the seeded World.
func benchmarkWorldSeededWrite(b *testing.B, c worldSeededWriteCase) {
	// Open an Engine on the seeded World.
	ctx := context.Background()
	eng, cleanup := setupWorldSeededBench(ctx, b)
	defer cleanup()

	// Apply and commit c once per iteration, counting block traffic.
	b.ReportAllocs()
	b.ResetTimer()
	var counts worldWriteBenchCounts
	for i := range b.N {
		opCtx, readCounter, writeCounter := withWorldWriteBenchCounters(ctx)
		commitWorldSeededCase(opCtx, b, eng, c, i)
		counts.add(readCounter, writeCounter)
	}

	// Report the per-iteration results.
	b.ReportMetric(1, "world_commits/op")
	if c.objects != 0 {
		b.ReportMetric(c.objects, "objects/op")
	}
	counts.report(b)
}

// commitWorldSeededCase applies commit i of c in one transaction on eng.
func commitWorldSeededCase(ctx context.Context, tb testing.TB, eng *world_block.Engine, c worldSeededWriteCase, i int) {
	tb.Helper()
	ws := newWorldWriteBenchTx(ctx, tb, eng)
	defer ws.Discard()
	if err := c.op(ctx, tb, ws, i); err != nil {
		tb.Fatal(err.Error())
	}
	if err := ws.Commit(ctx); err != nil {
		tb.Fatal(err.Error())
	}
}

// benchmarkWorldSeededStoredPages applies c in one committed transaction per
// iteration on an Engine over the seeded World whose writes a PendingStore
// keeps, and stores the kept pages every window commits.
func benchmarkWorldSeededStoredPages(b *testing.B, c worldSeededWriteCase, window int) {
	// Open an Engine on the seeded World, keeping its writes in memory.
	ctx := context.Background()
	tbed, seedRef := seededWorld(ctx, b)
	ocs, err := tbed.BuildEmptyCursor(ctx)
	if err != nil {
		b.Fatal(err.Error())
	}
	defer ocs.Release()
	stored := &storedBatchStore{StoreOps: ocs.GetBlockStore()}
	pages := block.NewPendingStore(stored, 1<<30)
	head := seedRef.CloneVT()
	head.BucketId = tbed.BucketId
	cursor := bucket_lookup.NewCursor(ctx, tbed.Bus, tbed.Logger, tbed.StepFactorySet, pages, ocs.GetTransformer(), head, ocs.GetOpArgs(), ocs.GetTransformConf())
	defer cursor.Release()
	le := logrus.NewEntry(logrus.New())
	eng, err := world_block.NewEngine(ctx, le, cursor, world_mock.LookupMockOp, nil, false)
	if err != nil {
		b.Fatal(err.Error())
	}
	defer eng.Close()

	// Store the kept pages the World reaches, counting them by tree and
	// level first, then drop the pages no later World reaches.
	var counts world_block.PageCounts
	var walked time.Duration
	storeWorld := func() {
		root := eng.GetRootRef().GetRootRef()
		start := time.Now()
		err := world_block.WalkPages(ctx, pages, cursor.GetTransformer(), []*block.BlockRef{root}, func(p world_block.Page) (bool, error) {
			size, ok := pages.Unflushed(p.Ref)
			if ok {
				counts.Add(p, size)
			}
			return ok, nil
		})
		walked += time.Since(start)
		if err != nil {
			b.Fatal(err.Error())
		}
		if err := pages.SetRoots(ctx, nil, []block.NamedRoot{{Name: "bench/world", Ref: root}}); err != nil {
			b.Fatal(err.Error())
		}
		pages.Sweep([]*block.BlockRef{root}, pages.SweepMark())
	}

	// Commit c once per iteration, storing every window commits.
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		commitWorldSeededCase(ctx, b, eng, c, i)
		if (i+1)%window == 0 {
			storeWorld()
		}
	}
	if b.N%window != 0 {
		storeWorld()
	}
	b.StopTimer()

	// Report the stored pages per commit, by tree, beside what the stores
	// actually wrote.
	n := float64(b.N)
	var total world_block.PageCount
	byTree := make(map[world_block.PageTree]world_block.PageCount)
	snap := counts.Snapshot()
	levels := slices.SortedFunc(maps.Keys(snap), func(x, y world_block.PageKey) int {
		return cmp.Or(cmp.Compare(x.Tree, y.Tree), cmp.Compare(x.Level, y.Level))
	})
	for _, key := range levels {
		cnt := snap[key]
		t := byTree[key.Tree]
		t.Pages, t.Bytes = t.Pages+cnt.Pages, t.Bytes+cnt.Bytes
		byTree[key.Tree] = t
		total.Pages, total.Bytes = total.Pages+cnt.Pages, total.Bytes+cnt.Bytes
	}
	b.ReportMetric(1, "world_commits/op")
	if c.objects != 0 {
		b.ReportMetric(c.objects, "objects/op")
	}
	b.ReportMetric(float64(total.Pages)/n, "pages/op")
	b.ReportMetric(float64(walked.Nanoseconds())/n, "walk-ns/op")
	b.ReportMetric(float64(total.Bytes)/n, "page-bytes/op")
	b.ReportMetric(float64(stored.bytes)/n, "stored-bytes/op")
	for tree, cnt := range byTree {
		b.ReportMetric(float64(cnt.Bytes)/n, tree.String()+"-bytes/op")
	}

	// Log the split by level.
	for _, key := range levels {
		cnt := snap[key]
		b.Logf("n=%d tree=%s level=%d pages/op=%.3f bytes/op=%.1f", b.N, key.Tree, key.Level, float64(cnt.Pages)/n, float64(cnt.Bytes)/n)
	}
}

// storedBatchStore counts the blocks the PendingStore's root moves store,
// and stores them without moving a root, since a testbed store holds none.
type storedBatchStore struct {
	block.StoreOps
	// bytes is the length of the blocks stored by root moves.
	bytes uint64
}

// SetRoots stores entries, counting them, and moves no root.
func (s *storedBatchStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, _ []block.NamedRoot) error {
	for _, e := range entries {
		if !e.Tombstone {
			s.bytes += uint64(len(e.Data))
		}
	}
	_, err := s.PutBlockBatch(ctx, entries)
	return err
}

// setupWorldSeededBench opens a World Engine on the seeded World, seeding it
// on first use.
func setupWorldSeededBench(ctx context.Context, tb testing.TB) (*world_block.Engine, func()) {
	tb.Helper()
	tbed, rootRef := seededWorld(ctx, tb)
	return openSeededEngine(ctx, tb, tbed, rootRef)
}

// seededWorld returns the testbed and root of the seeded World, seeding the
// shared World once per process, on a context that outlives the first caller.
func seededWorld(ctx context.Context, tb testing.TB) (*testbed.Testbed, *bucket.ObjectRef) {
	tb.Helper()
	worldSeed.once.Do(func() {
		worldSeed.testbed, worldSeed.rootRef, worldSeed.err = seedWorld(context.WithoutCancel(ctx), worldSeedObjects)
	})
	if worldSeed.err != nil {
		tb.Fatal(worldSeed.err.Error())
	}
	return worldSeed.testbed, worldSeed.rootRef
}

// openSeededEngine opens a World Engine on a seeded root of tbed.
func openSeededEngine(
	ctx context.Context,
	tb testing.TB,
	tbed *testbed.Testbed,
	rootRef *bucket.ObjectRef,
) (*world_block.Engine, func()) {
	// Build a cursor at the seeded root.
	tb.Helper()
	ocs, err := tbed.BuildEmptyCursor(ctx)
	if err != nil {
		tb.Fatal(err.Error())
	}
	ocs.SetRootRef(rootRef.GetRootRef())

	// Open the Engine on the cursor.
	le := logrus.NewEntry(logrus.New())
	eng, err := world_block.NewEngine(ctx, le, ocs, world_mock.LookupMockOp, nil, false)
	if err != nil {
		ocs.Release()
		tb.Fatal(err.Error())
	}
	return eng, func() {
		eng.Close()
		ocs.Release()
	}
}

// seedWorld builds a testbed World of objects objects in batches of
// worldSeedObjectsPerTx and returns the testbed and the World root.
func seedWorld(ctx context.Context, objects int) (*testbed.Testbed, *bucket.ObjectRef, error) {
	// Start a testbed with an empty cursor.
	le := logrus.NewEntry(logrus.New())
	tbed, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		return nil, nil, err
	}
	ocs, err := buildNewestWorldCursor(ctx, tbed)
	if err != nil {
		tbed.Release()
		return nil, nil, err
	}
	defer ocs.Release()

	// Build the Engine on the cursor.
	eng, err := world_block.NewEngine(ctx, le, ocs, world_mock.LookupMockOp, nil, false)
	if err != nil {
		tbed.Release()
		return nil, nil, err
	}
	defer eng.Close()

	// Create the type hubs, then the objects in batches.
	if err := seedWorldTx(ctx, eng, func(ws world.WorldState) error {
		for i := range worldSeedTypes {
			obj, err := ws.CreateObject(ctx, worldSeedTypeKey(i), nil)
			world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		tbed.Release()
		return nil, nil, err
	}
	for start := 0; start < objects; start += worldSeedObjectsPerTx {
		if err := seedWorldTx(ctx, eng, func(ws world.WorldState) error {
			return seedWorldObjects(ctx, ws, start, min(start+worldSeedObjectsPerTx, objects))
		}); err != nil {
			tbed.Release()
			return nil, nil, err
		}
	}
	return tbed, eng.GetRootRef(), nil
}

// seedWorldTx applies fn in one committed Engine transaction.
func seedWorldTx(ctx context.Context, eng *world_block.Engine, fn func(ws world.WorldState) error) error {
	// Open the write transaction.
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Apply fn and commit.
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// seedWorldObjects creates the seed objects in [start, end) with their type
// and parent edges.
func seedWorldObjects(ctx context.Context, ws world.WorldState, start, end int) error {
	for i := start; i < end; i++ {
		// Create the object and its type edge.
		key := worldSeedObjectKey(i)
		obj, err := ws.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(key, worldSeedTypePred, worldSeedTypeKey(i), "")); err != nil {
			return err
		}

		// Link every object after the first to an earlier parent.
		if i == 0 {
			continue
		}
		parentQuad := world.NewGraphQuadWithKeys(key, worldSeedParentPred, worldSeedObjectKey(i/2), "")
		if err := ws.SetGraphQuad(ctx, parentQuad); err != nil {
			return err
		}
	}
	return nil
}

// worldSeedObjectKey returns the key of seed object i.
func worldSeedObjectKey(i int) string {
	return worldSeedObjectPrefix + strconv.Itoa(i)
}

// worldSeedObjectPrefix starts each seed object key.
const worldSeedObjectPrefix = "bench/seeded/object/"

// worldSeedTypeKey returns the type hub of seed object i.
func worldSeedTypeKey(i int) string {
	return "bench/seeded/type/" + strconv.Itoa(i%worldSeedTypes)
}

// worldSeedPick spreads iteration i across the seed objects.
func worldSeedPick(i int) int {
	return i * 7919 % worldSeedObjects
}

// setupWorldWriteBench builds a World Engine on an empty testbed bucket, as
// production does.
func setupWorldWriteBench(ctx context.Context, tb testing.TB) (*world_block.Engine, func()) {
	// Start a testbed with an empty bucket cursor.
	tb.Helper()
	le := logrus.NewEntry(logrus.New())
	tbed, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		tb.Fatal(err.Error())
	}
	ocs, err := buildNewestWorldCursor(ctx, tbed)
	if err != nil {
		tbed.Release()
		tb.Fatal(err.Error())
	}

	// Build the Engine on the cursor.
	eng, err := world_block.NewEngine(ctx, le, ocs, world_mock.LookupMockOp, nil, false)
	if err != nil {
		ocs.Release()
		tbed.Release()
		tb.Fatal(err.Error())
	}
	cleanup := func() {
		eng.Close()
		ocs.Release()
		tbed.Release()
	}
	return eng, cleanup
}

// setupWorldState builds a writable WorldState directly on an empty testbed
// bucket, without an Engine.
func setupWorldState(ctx context.Context, tb testing.TB) (*world_block.WorldState, func()) {
	// Start a testbed with an empty bucket cursor.
	tb.Helper()
	le := logrus.NewEntry(logrus.New())
	tbed, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		tb.Fatal(err.Error())
	}
	ocs, err := buildNewestWorldCursor(ctx, tbed)
	if err != nil {
		tbed.Release()
		tb.Fatal(err.Error())
	}

	// Build a writable World on the cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		ocs.Release()
		tbed.Release()
		tb.Fatal(err.Error())
	}
	cleanup := func() {
		ws.Discard()
		ocs.Release()
		tbed.Release()
	}
	return ws, cleanup
}

// buildNewestWorldCursor returns a cursor of tbed rooted at an empty World in
// the newest format, as a new World starts.
func buildNewestWorldCursor(ctx context.Context, tbed *testbed.Testbed) (*bucket_lookup.Cursor, error) {
	// Open an empty cursor and write the World root to it.
	ocs, err := tbed.BuildEmptyCursor(ctx)
	if err != nil {
		return nil, err
	}
	btx, bcs := ocs.BuildTransaction(nil)
	root := world_block.NewWorld(world_block.NewestWorldFormat, false)
	bcs.SetBlock(root, true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		ocs.Release()
		return nil, err
	}
	ocs.SetRootRef(rootRef)
	return ocs, nil
}

// newWorldWriteBenchTx opens a write transaction on the Engine.
func newWorldWriteBenchTx(ctx context.Context, tb testing.TB, eng *world_block.Engine) world.Tx {
	// Open the transaction or fail the benchmark.
	tb.Helper()
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		tb.Fatal(err.Error())
	}
	return tx
}

// createWorldWriteBenchObject creates an empty object and releases its state.
func createWorldWriteBenchObject(ctx context.Context, tb testing.TB, ws world.WorldState, key string) {
	// Create the object and release its state.
	tb.Helper()
	obj, err := ws.CreateObject(ctx, key, nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		tb.Fatal(err.Error())
	}
}

// worldWriteBenchCounts accumulates block reads and writes across iterations.
type worldWriteBenchCounts struct {
	readCount, readBytes   uint64
	writeCount, writeBytes uint64
}

// withWorldWriteBenchCounters returns a context that counts block reads and writes.
func withWorldWriteBenchCounters(ctx context.Context) (context.Context, *block.ReadCounter, *block.WriteCounter) {
	ctx, readCounter := block.WithReadCounter(ctx)
	ctx, writeCounter := block.WithWriteCounter(ctx)
	return ctx, readCounter, writeCounter
}

// add adds one iteration's counter values.
func (c *worldWriteBenchCounts) add(readCounter *block.ReadCounter, writeCounter *block.WriteCounter) {
	// Add the read and write snapshots to the totals.
	reads := readCounter.Snapshot()
	writes := writeCounter.Snapshot()
	c.readCount += reads.BlockReadCount
	c.readBytes += reads.BlockReadBytes
	c.writeCount += writes.BlockWriteCount
	c.writeBytes += writes.BlockWriteBytes
}

// report reports per-iteration block read and write metrics.
func (c *worldWriteBenchCounts) report(b *testing.B) {
	// Report nothing when no iterations ran.
	if b.N == 0 {
		return
	}

	// Report block reads and writes per iteration.
	denom := float64(b.N)
	b.ReportMetric(float64(c.readCount)/denom, "block-reads/op")
	b.ReportMetric(float64(c.readBytes)/denom, "block-read-bytes/op")
	b.ReportMetric(float64(c.writeCount)/denom, "block-writes/op")
	b.ReportMetric(float64(c.writeBytes)/denom, "block-write-bytes/op")
}

// bumpObjectRev increments the revision of the object at key.
func bumpObjectRev(ctx context.Context, ws world.WorldStateObject, key string) error {
	obj, err := world.MustGetObject(ctx, ws, key)
	if err != nil {
		return err
	}
	defer world.ReleaseObjectState(obj)
	_, revErr := obj.IncrementRev(ctx)
	return revErr
}
