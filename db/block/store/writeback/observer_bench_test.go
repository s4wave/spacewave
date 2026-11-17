//go:build !js

package block_store_writeback

import (
	"context"
	"encoding/binary"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/writelog"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

// observerBenchBlocks is the blocks one benchmarked write stores, about one
// World transaction's batch.
const observerBenchBlocks = 32

// newObserverBenchStore opens a MarkingStore over a Volume on a SQLite file,
// marking with a function that does nothing, so the write's cost is the
// Volume's commit and the observer's.
func newObserverBenchStore(b *testing.B) *MarkingStore {
	b.Helper()
	ctx := b.Context()
	kv, err := store_kvtx_sqlite.OpenWithPragmas(ctx, filepath.Join(b.TempDir(), "blocks.db"), "kv", store_kvtx_sqlite.Pragmas{WithoutRowID: true})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = kv.Close() })
	vol, err := volume_kvtx.NewVolume(ctx, "test/writeback", store_kvkey.NewDefaultKVKey(), kv, &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = vol.Close() })
	return NewMarkingStore(ownedStore{vol}, func(context.Context, []Mark) error { return nil })
}

// observerBenchBatch returns a batch of new 4 KiB blocks numbered from n.
func observerBenchBatch(b *testing.B, n uint64) []*block.PutBatchEntry {
	entries := make([]*block.PutBatchEntry, observerBenchBlocks)
	for i := range entries {
		data := make([]byte, 4096)
		binary.LittleEndian.PutUint64(data, n+uint64(i))
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			b.Fatal(err)
		}
		entries[i] = &block.PutBatchEntry{Ref: ref, Data: data, RefsKnown: true}
	}
	return entries
}

// BenchmarkObservedBatch measures one batch write through a MarkingStore on
// SQLite with the write log off and on, serially and from 8 writers.
func BenchmarkObservedBatch(b *testing.B) {
	for _, on := range []bool{false, true} {
		for _, par := range []int{1, 8} {
			b.Run("log="+strconv.FormatBool(on)+"/writers="+strconv.Itoa(par), func(b *testing.B) {
				m := newObserverBenchStore(b)
				if on {
					l := writelog.New(logrus.NewEntry(logrus.New()))
					if err := l.Set(&writelog.Options{Dir: filepath.Join(b.TempDir(), "wlog"), Cap: 1 << 40}); err != nil {
						b.Fatal(err)
					}
					b.Cleanup(func() { _ = l.Close() })
					m.SetObserver(l)
				}
				var next atomic.Uint64
				b.SetParallelism(par)
				b.ReportAllocs()
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						entries := observerBenchBatch(b, next.Add(observerBenchBlocks)<<8)
						if _, err := m.PutBlockBatch(b.Context(), entries); err != nil {
							b.Error(err)
							return
						}
					}
				})
			})
		}
	}
}
