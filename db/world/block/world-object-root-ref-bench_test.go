package world_block_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

func BenchmarkWorldStateGetObjectRootRefsBatch(b *testing.B) {
	ctx := context.Background()
	ws, keys, cleanup := setupObjectRootRefBenchWorld(ctx, b, 256)
	defer cleanup()

	b.Run("object-root-ref-loop", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			var found int
			for _, key := range keys {
				obj, exists, err := ws.GetObject(ctx, key)
				if err != nil {
					b.Fatal(err.Error())
				}
				if !exists {
					continue
				}
				if _, _, err := obj.GetRootRef(ctx); err != nil {
					b.Fatal(err.Error())
				}
				found++
			}
			if found != len(keys) {
				b.Fatalf("found = %d, want %d", found, len(keys))
			}
		}
	})

	b.Run("owner-batch", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			refs, err := world.GetObjectRootRefsBatch(ctx, ws, keys)
			if err != nil {
				b.Fatal(err.Error())
			}
			var found int
			for _, ref := range refs {
				if ref.Exists {
					found++
				}
			}
			if found != len(keys) {
				b.Fatalf("found = %d, want %d", found, len(keys))
			}
		}
	})
}

func setupObjectRootRefBenchWorld(ctx context.Context, tb testing.TB, count int) (*world_block.WorldState, []string, func()) {
	tb.Helper()

	le := logrus.NewEntry(logrus.New())
	tbed, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		tb.Fatal(err.Error())
	}
	ocs, err := tbed.BuildEmptyCursor(ctx)
	if err != nil {
		tbed.Release()
		tb.Fatal(err.Error())
	}
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		ocs.Release()
		tbed.Release()
		tb.Fatal(err.Error())
	}

	keys := make([]string, 0, count)
	for i := range count {
		key := "bench/root-ref/" + strconv.Itoa(i)
		if _, err := ws.CreateObject(ctx, key, &bucket.ObjectRef{BucketId: "bucket-" + strconv.Itoa(i)}); err != nil {
			ws.Discard()
			ocs.Release()
			tbed.Release()
			tb.Fatal(err.Error())
		}
		keys = append(keys, key)
	}

	cleanup := func() {
		ws.Discard()
		ocs.Release()
		tbed.Release()
	}
	return ws, keys, cleanup
}
