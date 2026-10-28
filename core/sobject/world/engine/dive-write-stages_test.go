package sobject_world_engine_test

import (
	"context"
	"os"
	"runtime/pprof"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/testbed"
)

// Seeded SharedObject World shape, as the block World's seeded benchmark:
// every object has a type edge to one of a few hubs and a parent edge.
const (
	diveSeedTypes        = 4
	diveSeedObjectsPerTx = 500
	diveTypePred         = "<dive/type>"
	diveParentPred       = "<dive/parent>"
	diveLinkPred         = "<dive/link>"
)

// diveObjectKey returns the key of seed object i.
func diveObjectKey(i int) string {
	return "dive/object/" + strconv.Itoa(i)
}

// diveTypeKey returns the type hub of seed object i.
func diveTypeKey(i int) string {
	return "dive/type/" + strconv.Itoa(i%diveSeedTypes)
}

// diveEngine is a SharedObject World engine on the local provider.
type diveEngine struct {
	eng     world.Engine
	release func()
}

// startDiveEngine starts a SharedObject World engine on a fresh testbed and
// seeds it with objects objects and twice as many edges.
func startDiveEngine(tb testing.TB, objects int) *diveEngine {
	// Start the testbed with the SharedObject engine and local provider.
	tb.Helper()
	ctx := context.Background()
	tbed, err := testbed.Default(ctx)
	if err != nil {
		tb.Fatal(err)
	}
	var rels []func()
	release := func() {
		for i := len(rels) - 1; i >= 0; i-- {
			rels[i]()
		}
		tbed.Release()
	}
	tbed.StaticResolver.AddFactory(sobject_world_engine.NewFactory(tbed.Bus))
	tbed.StaticResolver.AddFactory(provider_local.NewFactory(tbed.Bus))

	// Load the local provider.
	_, provRef, err := tbed.Bus.AddDirective(ctx, resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: "local",
		PeerId:     tbed.Volume.GetPeerID().String(),
	}), nil)
	if err != nil {
		release()
		tb.Fatal(err)
	}
	rels = append(rels, provRef.Release)

	// Open the account and create the SharedObject.
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(ctx, tbed.Bus, "local", "dive-account", false, nil)
	if err != nil {
		release()
		tb.Fatal(err)
	}
	rels = append(rels, provAccRef.Release)
	soProv, err := sobject.GetSharedObjectProviderAccountFeature(ctx, provAcc)
	if err != nil {
		release()
		tb.Fatal(err)
	}
	soRef, err := soProv.CreateSharedObject(ctx, "dive-shared-object", &sobject.SharedObjectMeta{BodyType: "dive"}, "", "")
	if err != nil {
		release()
		tb.Fatal(err)
	}

	// Serve the mock operations and start the engine.
	engineID := "dive-shared-object-engine"
	opc := world.NewLookupOpController("dive-ops", engineID, world_mock.LookupMockOp)
	relOpc, err := tbed.Bus.AddController(ctx, opc, nil)
	if err != nil {
		release()
		tb.Fatal(err)
	}
	rels = append(rels, relOpc)
	eng, engRef, err := world.StartEngine(ctx, tbed.Bus, sobject_world_engine.NewConfig(engineID, soRef), nil)
	if err != nil {
		release()
		tb.Fatal(err)
	}
	rels = append(rels, engRef.Release)

	// Seed the hubs and the objects in batches, labeled so profiles can drop
	// the seed work.
	de := &diveEngine{eng: eng, release: release}
	var seedErr error
	pprof.Do(ctx, pprof.Labels("dive", "seed"), func(ctx context.Context) {
		seedErr = de.seed(ctx, objects)
	})
	if seedErr != nil {
		release()
		tb.Fatal(seedErr)
	}
	return de
}

// seed writes the type hubs and objects seed objects in batches.
func (d *diveEngine) seed(ctx context.Context, objects int) error {
	if err := d.write(ctx, func(ws world.WorldState) error {
		for i := range diveSeedTypes {
			obj, err := ws.CreateObject(ctx, diveTypeKey(i), nil)
			world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	for start := 0; start < objects; start += diveSeedObjectsPerTx {
		end := min(start+diveSeedObjectsPerTx, objects)
		if err := d.write(ctx, func(ws world.WorldState) error {
			return diveSeedObjects(ctx, ws, start, end)
		}); err != nil {
			return err
		}
	}
	return nil
}

// write runs fn in one committed write transaction.
func (d *diveEngine) write(ctx context.Context, fn func(ws world.WorldState) error) error {
	// Open the write, run fn and commit it.
	tx, err := d.eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// diveSeedObjects creates seed objects [start, end) with their edges.
func diveSeedObjects(ctx context.Context, ws world.WorldState, start, end int) error {
	for i := start; i < end; i++ {
		key := diveObjectKey(i)
		obj, err := ws.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(key, diveTypePred, diveTypeKey(i), "")); err != nil {
			return err
		}
		if i == 0 {
			continue
		}
		if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(key, diveParentPred, diveObjectKey(i/2), "")); err != nil {
			return err
		}
	}
	return nil
}

// stageTotals sums commit stage times across writes.
type stageTotals struct {
	mtx     sync.Mutex
	n       int
	hold    time.Duration
	elapsed time.Duration
	stages  map[string]time.Duration
	order   []string
}

// observe records one write's stage times.
func (s *stageTotals) observe(times sobject_world_engine.CommitTimes) {
	// Add this write to the totals.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.stages == nil {
		s.stages = make(map[string]time.Duration)
	}
	s.n++
	s.hold += times.Hold
	s.elapsed += times.Elapsed
	for _, st := range times.Stages {
		if _, ok := s.stages[st.Name]; !ok {
			s.order = append(s.order, st.Name)
		}
		s.stages[st.Name] += st.Duration
	}
}

// report adds the mean of each stage, in microseconds, to b.
func (s *stageTotals) report(b *testing.B) {
	s.reportPer(b, 1)
}

// reportPer adds the mean of each stage per write, in microseconds, to b,
// where each transaction carries perTx writes.
func (s *stageTotals) reportPer(b *testing.B, perTx int) {
	// Report the mean of each total per write.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.n == 0 {
		return
	}
	n := float64(s.n * perTx)
	b.ReportMetric(float64(s.hold.Microseconds())/n, "hold-us/write")
	b.ReportMetric(float64(s.elapsed.Microseconds())/n, "elapsed-us/write")
	for _, name := range s.order {
		b.ReportMetric(float64(s.stages[name].Microseconds())/n, name+"-us/write")
	}
}

// diveWriteOp is one kind of benchmark write.
type diveWriteOp struct {
	name string
	fn   func(ctx context.Context, ws world.WorldState, i int) error
}

// diveSeedPick spreads iteration i across objects seed objects.
func diveSeedPick(i, objects int) int {
	return i * 7919 % objects
}

// diveWriteOps are the writes the stage benchmark times.
func diveWriteOps(objects int) []diveWriteOp {
	return []diveWriteOp{
		{"link-existing-objects", func(ctx context.Context, ws world.WorldState, i int) error {
			return ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(
				diveObjectKey(diveSeedPick(i, objects)),
				diveLinkPred,
				diveObjectKey(diveSeedPick(i+objects/2, objects)),
				"",
			))
		}},
		{"create-typed-object", func(ctx context.Context, ws world.WorldState, i int) error {
			// Create the object with a type edge and a parent edge.
			key := "dive/new/" + strconv.Itoa(i)
			obj, err := ws.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
			if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(key, diveTypePred, diveTypeKey(i), "")); err != nil {
				return err
			}
			return ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(key, diveParentPred, diveObjectKey(diveSeedPick(i, objects)), ""))
		}},
		{"increment-object-rev", func(ctx context.Context, ws world.WorldState, i int) error {
			// Bump the revision of one seed object.
			obj, err := world.MustGetObject(ctx, ws, diveObjectKey(diveSeedPick(i, objects)))
			if err != nil {
				return err
			}
			defer world.ReleaseObjectState(obj)
			_, err = obj.IncrementRev(ctx)
			return err
		}},
	}
}

// diveObjects returns the seed size, 2,000 unless DIVE_OBJECTS sets it.
func diveObjects(tb testing.TB) int {
	if s := testingEnv("DIVE_OBJECTS"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil {
			tb.Fatal(err)
		}
		return n
	}
	return 2000
}

// BenchmarkDiveWriteStages times each stage of one SharedObject World write
// on a seeded World, one writer at a time.
func BenchmarkDiveWriteStages(b *testing.B) {
	objects := diveObjects(b)
	for _, op := range diveWriteOps(objects) {
		b.Run(op.name, func(b *testing.B) {
			// Seed a World and time each write.
			de := startDiveEngine(b, objects)
			defer de.release()
			var totals stageTotals
			ctx := sobject_world_engine.WithCommitObserver(context.Background(), totals.observe)
			b.ResetTimer()
			pprof.Do(ctx, pprof.Labels("dive", "write"), func(ctx context.Context) {
				for i := range b.N {
					if err := de.write(ctx, func(ws world.WorldState) error {
						return op.fn(ctx, ws, i)
					}); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.StopTimer()
			totals.report(b)
		})
	}
}

// testingEnv reads an environment variable for the dive benchmarks.
func testingEnv(key string) string {
	return os.Getenv(key)
}

// BenchmarkDiveWriteBatch commits batch link writes per transaction. The
// per-write cost at a batch of K bounds an unencoded head that encodes once
// every K writes: the tree work and the encode are shared across the batch.
func BenchmarkDiveWriteBatch(b *testing.B) {
	objects := diveObjects(b)
	link := diveWriteOps(objects)[0]
	for _, batch := range []int{1, 4, 16, 64, 256} {
		b.Run("batch-"+strconv.Itoa(batch), func(b *testing.B) {
			// Seed a World and time each batch.
			de := startDiveEngine(b, objects)
			defer de.release()
			var totals stageTotals
			ctx := sobject_world_engine.WithCommitObserver(context.Background(), totals.observe)
			b.ResetTimer()
			pprof.Do(ctx, pprof.Labels("dive", "write"), func(ctx context.Context) {
				for i := range b.N {
					if err := de.write(ctx, func(ws world.WorldState) error {
						for j := range batch {
							if err := link.fn(ctx, ws, i*batch+j); err != nil {
								return err
							}
						}
						return nil
					}); err != nil {
						b.Fatal(err)
					}
				}
			})
			b.StopTimer()
			totals.reportPer(b, batch)
		})
	}
}

// BenchmarkDiveConcurrentWriters runs link writes from several writers at
// once and reports throughput and the mean writer lock hold. DIVE_RUN_AHEAD=1
// runs optimistic writes ahead of the lock; DIVE_READ_DELAY_US adds a sleep
// to each callback, standing in for a cold block read.
func BenchmarkDiveConcurrentWriters(b *testing.B) {
	objects := diveObjects(b)
	link := diveWriteOps(objects)[0]
	delay := time.Duration(diveEnvInt(b, "DIVE_READ_DELAY_US", 0)) * time.Microsecond
	for _, writers := range []int{1, 4, 16} {
		b.Run("writers-"+strconv.Itoa(writers), func(b *testing.B) {
			// Seed a World and mark every write optimistic.
			de := startDiveEngine(b, objects)
			defer de.release()
			var totals stageTotals
			ctx := sobject_world_engine.WithCommitObserver(context.Background(), totals.observe)
			ctx = world.WithOptimisticWrite(ctx)

			// Split b.N writes across the writers.
			var next atomic.Int64
			var wg sync.WaitGroup
			errs := make(chan error, writers)
			b.ResetTimer()
			for range writers {
				wg.Go(func() {
					pprof.Do(ctx, pprof.Labels("dive", "write"), func(ctx context.Context) {
						for {
							i := int(next.Add(1)) - 1
							if i >= b.N {
								return
							}
							if err := de.write(ctx, func(ws world.WorldState) error {
								if delay != 0 {
									time.Sleep(delay)
								}
								return link.fn(ctx, ws, i)
							}); err != nil {
								errs <- err
								return
							}
						}
					})
				})
			}
			wg.Wait()
			b.StopTimer()

			// Report the first failure, the throughput and the stage means.
			select {
			case err := <-errs:
				b.Fatal(err)
			default:
			}
			b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "writes/s")
			totals.report(b)
		})
	}
}

// diveEnvInt reads an integer environment variable, or def when it is unset.
func diveEnvInt(tb testing.TB, key string, def int) int {
	// Parse the variable when it is set.
	s := testingEnv(key)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		tb.Fatal(err)
	}
	return n
}
