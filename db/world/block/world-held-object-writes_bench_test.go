package world_block

import (
	"fmt"
	"math/rand/v2"
	"testing"

	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// BenchmarkHeldObjectWrites measures one commit on an okra object tree that
// revises 200 of 1,000 objects, some twice, and creates and deletes 20, with the object tree's
// writes held until the commit and applied at once.
func BenchmarkHeldObjectWrites(b *testing.B) {
	for _, mode := range []struct {
		name   string
		unheld bool
	}{{"held", false}, {"immediate", true}} {
		b.Run(mode.name, func(b *testing.B) {
			prev := objectWritesUnheld
			objectWritesUnheld = mode.unheld
			b.Cleanup(func() { objectWritesUnheld = prev })

			// Open an Engine and commit 1,000 objects.
			ctx := b.Context()
			le := logrus.NewEntry(logrus.New())
			le.Logger.SetLevel(logrus.WarnLevel)
			tb, err := testbed.NewTestbed(ctx, le)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(tb.Release)
			base, err := tb.BuildEmptyCursor(ctx)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(base.Release)
			btx, bcs := base.BuildTransaction(nil)
			root := NewWorld(false)
			root.ObjectKeyValue = kvtx_block.NewKeyValueStore(kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA)
			bcs.SetBlock(root, true)
			rootRef, _, err := btx.Write(ctx, true)
			if err != nil {
				b.Fatal(err)
			}
			base.SetRootRef(rootRef)
			engine, err := NewEngine(ctx, le, base, world_mock.LookupMockOp, nil, false)
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { _ = engine.Close() })
			writer, err := engine.NewBlockEngineTransaction(ctx, true)
			if err != nil {
				b.Fatal(err)
			}
			for i := range 1000 {
				obj, err := writer.CreateObject(ctx, fmt.Sprintf("obj/%04d", i), nil)
				world.ReleaseObjectState(obj)
				if err != nil {
					b.Fatal(err)
				}
			}
			if err := writer.Commit(ctx); err != nil {
				b.Fatal(err)
			}

			rng := rand.New(rand.NewPCG(1, 2))
			b.ResetTimer()
			for n := range b.N {
				writer, err := engine.NewBlockEngineTransaction(ctx, true)
				if err != nil {
					b.Fatal(err)
				}
				for range 200 {
					obj, found, err := writer.GetObject(ctx, fmt.Sprintf("obj/%04d", rng.IntN(1000)))
					if err != nil {
						b.Fatal(err)
					}
					if !found {
						continue
					}
					if _, err := obj.IncrementRev(ctx); err != nil {
						b.Fatal(err)
					}
					world.ReleaseObjectState(obj)
				}
				for i := range 20 {
					key := fmt.Sprintf("tmp/%d/%02d", n, i)
					obj, err := writer.CreateObject(ctx, key, nil)
					world.ReleaseObjectState(obj)
					if err != nil {
						b.Fatal(err)
					}
					if n > 0 {
						if _, err := writer.DeleteObject(ctx, fmt.Sprintf("tmp/%d/%02d", n-1, i)); err != nil {
							b.Fatal(err)
						}
					}
				}
				if err := writer.Commit(ctx); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
