package world_block

import (
	"context"
	"strconv"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

const (
	rootUpdateReadBenchObjectCount = 64
	rootUpdateReadBenchRootCount   = 16
)

func BenchmarkEngineRootUpdateReadAttribution(b *testing.B) {
	for _, tc := range []struct {
		name         string
		decodedCache bool
	}{
		{name: "no-decoded-cache"},
		{name: "shared-decoded-cache", decodedCache: true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.Run("root-update-rebuild", func(b *testing.B) {
				ctx := b.Context()
				fixture, cleanup := setupRootUpdateReadBench(ctx, b, tc.decodedCache)
				defer cleanup()
				fixture.waitDecodedCache()
				if err := fixture.publishRoot(ctx, fixture.roots[len(fixture.roots)-1]); err != nil {
					b.Fatal(err.Error())
				}

				b.ResetTimer()
				b.ReportAllocs()
				metrics := rootUpdateReadBenchMetrics{}
				for i := range b.N {
					opCtx, counter := block.WithReadCounter(ctx)
					if err := fixture.publishRoot(opCtx, fixture.roots[i%len(fixture.roots)]); err != nil {
						b.Fatal(err.Error())
					}
					metrics.add(counter.Snapshot())
				}
				metrics.report(b)
			})

			b.Run("object-refgraph-lookups", func(b *testing.B) {
				ctx := b.Context()
				fixture, cleanup := setupRootUpdateReadBench(ctx, b, tc.decodedCache)
				defer cleanup()
				fixture.waitDecodedCache()
				if err := fixture.publishRoot(ctx, fixture.roots[len(fixture.roots)-1]); err != nil {
					b.Fatal(err.Error())
				}

				b.ResetTimer()
				b.StopTimer()
				b.ReportAllocs()
				metrics := rootUpdateReadBenchMetrics{}
				for i := range b.N {
					if err := fixture.publishRoot(ctx, fixture.roots[i%len(fixture.roots)]); err != nil {
						b.Fatal(err.Error())
					}
					readTx, err := fixture.engine.NewBlockEngineTransaction(ctx, false)
					if err != nil {
						b.Fatal(err.Error())
					}
					opCtx, counter := block.WithReadCounter(ctx)

					b.StartTimer()
					readErr := fixture.readObjectAndGraphPaths(opCtx, readTx)
					b.StopTimer()

					readTx.Discard()
					if readErr != nil {
						b.Fatal(readErr.Error())
					}
					metrics.add(counter.Snapshot())
				}
				metrics.report(b)
			})

			b.Run("root-update-with-lookups", func(b *testing.B) {
				ctx := b.Context()
				fixture, cleanup := setupRootUpdateReadBench(ctx, b, tc.decodedCache)
				defer cleanup()
				fixture.waitDecodedCache()
				if err := fixture.publishRoot(ctx, fixture.roots[len(fixture.roots)-1]); err != nil {
					b.Fatal(err.Error())
				}

				b.ResetTimer()
				b.ReportAllocs()
				metrics := rootUpdateReadBenchMetrics{}
				for i := range b.N {
					opCtx, counter := block.WithReadCounter(ctx)
					if err := fixture.publishRoot(opCtx, fixture.roots[i%len(fixture.roots)]); err != nil {
						b.Fatal(err.Error())
					}
					readTx, err := fixture.engine.NewBlockEngineTransaction(opCtx, false)
					if err != nil {
						b.Fatal(err.Error())
					}
					if err := fixture.readObjectAndGraphPaths(opCtx, readTx); err != nil {
						readTx.Discard()
						b.Fatal(err.Error())
					}
					readTx.Discard()
					metrics.add(counter.Snapshot())
				}
				metrics.report(b)
			})
		})
	}
}

func TestEngineDefaultDecodedCacheReusesRepeatedReads(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name         string
		decodedCache bool
	}{
		{name: "default-cache", decodedCache: true},
		{name: "disabled-cache"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture, cleanup := setupRootUpdateReadBench(ctx, t, tc.decodedCache)
			defer cleanup()

			firstRoot := fixture.roots[1]
			secondRoot := fixture.roots[2]
			_ = fixture.readSnapshotAtRoot(t, ctx, firstRoot, true)
			snapshot := fixture.readSnapshotAtRoot(t, ctx, secondRoot, true)

			if tc.decodedCache {
				if snapshot.DecodedBlockCacheAttemptCount == 0 || snapshot.DecodedBlockCacheHitCount == 0 {
					t.Fatalf("default engine reads must use the engine-owned decoded cache, got %+v", snapshot)
				}
				return
			}

			if snapshot.DecodedBlockCacheAttemptCount != 0 ||
				snapshot.DecodedBlockCacheHitCount != 0 ||
				snapshot.DecodedBlockCacheMissCount != 0 ||
				snapshot.DecodedBlockStoreAttemptCount != 0 {
				t.Fatalf("disabled decoded cache must preserve uncached read attribution, got %+v", snapshot)
			}
			if snapshot.DecodedBlockUnmarshalCount == 0 {
				t.Fatalf("disabled decoded cache read should still decode blocks, got %+v", snapshot)
			}
		})
	}
}

func TestEngineDecodedCacheRootChangeInvalidatesOnlyOldRoot(t *testing.T) {
	ctx := context.Background()
	fixture, cleanup := setupRootUpdateReadBench(ctx, t, true)
	defer cleanup()

	oldRoot := fixture.roots[1]
	nextRoot := fixture.roots[2]
	_ = fixture.readSnapshotAtRoot(t, ctx, oldRoot, true)
	_ = fixture.readSnapshotAtRoot(t, ctx, nextRoot, true)

	snapshot := fixture.readSnapshotAtRoot(t, ctx, oldRoot, false)
	if snapshot.DecodedBlockCacheMissCount == 0 {
		t.Fatalf("returning to the invalidated old root should miss that root entry, got %+v", snapshot)
	}
	if snapshot.DecodedBlockCacheHitCount == 0 {
		t.Fatalf("root change should retain shared child decoded entries, got %+v", snapshot)
	}
}

type rootUpdateReadBenchFixture struct {
	engine        *Engine
	decodedBlocks *block.DecodedBlockCache
	roots         []*bucket.ObjectRef
	objectKeys    []string
	graphFilter   []world.GraphQuad
}

func setupRootUpdateReadBench(ctx context.Context, tb testing.TB, decodedCache bool) (*rootUpdateReadBenchFixture, func()) {
	tb.Helper()

	log := logrus.New()
	log.SetLevel(logrus.WarnLevel)
	le := logrus.NewEntry(log)
	tbed, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		tb.Fatal(err.Error())
	}
	root, err := tbed.BuildEmptyCursor(ctx)
	if err != nil {
		tbed.Release()
		tb.Fatal(err.Error())
	}

	var opts []EngineOption
	if !decodedCache {
		opts = append(opts, WithDecodedBlockCacheOptions(block.DecodedBlockCacheOptions{Disabled: true}))
	}

	eng, err := NewEngine(ctx, le, root, world_mock.LookupMockOp, nil, false, opts...)
	if err != nil {
		root.Release()
		tbed.Release()
		tb.Fatal(err.Error())
	}

	cleanup := func() {
		if err := eng.Close(); err != nil {
			tb.Fatal(err.Error())
		}
		tbed.Release()
	}

	objectKeys := make([]string, rootUpdateReadBenchObjectCount)
	graphFilters := make([]world.GraphQuad, rootUpdateReadBenchObjectCount)
	writeTx, err := eng.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		cleanup()
		tb.Fatal(err.Error())
	}
	for i := range rootUpdateReadBenchObjectCount {
		objectKey := "bench/root-update/object/" + strconv.Itoa(i)
		targetKey := objectKey + "/target"
		objectKeys[i] = objectKey
		graphFilters[i] = world.NewGraphQuadWithKeys(objectKey, "<bench/root-update/ref>", "", "")
		if _, err := writeTx.CreateObject(ctx, objectKey, nil); err != nil {
			writeTx.Discard()
			cleanup()
			tb.Fatal(err.Error())
		}
		if _, err := writeTx.CreateObject(ctx, targetKey, nil); err != nil {
			writeTx.Discard()
			cleanup()
			tb.Fatal(err.Error())
		}
		if err := writeTx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(objectKey, "<bench/root-update/ref>", targetKey, "")); err != nil {
			writeTx.Discard()
			cleanup()
			tb.Fatal(err.Error())
		}
	}
	if err := writeTx.Commit(ctx); err != nil {
		cleanup()
		tb.Fatal(err.Error())
	}

	roots := make([]*bucket.ObjectRef, 0, rootUpdateReadBenchRootCount)
	roots = append(roots, eng.GetRootRef())
	for version := 1; version < rootUpdateReadBenchRootCount; version++ {
		writeTx, err := eng.NewBlockEngineTransaction(ctx, true)
		if err != nil {
			cleanup()
			tb.Fatal(err.Error())
		}
		obj, found, err := writeTx.GetObject(ctx, objectKeys[version%len(objectKeys)])
		if err != nil {
			writeTx.Discard()
			cleanup()
			tb.Fatal(err.Error())
		}
		if !found {
			writeTx.Discard()
			cleanup()
			tb.Fatalf("object %q not found", objectKeys[version%len(objectKeys)])
		}
		if _, err := obj.IncrementRev(ctx); err != nil {
			writeTx.Discard()
			cleanup()
			tb.Fatal(err.Error())
		}
		if err := writeTx.Commit(ctx); err != nil {
			cleanup()
			tb.Fatal(err.Error())
		}
		roots = append(roots, eng.GetRootRef())
	}

	fixture := &rootUpdateReadBenchFixture{
		engine:        eng,
		decodedBlocks: eng.decodedBlocks,
		roots:         roots,
		objectKeys:    objectKeys,
		graphFilter:   graphFilters,
	}
	return fixture, cleanup
}

func (f *rootUpdateReadBenchFixture) waitDecodedCache() {
	if f.decodedBlocks != nil {
		f.decodedBlocks.Wait()
	}
}

func (f *rootUpdateReadBenchFixture) publishRoot(ctx context.Context, ref *bucket.ObjectRef) error {
	f.engine.rmtx.Lock()
	defer f.engine.rmtx.Unlock()
	return f.engine.setRootRefLocked(ctx, ref)
}

func (f *rootUpdateReadBenchFixture) readSnapshotAtRoot(t *testing.T, ctx context.Context, ref *bucket.ObjectRef, waitAfterPublish bool) block.ReadCounterSnapshot {
	t.Helper()

	opCtx, counter := block.WithReadCounter(ctx)
	if err := f.publishRoot(opCtx, ref); err != nil {
		t.Fatal(err.Error())
	}
	if waitAfterPublish {
		f.waitDecodedCache()
	}
	readTx, err := f.engine.NewBlockEngineTransaction(opCtx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readTx.Discard()
	if err := f.readObjectAndGraphPaths(opCtx, readTx); err != nil {
		t.Fatal(err.Error())
	}
	f.waitDecodedCache()
	return counter.Snapshot()
}

func (f *rootUpdateReadBenchFixture) readObjectAndGraphPaths(ctx context.Context, readTx *EngineTx) error {
	refs, err := readTx.GetObjectRootRefsBatch(ctx, f.objectKeys)
	if err != nil {
		return err
	}
	var foundObjects int
	for _, ref := range refs {
		if ref.Exists {
			foundObjects++
		}
	}
	if foundObjects != len(f.objectKeys) {
		return errors.Errorf("object refs found = %d, want %d", foundObjects, len(f.objectKeys))
	}

	results, err := readTx.LookupGraphQuadsBatch(ctx, f.graphFilter, 4)
	if err != nil {
		return err
	}
	var foundQuads int
	for _, quads := range results {
		foundQuads += len(quads)
	}
	if foundQuads != len(f.graphFilter) {
		return errors.Errorf("graph quads found = %d, want %d", foundQuads, len(f.graphFilter))
	}
	return nil
}

type rootUpdateReadBenchMetrics struct {
	readCount       uint64
	readBytes       uint64
	unmarshalCount  uint64
	unmarshalBytes  uint64
	cacheAttempts   uint64
	cacheHits       uint64
	cacheMisses     uint64
	cacheClones     uint64
	cacheStoreCount uint64
}

func (m *rootUpdateReadBenchMetrics) add(snapshot block.ReadCounterSnapshot) {
	m.readCount += snapshot.BlockReadCount
	m.readBytes += snapshot.BlockReadBytes
	m.unmarshalCount += snapshot.DecodedBlockUnmarshalCount
	m.unmarshalBytes += snapshot.DecodedBlockUnmarshalBytes
	m.cacheAttempts += snapshot.DecodedBlockCacheAttemptCount
	m.cacheHits += snapshot.DecodedBlockCacheHitCount
	m.cacheMisses += snapshot.DecodedBlockCacheMissCount
	m.cacheClones += snapshot.DecodedBlockCloneCount
	m.cacheStoreCount += snapshot.DecodedBlockStoreAcceptedCount
}

func (m rootUpdateReadBenchMetrics) report(b *testing.B) {
	if b.N == 0 {
		return
	}
	denom := float64(b.N)
	b.ReportMetric(float64(m.readCount)/denom, "block-reads/op")
	b.ReportMetric(float64(m.readBytes)/denom, "block-read-bytes/op")
	b.ReportMetric(float64(m.unmarshalCount)/denom, "decoded-unmarshals/op")
	b.ReportMetric(float64(m.unmarshalBytes)/denom, "decoded-unmarshal-bytes/op")
	b.ReportMetric(float64(m.cacheAttempts)/denom, "decoded-cache-attempts/op")
	b.ReportMetric(float64(m.cacheHits)/denom, "decoded-cache-hits/op")
	b.ReportMetric(float64(m.cacheMisses)/denom, "decoded-cache-misses/op")
	b.ReportMetric(float64(m.cacheClones)/denom, "decoded-cache-clones/op")
	b.ReportMetric(float64(m.cacheStoreCount)/denom, "decoded-cache-stores/op")
}
