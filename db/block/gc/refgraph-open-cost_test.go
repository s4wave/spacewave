package block_gc

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/cayley/graph"
	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestNewRefGraphOpenCostIsIndependentOfEdgeCount pins the property that makes
// a RefGraph usable on a large store: opening one reads a fixed number of keys
// no matter how many gc/ref edges the graph holds. A RefGraph that materializes
// its edges at open turns every process start into a full scan of the store,
// and the store is meant to hold arbitrary amounts of data.
func TestNewRefGraphOpenCostIsIndependentOfEdgeCount(t *testing.T) {
	ctx := context.Background()
	small := openCostForEdgeCount(t, ctx, 16)
	large := openCostForEdgeCount(t, ctx, 2048)
	if small != large {
		t.Fatalf(
			"opening a graph of 16 edges read %d keys and one of 2048 edges read %d; open scales with edge count",
			small, large,
		)
	}
}

func TestAddRefDuplicateDoesNotCommit(t *testing.T) {
	// Open an empty reference graph to seed the measured workload.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	seed, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}

	// Seed the ownership edge used by the duplicate-cost check.
	if err := seed.AddRef(ctx, "owner", "target"); err != nil {
		t.Fatal(err)
	}

	// Close the seeded graph before reopening it for measurement.
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen the seeded graph through a transaction-counting store.
	counted := &countingStore{Store: store}
	rg, err := NewRefGraph(ctx, counted, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	defer rg.Close()

	// Write the duplicate edge while counting commits.
	before := counted.commits.Load()
	if err := rg.AddRef(ctx, "owner", "target"); err != nil {
		t.Fatal(err)
	}

	// Verify the duplicate edge causes no commit.
	if got := counted.commits.Load(); got != before {
		t.Fatalf("duplicate AddRef committed %d transaction(s)", got-before)
	}

	// Write a new edge to contrast its commit cost.
	if err := rg.AddRef(ctx, "owner", "other-target"); err != nil {
		t.Fatal(err)
	}

	// Verify the new edge commits a transaction.
	if got := counted.commits.Load(); got <= before {
		t.Fatal("new AddRef did not commit a transaction")
	}
}

func TestAddRefDuplicateReadCostIsIndependentOfEdgeChurn(t *testing.T) {
	small := addRefDuplicateReadCostForChurn(t, 1)
	large := addRefDuplicateReadCostForChurn(t, 32)
	if small != large {
		t.Fatalf("adding a duplicate ref read %d keys after 1 churn cycle and %d after 32", small, large)
	}
}

func addRefDuplicateReadCostForChurn(t *testing.T, churn int) int64 {
	// Open an empty reference graph to seed the measured workload.
	t.Helper()
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	seed, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}

	// Seed anchored source and target nodes before edge churn.
	if err := seed.ApplyRefBatch(ctx, []RefEdge{
		{Subject: "owner", Object: "owner-anchor"},
		{Subject: "target-anchor", Object: "target"},
		{Subject: "owner", Object: "target"},
	}, nil); err != nil {
		t.Fatal(err)
	}

	// Remove and restore the target edge for each churn cycle.
	for range churn {
		// Remove the ownership edge during a churn cycle.
		if err := seed.RemoveRef(ctx, "owner", "target"); err != nil {
			t.Fatal(err)
		}

		// Restore the ownership edge during a churn cycle.
		if err := seed.AddRef(ctx, "owner", "target"); err != nil {
			t.Fatal(err)
		}
	}

	// Close the seeded graph before reopening it for measurement.
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen the seeded graph through a transaction-counting store.
	counted := &countingStore{Store: store}
	rg, err := NewRefGraph(ctx, counted, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	defer rg.Close()

	// Measure key reads for the new or duplicate edge.
	before := counted.reads.Load()
	if err := rg.AddRef(ctx, "owner", "target"); err != nil {
		t.Fatal(err)
	}
	return counted.reads.Load() - before
}

func TestAddRefNewEdgeReadCostIsIndependentOfTargetFanIn(t *testing.T) {
	small := addRefReadCostForTargetFanIn(t, 1)
	large := addRefReadCostForTargetFanIn(t, 128)
	if small != large {
		t.Fatalf("adding a new ref read %d keys at fan-in 1 and %d at fan-in 128", small, large)
	}
}

func TestHasRefRejectsSubjectIDPrefixCollision(t *testing.T) {
	// Open an empty reference graph to seed the measured workload.
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	rg, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	defer rg.Close()

	// Seed graph nodes with potentially colliding subject ID prefixes.
	const subjectCount = 256
	edges := make([]RefEdge, subjectCount+1)
	subjects := make([]string, subjectCount)
	for i := range subjects {
		subjects[i] = "subject/" + strconv.Itoa(i)
		edges[i] = RefEdge{Subject: subjects[i], Object: "seed-target"}
	}
	edges[subjectCount] = RefEdge{Subject: "target", Object: "seed-target"}
	if err := rg.ApplyRefBatch(ctx, edges, nil); err != nil {
		t.Fatal(err)
	}

	// Resolve the graph node IDs used to construct prefix collisions.
	qs, ok := graph.Unwrap(rg.handle.QuadStore).(*cayley_kv.QuadStore)
	if !ok {
		t.Fatalf("unexpected quad store %T", graph.Unwrap(rg.handle.QuadStore))
	}
	names := append([]string{PredGCRef, "target"}, subjects...)
	ids, err := resolveIRIRefIDs(ctx, qs, names)
	if err != nil {
		t.Fatal(err)
	}

	// Find an absent edge whose subject ID prefix collides.
	absent, collisions := findSubjectPrefixCollisions(ids, subjects, 1)
	if absent == "" {
		t.Fatal("fixture did not produce a subject ID prefix collision")
	}

	// Choose the existing collision edge for the absent-edge check.
	existing := collisions[0]

	// Write the collision edge that shares the absent edge prefix.
	if err := rg.AddRef(ctx, existing, "target"); err != nil {
		t.Fatal(err)
	}

	// Verify the exact absent-edge lookup result.
	if found, err := rg.hasRef(ctx, absent, "target"); err != nil {
		t.Fatal(err)
	} else if found {
		t.Fatalf("edge %q -> target matched existing edge %q -> target", absent, existing)
	}

	// Write the exact edge whose prefix previously collided.
	if err := rg.AddRef(ctx, absent, "target"); err != nil {
		t.Fatal(err)
	}

	// Verify the exact absent-edge lookup result.
	if found, err := rg.hasRef(ctx, absent, "target"); err != nil {
		t.Fatal(err)
	} else if !found {
		t.Fatalf("edge %q -> target was not stored", absent)
	}
}

func TestAddRefAbsentEdgeReadCostIsIndependentOfDeletedPrefixCollisions(t *testing.T) {
	small := addRefReadCostForDeletedPrefixCollisions(t, 1)
	large := addRefReadCostForDeletedPrefixCollisions(t, 32)
	if small != large {
		t.Fatalf("adding an absent ref read %d keys with 1 deleted prefix collision and %d with 32", small, large)
	}
}

func addRefReadCostForDeletedPrefixCollisions(t *testing.T, collisionCount int) int64 {
	// Open an empty reference graph to seed the measured workload.
	t.Helper()
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	seed, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}

	// Seed graph nodes with potentially colliding subject ID prefixes.
	const subjectCount = 1024
	edges := make([]RefEdge, subjectCount+1)
	subjects := make([]string, subjectCount)
	for i := range subjects {
		subjects[i] = "deleted-collision/subject/" + strconv.Itoa(i)
		edges[i] = RefEdge{Subject: subjects[i], Object: "seed-target"}
	}
	edges[subjectCount] = RefEdge{Subject: "target", Object: "seed-target"}
	if err := seed.ApplyRefBatch(ctx, edges, nil); err != nil {
		t.Fatal(err)
	}

	// Resolve the graph node IDs used to construct prefix collisions.
	qs, ok := graph.Unwrap(seed.handle.QuadStore).(*cayley_kv.QuadStore)
	if !ok {
		t.Fatalf("unexpected quad store %T", graph.Unwrap(seed.handle.QuadStore))
	}
	names := append([]string{PredGCRef, "target"}, subjects...)
	ids, err := resolveIRIRefIDs(ctx, qs, names)
	if err != nil {
		t.Fatal(err)
	}

	// Find an absent edge whose subject ID prefix collides.
	absent, collisions := findSubjectPrefixCollisions(ids, subjects, collisionCount)
	if absent == "" {
		t.Fatalf("fixture did not produce %d subject ID prefix collisions", collisionCount)
	}

	// Create and delete the colliding edges before measuring reads.
	collisionEdges := make([]RefEdge, len(collisions))
	for i, subject := range collisions {
		collisionEdges[i] = RefEdge{Subject: subject, Object: "target"}
	}
	if err := seed.ApplyRefBatch(ctx, collisionEdges, nil); err != nil {
		t.Fatal(err)
	}

	// Delete the collision edges while retaining their index history.
	if err := seed.ApplyRefBatch(ctx, nil, collisionEdges); err != nil {
		t.Fatal(err)
	}

	// Close the seeded graph before reopening it for measurement.
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen the seeded graph through a transaction-counting store.
	counted := &countingStore{Store: store}
	rg, err := NewRefGraph(ctx, counted, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	defer rg.Close()

	// Measure key reads for the new or duplicate edge.
	before := counted.reads.Load()

	// Write the exact edge whose prefix previously collided.
	if err := rg.AddRef(ctx, absent, "target"); err != nil {
		t.Fatal(err)
	}
	return counted.reads.Load() - before
}

func findSubjectPrefixCollisions(
	ids map[string]uint64,
	subjects []string,
	limit int,
) (string, []string) {
	// Search reverse-index keys for distinct subject prefix collisions.
	index := cayley_kv.DefaultQuadIndexes[1]
	for _, candidate := range subjects {
		// Find extensions sharing the candidate subject key prefix.
		candidateKey := index.Key([]uint64{ids["target"], ids[PredGCRef], ids[candidate]})
		collisions := make([]string, 0, limit)
		for _, extension := range subjects {
			// Compare the extension key and retain matching prefix collisions.
			extensionKey := index.Key([]uint64{ids["target"], ids[PredGCRef], ids[extension]})
			if extensionKey.Compare(candidateKey) != 0 && extensionKey.HasPrefix(candidateKey) {
				// Retain prefix collisions until the requested count is reached.
				collisions = append(collisions, extension)
				if len(collisions) == limit {
					return candidate, collisions
				}
			}
		}
	}
	return "", nil
}

func addRefReadCostForTargetFanIn(t *testing.T, fanIn int) int64 {
	// Open an empty reference graph to seed the measured workload.
	t.Helper()
	ctx := context.Background()
	store := store_kvtx_inmem.NewStore()
	seed, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}

	// Seed the target fan-in and the prospective new owner.
	edges := make([]RefEdge, fanIn+1)
	for idx := range edges {
		edges[idx] = RefEdge{
			Subject: "owner/" + strconv.Itoa(idx),
			Object:  "target",
		}
	}
	edges[fanIn] = RefEdge{Subject: "new-owner", Object: "seed-target"}
	if err := seed.ApplyRefBatch(ctx, edges, nil); err != nil {
		t.Fatal(err)
	}

	// Close the seeded graph before reopening it for measurement.
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen the seeded graph through a transaction-counting store.
	counted := &countingStore{Store: store}
	rg, err := NewRefGraph(ctx, counted, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	defer rg.Close()

	// Measure key reads for the new or duplicate edge.
	before := counted.reads.Load()
	if err := rg.AddRef(ctx, "new-owner", "target"); err != nil {
		t.Fatal(err)
	}
	return counted.reads.Load() - before
}

// openCostForEdgeCount seeds edgeCount gc/ref edges into a fresh store, then
// reports how many key reads a second RefGraph over that same store performs
// while opening.
func openCostForEdgeCount(t *testing.T, ctx context.Context, edgeCount int) int64 {
	// Open an empty reference graph to seed the measured workload.
	t.Helper()
	store := store_kvtx_inmem.NewStore()
	seed, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}

	// Seed the requested number of reference graph edges.
	edges := make([]RefEdge, edgeCount)
	for i := range edges {
		edges[i] = RefEdge{
			Subject: "open-cost/subject/" + strconv.Itoa(i),
			Object:  "open-cost/object/" + strconv.Itoa(i),
		}
	}
	if err := seed.ApplyRefBatch(ctx, edges, nil); err != nil {
		t.Fatal(err)
	}

	// Close the seeded graph before reopening it for measurement.
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen the seeded graph through a transaction-counting store.
	counted := &countingStore{Store: store}
	rg, err := NewRefGraph(ctx, counted, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}

	// Capture opening reads and register the reopened graph cleanup.
	reads := counted.reads.Load()
	t.Cleanup(func() { _ = rg.Close() })
	return reads
}

// countingStore counts the keys read through every transaction it hands out.
type countingStore struct {
	kvtx.Store
	reads   atomic.Int64
	commits atomic.Int64
	opens   atomic.Int64
}

func (s *countingStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	s.opens.Add(1)
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return &countingTx{Tx: tx, store: s}, nil
}

type countingTx struct {
	kvtx.Tx
	store *countingStore
}

func (t *countingTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	t.store.reads.Add(1)
	return t.Tx.Get(ctx, key)
}

func (t *countingTx) Exists(ctx context.Context, key []byte) (bool, error) {
	t.store.reads.Add(1)
	return t.Tx.Exists(ctx, key)
}

func (t *countingTx) Commit(ctx context.Context) error {
	if err := t.Tx.Commit(ctx); err != nil {
		return err
	}
	t.store.commits.Add(1)
	return nil
}

func (t *countingTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	return t.Tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		t.store.reads.Add(1)
		return cb(key, value)
	})
}

func (t *countingTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.Tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
		t.store.reads.Add(1)
		return cb(key)
	})
}

func (t *countingTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	return &countingIterator{
		Iterator: t.Tx.Iterate(ctx, prefix, sort, reverse),
		store:    t.store,
	}
}

// countingIterator charges one read per entry the caller steps onto, so a scan
// of the whole graph costs as much as reading every key by hand.
type countingIterator struct {
	kvtx.Iterator
	store *countingStore
}

func (it *countingIterator) Next() bool {
	if !it.Iterator.Next() {
		return false
	}
	it.store.reads.Add(1)
	return true
}
